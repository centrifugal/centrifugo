package server

import (
	"encoding/json"
	"sync"

	"github.com/FZambia/go-logger"
	"github.com/centrifugal/centrifugo/libcentrifugo/bytequeue"
	"github.com/centrifugal/centrifugo/libcentrifugo/proto"
	"github.com/gorilla/securecookie"
	"github.com/satori/go.uuid"
)

const (
	// AuthTokenKey is a key for admin authorization token.
	AuthTokenKey = "token"
	// AuthTokenValue is a value for secure admin authorization token.
	AuthTokenValue = "authorized"
)

func (app *Application) adminAuthToken() (string, error) {
	app.RLock()
	secret := app.config.AdminSecret
	app.RUnlock()
	if secret == "" {
		logger.ERROR.Println("provide web_secret in configuration")
		return "", ErrInternalServerError
	}
	s := securecookie.New([]byte(secret), nil)
	return s.Encode(AuthTokenKey, AuthTokenValue)
}

// checkAdminAuthToken checks admin connection token which Centrifugo returns after admin login.
func (app *Application) checkAdminAuthToken(token string) error {

	app.RLock()
	insecure := app.config.InsecureAdmin
	secret := app.config.AdminSecret
	app.RUnlock()

	if insecure {
		return nil
	}

	if secret == "" {
		logger.ERROR.Println("provide admin_secret in configuration")
		return ErrUnauthorized
	}

	if token == "" {
		return ErrUnauthorized
	}

	s := securecookie.New([]byte(secret), nil)
	var val string
	err := s.Decode(AuthTokenKey, token, &val)
	if err != nil {
		return ErrUnauthorized
	}

	if val != AuthTokenValue {
		return ErrUnauthorized
	}
	return nil
}

// adminQueueMaxSize sets admin queue max size to 10MB.
const adminQueueMaxSize = 10485760

// adminClient is a wrapper over admin connection.
type adminClient struct {
	sync.RWMutex
	app           *Application
	uid           proto.ConnID
	sess          session
	watch         bool
	authenticated bool
	closeCh       chan struct{}
	closed        bool
	maxQueueSize  int
	messages      bytequeue.ByteQueue
}

func newAdminClient(app *Application, sess session) (*adminClient, error) {
	c := &adminClient{
		uid:           proto.ConnID(uuid.NewV4().String()),
		app:           app,
		sess:          sess,
		watch:         false,
		authenticated: false,
		closeCh:       make(chan struct{}),
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

// Close called to close connection.
func (c *adminClient) Close(reason string) error {
	c.Lock()
	defer c.Unlock()

	if c.closed {
		return nil
	}

	if reason != "" {
		logger.DEBUG.Printf("Closing admin connection %s: %s", c.UID(), reason)
	}

	close(c.closeCh)
	c.closed = true

	c.messages.Close()
	c.sess.Close(CloseStatus, reason)

	err := c.app.removeAdminConn(c)
	if err != nil {
		logger.ERROR.Println(err)
		return nil
	}

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
			c.Close("error sending message")
			return
		}
	}
}

func (c *adminClient) UID() proto.ConnID {
	return c.uid
}

func (c *adminClient) Send(message []byte) error {
	if c.messages.Size() > c.maxQueueSize {
		c.Close("slow")
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

	err = c.Send(respBytes)
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
	c.app.RLock()
	defer c.app.RUnlock()
	body := proto.AdminInfoBody{
		Data: map[string]interface{}{
			"engine": c.app.engine.Name(),
			"config": c.app.config,
		},
	}
	return proto.NewAPIAdminInfoResponse(body), nil
}

// pingCmd handles ping command from admin client.
func (c *adminClient) pingCmd() (proto.Response, error) {
	return proto.NewAPIAdminPingResponse("pong"), nil
}
