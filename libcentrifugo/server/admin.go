package server

import (
	"encoding/json"
	"sync"

	"github.com/FZambia/go-logger"
	"github.com/centrifugal/centrifugo/libcentrifugo/bytequeue"
	"github.com/centrifugal/centrifugo/libcentrifugo/proto"
	"github.com/satori/go.uuid"
)

// adminQueueMaxSize sets admin queue max size to 10MB.
const adminQueueMaxSize = 10485760

// adminClient is a wrapper over admin connection.
type adminClient struct {
	sync.RWMutex
	app           *Application
	UID           proto.ConnID
	sess          session
	watch         bool
	authenticated bool
	closeChan     chan struct{}
	maxQueueSize  int
	messages      bytequeue.ByteQueue
}

func newAdminClient(app *Application, sess session) (*adminClient, error) {
	c := &adminClient{
		UID:           proto.ConnID(uuid.NewV4().String()),
		app:           app,
		sess:          sess,
		watch:         false,
		authenticated: false,
		closeChan:     make(chan struct{}),
		maxQueueSize:  adminQueueMaxSize,
		messages:      bytequeue.New(2),
	}

	app.RLock()
	insecure := app.config.InsecureAdmin
	app.RUnlock()

	if insecure {
		err := app.addAdminConn(c)
		if err != nil {
			return nil, err
		}
		c.authenticated = true
	}

	go c.sendMessages()

	return c, nil
}

func (c *adminClient) close(reason string) error {
	// TODO: better locking for client - at moment we close message queue in 2 places, here and in clean() method
	c.messages.Close()
	c.sess.Close(CloseStatus, reason)
	return nil
}

// clean called when connection was closed to make different clean up
// actions for a client
func (c *adminClient) clean() error {
	c.Lock()
	defer c.Unlock()

	select {
	case <-c.closeChan:
		return nil
	default:
		close(c.closeChan)
	}

	err := c.app.removeAdminConn(c)
	if err != nil {
		logger.ERROR.Println(err)
		return nil
	}
	c.messages.Close()
	c.authenticated = false
	return nil
}

// sendMessages waits for messages from queue and sends them to client.
func (c *adminClient) sendMessages() {
	for {
		msg, ok := c.messages.Wait()
		if !ok {
			if c.messages.Closed() {
				return
			}
			continue
		}
		err := c.sess.Send(msg)
		if err != nil {
			logger.INFO.Println("error sending to", c.uid(), err.Error())
			c.close("error sending message")
			return
		}
	}
}

func (c *adminClient) uid() proto.ConnID {
	return c.UID
}

func (c *adminClient) send(message []byte) error {
	if c.messages.Size() > c.maxQueueSize {
		c.close("slow")
		return ErrClientClosed
	}
	if !c.watch {
		// At moment we only use this method to send asynchronous
		// messages to admin client when new message published into channel with
		// watch option enabled and admin watch option was set to true in connection
		// params. If we introduce another types of asynchronous admin messages coming
		// from admin hub broadcast then we will need to refactor this – it seems that
		// we then need to know a type of message coming to decide what to do with it
		// on broadcast level, for example 1-byte prefix meaning message type (what
		// @banks actually suggested).
		return nil
	}
	ok := c.messages.Add(message)
	if !ok {
		return ErrClientClosed
	}
	return nil
}

// message handles message received from admin connection
func (c *adminClient) message(msg []byte) error {

	cmds, err := cmdFromRequestMsg(msg)
	if err != nil {
		logger.ERROR.Println(err)
		return ErrInvalidMessage
	}
	if len(cmds) == 0 {
		return nil
	}

	var mr proto.MultiAPIResponse

	for _, command := range cmds {

		c.Lock()

		if command.Method != "connect" && !c.authenticated {
			c.Unlock()
			return ErrUnauthorized
		}

		var resp proto.Response

		switch command.Method {
		case "connect":
			var cmd proto.ConnectAdminCommand
			err = json.Unmarshal(command.Params, &cmd)
			if err != nil {
				c.Unlock()
				logger.ERROR.Println(err)
				return ErrInvalidMessage
			}
			resp, err = c.connectCmd(&cmd)
		case "ping":
			resp, err = c.pingCmd()
		case "info":
			resp, err = c.infoCmd()
		default:
			resp, err = c.app.APICmd(command)
		}
		if err != nil {
			c.Unlock()
			logger.ERROR.Println(err)
			return err
		}

		c.Unlock()

		resp.SetUID(command.UID)
		mr = append(mr, resp)
	}

	respBytes, err := json.Marshal(mr)
	if err != nil {
		logger.ERROR.Println(err)
		return ErrInternalServerError
	}

	err = c.send(respBytes)
	if err != nil {
		return ErrInternalServerError
	}

	return nil
}

// connectCmd checks provided token and adds admin connection into application
// registry if token correct
func (c *adminClient) connectCmd(cmd *proto.ConnectAdminCommand) (proto.Response, error) {

	err := c.app.checkAdminAuthToken(cmd.Token)
	if err != nil {
		return nil, ErrUnauthorized
	}

	err = c.app.addAdminConn(c)
	if err != nil {
		logger.ERROR.Println(err)
		return nil, ErrInternalServerError
	}

	c.authenticated = true

	if cmd.Watch {
		c.watch = true
	}

	return proto.NewAPIAdminConnectResponse(true), nil
}

// infoCmd handles info command from admin client.
func (c *adminClient) infoCmd() (proto.Response, error) {
	c.app.nodesMu.Lock()
	defer c.app.nodesMu.Unlock()
	c.app.RLock()
	defer c.app.RUnlock()
	body := proto.AdminInfoBody{
		Engine: c.app.engine.Name(),
		Config: c.app.config,
	}
	return proto.NewAPIAdminInfoResponse(body), nil
}

// pingCmd handles ping command from admin client.
func (c *adminClient) pingCmd() (proto.Response, error) {
	return proto.NewAPIAdminPingResponse("pong"), nil
}
