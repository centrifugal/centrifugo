package main

import (
	"encoding/json"

	"github.com/centrifugal/centrifugo/logger"
	"github.com/gorilla/securecookie"
	"github.com/gorilla/websocket"
	"github.com/mitchellh/mapstructure"
	"github.com/nu7hatch/gouuid"
)

// adminClient is a wrapper over admin websocket connection
type adminClient struct {
	app          *application
	uid          string
	ws           *websocket.Conn
	writeChannel chan []byte
	closeChannel chan struct{}
}

func newAdminClient(app *application, ws *websocket.Conn) (*adminClient, error) {
	uid, err := uuid.NewV4()
	if err != nil {
		return nil, err
	}
	return &adminClient{
		uid:          uid.String(),
		app:          app,
		ws:           ws,
		writeChannel: make(chan []byte, 256),
		closeChannel: make(chan struct{}),
	}, nil
}

func (c *adminClient) getUid() string {
	return c.uid
}

func (c *adminClient) send(message string) error {
	select {
	case c.writeChannel <- []byte(message):
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
		case message := <-c.writeChannel:
			err := c.ws.WriteMessage(websocket.TextMessage, message)
			if err != nil {
				return
			}
		case <-c.closeChannel:
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
	token := cmd.Token
	if token == "" {
		return nil, ErrUnauthorized
	}

	s := securecookie.New([]byte(c.app.secret), nil)
	var val string
	err := s.Decode(tokenKey, token, &val)
	if err != nil {
		return nil, ErrUnauthorized
	}

	if val != tokenValue {
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
