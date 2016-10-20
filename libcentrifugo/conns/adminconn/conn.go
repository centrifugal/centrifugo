package adminconn

import (
	"encoding/json"
	"sync"

	"github.com/FZambia/go-logger"
	"github.com/centrifugal/centrifugo/libcentrifugo/api/v1"
	"github.com/centrifugal/centrifugo/libcentrifugo/bytequeue"
	"github.com/centrifugal/centrifugo/libcentrifugo/conns"
	"github.com/centrifugal/centrifugo/libcentrifugo/node"
	"github.com/centrifugal/centrifugo/libcentrifugo/proto"
	"github.com/gorilla/securecookie"
	"github.com/satori/go.uuid"
)

const (
	// CloseStatus is status code set when closing client connections.
	CloseStatus = 3000
)

const (
	// AuthTokenKey is a key for admin authorization token.
	AuthTokenKey = "token"
	// AuthTokenValue is a value for secure admin authorization token.
	AuthTokenValue = "authorized"
)

type AdminOptions struct{}

func AdminAuthToken(secret string) (string, error) {
	if secret == "" {
		logger.ERROR.Println("provide admin_secret in configuration")
		return "", proto.ErrInternalServerError
	}
	s := securecookie.New([]byte(secret), nil)
	return s.Encode(AuthTokenKey, AuthTokenValue)
}

// checkAdminAuthToken checks admin connection token which Centrifugo returns after admin login.
func checkAdminAuthToken(n *node.Node, token string) error {

	config := n.Config()
	insecure := config.InsecureAdmin
	secret := config.AdminSecret

	if insecure {
		return nil
	}

	if secret == "" {
		logger.ERROR.Println("provide admin_secret in configuration")
		return proto.ErrUnauthorized
	}

	if token == "" {
		return proto.ErrUnauthorized
	}

	s := securecookie.New([]byte(secret), nil)
	var val string
	err := s.Decode(AuthTokenKey, token, &val)
	if err != nil {
		return proto.ErrUnauthorized
	}

	if val != AuthTokenValue {
		return proto.ErrUnauthorized
	}
	return nil
}

// adminQueueMaxSize sets admin queue max size to 10MB.
const adminQueueMaxSize = 10485760

// adminClient is a wrapper over admin connection.
type adminClient struct {
	sync.RWMutex
	node          *node.Node
	uid           proto.ConnID
	sess          conns.Session
	watch         bool
	authenticated bool
	closeCh       chan struct{}
	closed        bool
	maxQueueSize  int
	messages      bytequeue.ByteQueue
}

var (
	arrayJSONPrefix  byte = '['
	objectJSONPrefix byte = '{'
)

func apiCommandsFromJSON(msg []byte) ([]proto.ApiCommand, error) {
	var cmds []proto.ApiCommand

	if len(msg) == 0 {
		return cmds, nil
	}

	firstByte := msg[0]

	switch firstByte {
	case objectJSONPrefix:
		// single command request
		var command proto.ApiCommand
		err := json.Unmarshal(msg, &command)
		if err != nil {
			return nil, err
		}
		cmds = append(cmds, command)
	case arrayJSONPrefix:
		// array of commands received
		err := json.Unmarshal(msg, &cmds)
		if err != nil {
			return nil, err
		}
	default:
		return nil, proto.ErrInvalidMessage
	}
	return cmds, nil
}

func New(n *node.Node, sess conns.Session, opts *AdminOptions) (conns.AdminConn, error) {
	c := &adminClient{
		uid:           proto.ConnID(uuid.NewV4().String()),
		node:          n,
		sess:          sess,
		watch:         false,
		authenticated: false,
		closeCh:       make(chan struct{}),
		maxQueueSize:  adminQueueMaxSize,
		messages:      bytequeue.New(2),
	}

	insecure := c.node.Config().InsecureAdmin

	if insecure {
		err := c.node.AdminHub().Add(c)
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

	err := c.node.AdminHub().Remove(c)
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
		return proto.ErrClientClosed
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
		return proto.ErrClientClosed
	}
	return nil
}

// Handle handles message received from admin connection
func (c *adminClient) Handle(msg []byte) error {

	cmds, err := apiCommandsFromJSON(msg)
	if err != nil {
		logger.ERROR.Println(err)
		return proto.ErrInvalidMessage
	}
	if len(cmds) == 0 {
		return nil
	}

	var mr proto.MultiAPIResponse

	for _, command := range cmds {

		c.Lock()

		if command.Method != "connect" && !c.authenticated {
			c.Unlock()
			return proto.ErrUnauthorized
		}

		var resp proto.Response

		switch command.Method {
		case "connect":
			var cmd proto.ConnectAdminCommand
			err = json.Unmarshal(command.Params, &cmd)
			if err != nil {
				c.Unlock()
				logger.ERROR.Println(err)
				return proto.ErrInvalidMessage
			}
			resp, err = c.connectCmd(&cmd)
		case "ping":
			resp, err = c.pingCmd()
		case "info":
			resp, err = c.infoCmd()
		default:
			resp, err = apiv1.APICmd(c.node, command, nil)
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
		return proto.ErrInternalServerError
	}

	err = c.Send(respBytes)
	if err != nil {
		return proto.ErrInternalServerError
	}

	return nil
}

// connectCmd checks provided token and adds admin connection into application
// registry if token correct
func (c *adminClient) connectCmd(cmd *proto.ConnectAdminCommand) (proto.Response, error) {

	err := checkAdminAuthToken(c.node, cmd.Token)
	if err != nil {
		return nil, proto.ErrUnauthorized
	}

	err = c.node.AdminHub().Add(c)
	if err != nil {
		logger.ERROR.Println(err)
		return nil, proto.ErrInternalServerError
	}

	c.authenticated = true

	if cmd.Watch {
		c.watch = true
	}

	return proto.NewAPIAdminConnectResponse(true), nil
}

// infoCmd handles info command from admin client.
func (c *adminClient) infoCmd() (proto.Response, error) {
	body := proto.AdminInfoBody{
		Data: map[string]interface{}{
			"version": c.node.Version(),
			"engine":  c.node.Engine().Name(),
			"config":  c.node.Config(),
		},
	}
	return proto.NewAPIAdminInfoResponse(body), nil
}

// pingCmd handles ping command from admin client.
func (c *adminClient) pingCmd() (proto.Response, error) {
	return proto.NewAPIAdminPingResponse("pong"), nil
}
