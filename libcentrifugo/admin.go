package libcentrifugo

import (
	"encoding/json"

	"github.com/centrifugal/centrifugo/Godeps/_workspace/src/github.com/gorilla/websocket"
	"github.com/centrifugal/centrifugo/Godeps/_workspace/src/github.com/nu7hatch/gouuid"
	"github.com/centrifugal/centrifugo/libcentrifugo/logger"
)

// use interface to mimic websocket connection write method we use here
type adminSession interface {
	WriteMessage(int, []byte) error
}

// adminClient is a wrapper over admin websocket connection
type adminClient struct {
	app       *Application
	UID       ConnID
	sess      adminSession
	writeChan chan []byte
	closeChan chan struct{}
}

func newAdminClient(app *Application, s adminSession) (*adminClient, error) {
	uid, err := uuid.NewV4()
	if err != nil {
		return nil, err
	}
	return &adminClient{
		UID:       ConnID(uid.String()),
		app:       app,
		sess:      s,
		writeChan: make(chan []byte, 256),
		closeChan: make(chan struct{}),
	}, nil
}

func (c *adminClient) uid() ConnID {
	return c.UID
}

func (c *adminClient) send(message []byte) error {
	select {
	case c.writeChan <- message:
	default:
		logger.ERROR.Println("can't write into admin ws connection write channel")
		return ErrInternalServerError
	}
	return nil
}

// writer reads from channel and sends received messages to admin
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

// handleMessage handles message received from admin connection
func (c *adminClient) handleMessage(msg []byte) (*response, error) {

	var err error
	var resp *response

	var command adminCommand
	err = json.Unmarshal(msg, &command)
	if err != nil {
		return nil, err
	}

	method := command.Method
	params := command.Params

	switch method {
	case "auth":
		var cmd authAdminCommand
		err = json.Unmarshal(params, &cmd)
		if err != nil {
			logger.ERROR.Println(err)
			return nil, ErrInvalidMessage
		}
		resp, err = c.authCmd(&cmd)
	case "ping":
		resp, err = c.pingCmd()
	default:
		return nil, ErrInvalidMessage
	}
	return resp, err
}

// authCmd checks provided token and adds admin connection into application
// registry if token correct
func (c *adminClient) authCmd(cmd *authAdminCommand) (*response, error) {

	err := c.app.checkAdminAuthToken(cmd.Token)
	if err != nil {
		return nil, ErrUnauthorized
	}

	err = c.app.addAdminConn(c)
	if err != nil {
		logger.ERROR.Println(err)
		return nil, ErrInternalServerError
	}

	resp := newResponse("auth")
	resp.Body = true
	return resp, nil
}

// pingCmd handles ping command from admin client
func (c *adminClient) pingCmd() (*response, error) {
	resp := newResponse("ping")
	resp.Body = "pong"
	return resp, nil
}
