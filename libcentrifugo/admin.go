package libcentrifugo

import (
	"encoding/json"
	"sync"

	"github.com/FZambia/go-logger"
	"github.com/gorilla/websocket"
	"github.com/satori/go.uuid"
)

// use interface to mimic websocket connection write method we use here
type adminSession interface {
	WriteMessage(int, []byte) error
}

// adminClient is a wrapper over admin websocket connection
type adminClient struct {
	sync.RWMutex
	app           *Application
	UID           ConnID
	sess          adminSession
	watch         bool
	authenticated bool
	writeChan     chan []byte
	closeChan     chan struct{}
}

func newAdminClient(app *Application, sess adminSession) (*adminClient, error) {
	c := &adminClient{
		UID:           ConnID(uuid.NewV4().String()),
		app:           app,
		sess:          sess,
		watch:         false,
		authenticated: false,
		writeChan:     make(chan []byte, 256),
		closeChan:     make(chan struct{}),
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

	return c, nil
}

func (c *adminClient) uid() ConnID {
	return c.UID
}

func (c *adminClient) send(message []byte) error {
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
	return c.write(message)
}

func (c *adminClient) write(message []byte) error {
	// TODO: introduce queue here - similar to client's queue.
	select {
	case c.writeChan <- message:
	default:
		logger.ERROR.Println("can't write into admin ws connection write channel")
		return ErrInternalServerError
	}
	return nil
}

// writer reads from buffered channel and sends received messages to admin.
func (c *adminClient) writer() {
	for {
		select {
		case message := <-c.writeChan:
			err := c.sess.WriteMessage(websocket.TextMessage, message)
			if err != nil {
				return
			}
		case <-c.closeChan:
			return
		}
	}
}

// message handles message received from admin connection
func (c *adminClient) message(msg []byte) error {

	commands, err := cmdFromRequestMsg(msg)
	if err != nil {
		logger.ERROR.Println(err)
		return ErrInvalidMessage
	}
	if len(commands) == 0 {
		return nil
	}

	var mr multiAPIResponse

	for _, command := range commands {

		c.Lock()

		if command.Method != "connect" && !c.authenticated {
			c.Unlock()
			return ErrUnauthorized
		}

		var resp response

		switch command.Method {
		case "connect":
			var cmd connectAdminCommand
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
			resp, err = c.app.apiCmd(command)
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

	err = c.write(respBytes)
	if err != nil {
		return ErrInternalServerError
	}

	return nil
}

// connectCmd checks provided token and adds admin connection into application
// registry if token correct
func (c *adminClient) connectCmd(cmd *connectAdminCommand) (response, error) {

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

	return newAPIAdminConnectResponse(true), nil
}

// infoCmd handles info command from admin client.
// TODO: make this a proper API method with strictly defined response body.
func (c *adminClient) infoCmd() (response, error) {
	c.app.nodesMu.Lock()
	defer c.app.nodesMu.Unlock()
	c.app.RLock()
	defer c.app.RUnlock()
	body := adminInfoBody{
		Engine: c.app.engine.name(),
		Config: c.app.config,
	}
	return newAPIAdminInfoResponse(body), nil
}

// pingCmd handles ping command from admin client.
func (c *adminClient) pingCmd() (response, error) {
	return newAPIAdminPingResponse("pong"), nil
}
