package adminconn

import (
	"encoding/json"
	"sync"

	"github.com/centrifugal/centrifugo/libcentrifugo/api/v1"
	"github.com/centrifugal/centrifugo/libcentrifugo/auth"
	"github.com/centrifugal/centrifugo/libcentrifugo/bytequeue"
	"github.com/centrifugal/centrifugo/libcentrifugo/conns"
	"github.com/centrifugal/centrifugo/libcentrifugo/logger"
	"github.com/centrifugal/centrifugo/libcentrifugo/node"
	"github.com/centrifugal/centrifugo/libcentrifugo/proto"
	"github.com/satori/go.uuid"
)

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

	authenticated := auth.CheckAdminToken(secret, token)
	if !authenticated {
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
	uid           string
	sess          conns.Session
	watch         bool
	authenticated bool
	closeCh       chan struct{}
	closed        bool
	maxQueueSize  int
	messages      bytequeue.ByteQueue
}

// New initializes new AdminConn.
func New(n *node.Node, sess conns.Session) (conns.AdminConn, error) {
	c := &adminClient{
		uid:           uuid.NewV4().String(),
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
func (c *adminClient) Close(advice *conns.DisconnectAdvice) error {
	c.Lock()
	defer c.Unlock()

	if c.closed {
		return nil
	}

	if advice != nil && advice.Reason != "" {
		logger.DEBUG.Printf("Closing admin connection %s: %s", c.UID(), advice.Reason)
	}

	close(c.closeCh)
	c.closed = true

	c.messages.Close()
	c.sess.Close(advice)

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
			c.Close(&conns.DisconnectAdvice{Reason: "error sending message", Reconnect: true})
			return
		}
	}
}

func (c *adminClient) UID() string {
	return c.uid
}

func (c *adminClient) Send(message []byte) error {
	if c.messages.Size() > c.maxQueueSize {
		c.Close(&conns.DisconnectAdvice{Reason: "slow", Reconnect: true})
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

	cmds, err := apiv1.APICommandsFromJSON(msg)
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
			resp, err = apiv1.APICmd(c.node, command)
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

	return proto.NewAdminConnectResponse(true), nil
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
	return proto.NewAdminInfoResponse(body), nil
}

// pingCmd handles ping command from admin client.
func (c *adminClient) pingCmd() (proto.Response, error) {
	return proto.NewAdminPingResponse("pong"), nil
}
