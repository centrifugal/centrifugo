package libcentrifugo

import (
	"encoding/json"
	"sync"

	"github.com/centrifugal/centrifugo/Godeps/_workspace/src/github.com/FZambia/go-logger"
	"github.com/centrifugal/centrifugo/Godeps/_workspace/src/github.com/gorilla/websocket"
	"github.com/centrifugal/centrifugo/Godeps/_workspace/src/github.com/satori/go.uuid"
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
	return &adminClient{
		UID:           ConnID(uuid.NewV4().String()),
		app:           app,
		sess:          sess,
		watch:         false,
		authenticated: false,
		writeChan:     make(chan []byte, 256),
		closeChan:     make(chan struct{}),
	}, nil
}

func (c *adminClient) uid() ConnID {
	return c.UID
}

func (c *adminClient) send(message []byte) error {
	c.RLock()
	watch := c.watch
	c.RUnlock()
	if !watch {
		// At moment we only use this method to send asynchronous
		// messages to admin client when new message published into channel
		// and admin watch option was set to true. If we introduce another
		// types of asynchronous admin messages coming from admin hub broadcast
		// then we will need to refactor this – it seems that we then need to
		// know a type of message coming to decide what to do with it on broadcast
		// level, for example 1-byte prefix meaning message type (what @banks
		// actually suggested).
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

	var mr multiResponse

	for _, command := range commands {

		c.Lock()

		if command.Method != "connect" && !c.authenticated {
			return ErrUnauthorized
		}

		var resp *response

		switch command.Method {
		case "connect":
			var cmd connectAdminCommand
			err = json.Unmarshal(command.Params, &cmd)
			if err != nil {
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
			logger.ERROR.Println(err)
			return err
		}

		c.Unlock()

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
func (c *adminClient) connectCmd(cmd *connectAdminCommand) (*response, error) {

	err := c.app.checkAdminAuthToken(cmd.Token)
	if err != nil {
		return nil, ErrUnauthorized
	}

	// TODO: handle case when authentication message not needed.
	c.authenticated = true

	if cmd.Watch {
		c.watch = true
	}

	err = c.app.addAdminConn(c)
	if err != nil {
		logger.ERROR.Println(err)
		return nil, ErrInternalServerError
	}

	resp := newResponse("connect")
	resp.Body = true
	return resp, nil
}

// infoCmd handles info command from admin client.
func (c *adminClient) infoCmd() (*response, error) {
	c.app.nodesMu.Lock()
	defer c.app.nodesMu.Unlock()
	c.app.RLock()
	defer c.app.RUnlock()
	resp := newResponse("info")
	resp.Body = map[string]interface{}{
		"engine": c.app.engine.name(),
		"config": c.app.config,
	}
	return resp, nil
}

// pingCmd handles ping command from admin client.
func (c *adminClient) pingCmd() (*response, error) {
	resp := newResponse("ping")
	resp.Body = "pong"
	return resp, nil
}
