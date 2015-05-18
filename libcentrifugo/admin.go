package libcentrifugo

import (
	"encoding/json"

	"github.com/centrifugal/centrifugo/libcentrifugo/logger"
	"github.com/gorilla/websocket"
	"github.com/mitchellh/mapstructure"
	"github.com/nu7hatch/gouuid"
)

// use interface to mimic websocket connection write method we use here
type adminSession interface {
	WriteMessage(int, []byte) error
}

// adminClient is a wrapper over admin websocket connection
type adminClient struct {
	app       *application
	uid       string
	session   adminSession
	writeChan chan []byte
	closeChan chan struct{}
}

func newAdminClient(app *application, s adminSession) (*adminClient, error) {
	uid, err := uuid.NewV4()
	if err != nil {
		return nil, err
	}
	return &adminClient{
		uid:       uid.String(),
		app:       app,
		session:   s,
		writeChan: make(chan []byte, 256),
		closeChan: make(chan struct{}),
	}, nil
}

func (c *adminClient) getUid() string {
	return c.uid
}

func (c *adminClient) send(message string) error {
	select {
	case c.writeChan <- []byte(message):
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
			err := c.session.WriteMessage(websocket.TextMessage, message)
			if err != nil {
				return
			}
		case <-c.closeChan:
			return
		}
	}
}

// handleMessage handles message received from admin connection
func (c *adminClient) handleMessage(message []byte) (*response, error) {

	var err error
	var resp *response

	var command adminCommand
	err = json.Unmarshal(message, &command)
	if err != nil {
		return nil, err
	}

	method := command.Method
	params := command.Params

	switch method {
	case "auth":
		var cmd authAdminCommand
		err = mapstructure.Decode(params, &cmd)
		if err != nil {
			logger.ERROR.Println(err)
			return nil, ErrInvalidAdminMessage
		}
		resp, err = c.handleAuthCommand(&cmd)
	default:
		return nil, ErrInvalidAdminMessage
	}
	return resp, err
}

// handleAuthCommand checks provided token and adds admin connection into application
// registry if token correct
func (c *adminClient) handleAuthCommand(cmd *authAdminCommand) (*response, error) {

	err := c.app.checkAuthToken(cmd.Token)
	if err != nil {
		return nil, ErrUnauthorized
	}

	err = c.app.addAdminConnection(c)
	if err != nil {
		logger.ERROR.Println(err)
		return nil, ErrInternalServerError
	}

	resp := newResponse("auth")
	resp.Body = true
	return resp, nil
}
