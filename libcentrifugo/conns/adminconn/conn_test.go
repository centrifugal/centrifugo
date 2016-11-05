package adminconn

import (
	"testing"

	"github.com/centrifugal/centrifugo/libcentrifugo/auth"
	"github.com/centrifugal/centrifugo/libcentrifugo/conns"
	"github.com/centrifugal/centrifugo/libcentrifugo/node"
	"github.com/centrifugal/centrifugo/libcentrifugo/proto"
	"github.com/stretchr/testify/assert"
)

type TestEngine struct{}

func NewTestEngine() *TestEngine {
	return &TestEngine{}
}

func (e *TestEngine) Name() string {
	return "test engine"
}

func (e *TestEngine) Run() error {
	return nil
}

func (e *TestEngine) Shutdown() error {
	return nil
}

func (e *TestEngine) PublishMessage(message *proto.Message, opts *proto.ChannelOptions) <-chan error {
	eChan := make(chan error, 1)
	eChan <- nil
	return eChan
}

func (e *TestEngine) PublishJoin(message *proto.JoinMessage, opts *proto.ChannelOptions) <-chan error {
	eChan := make(chan error, 1)
	eChan <- nil
	return eChan
}

func (e *TestEngine) PublishLeave(message *proto.LeaveMessage, opts *proto.ChannelOptions) <-chan error {
	eChan := make(chan error, 1)
	eChan <- nil
	return eChan
}

func (e *TestEngine) PublishAdmin(message *proto.AdminMessage) <-chan error {
	eChan := make(chan error, 1)
	eChan <- nil
	return eChan
}

func (e *TestEngine) PublishControl(message *proto.ControlMessage) <-chan error {
	eChan := make(chan error, 1)
	eChan <- nil
	return eChan
}

func (e *TestEngine) Subscribe(ch string) error {
	return nil
}

func (e *TestEngine) Unsubscribe(ch string) error {
	return nil
}

func (e *TestEngine) AddPresence(ch string, uid string, info proto.ClientInfo, expire int) error {
	return nil
}

func (e *TestEngine) RemovePresence(ch string, uid string) error {
	return nil
}

func (e *TestEngine) Presence(ch string) (map[string]proto.ClientInfo, error) {
	return map[string]proto.ClientInfo{}, nil
}

func (e *TestEngine) History(ch string, limit int) ([]proto.Message, error) {
	return []proto.Message{}, nil
}

func (e *TestEngine) Channels() ([]string, error) {
	return []string{}, nil
}

type TestSession struct {
	sink   chan []byte
	closed bool
}

func NewTestSession() *TestSession {
	return &TestSession{}
}

func (t *TestSession) Send(msg []byte) error {
	if t.sink != nil {
		t.sink <- msg
	}
	return nil
}

func (t *TestSession) Close(adv *conns.DisconnectAdvice) error {
	t.closed = true
	return nil
}

func getTestChannelOptions() proto.ChannelOptions {
	return proto.ChannelOptions{
		Watch:           true,
		Publish:         true,
		Presence:        true,
		HistorySize:     1,
		HistoryLifetime: 1,
	}
}

func getTestNamespace(name node.NamespaceKey) node.Namespace {
	return node.Namespace{
		Name:           name,
		ChannelOptions: getTestChannelOptions(),
	}
}

func NewTestConfig() *node.Config {
	c := node.DefaultConfig
	var ns []node.Namespace
	ns = append(ns, getTestNamespace("test"))
	c.Namespaces = ns
	c.Secret = "secret"
	c.ChannelOptions = getTestChannelOptions()
	return c
}

func NewTestNode() *node.Node {
	c := NewTestConfig()
	n := node.New("", c)
	err := n.Run(&node.RunOptions{Engine: NewTestEngine()})
	if err != nil {
		panic(err)
	}
	return n
}

func NewTestNodeWithConfig(c *node.Config) *node.Node {
	if c == nil {
		c = NewTestConfig()
	}
	n := node.New("", c)
	err := n.Run(&node.RunOptions{Engine: NewTestEngine()})
	if err != nil {
		panic(err)
	}
	return n
}

type testAdminSession struct{}

func (s *testAdminSession) Send([]byte) error {
	return nil
}

func (s *testAdminSession) Close(adv *conns.DisconnectAdvice) error {
	return nil
}

func newAdminTestConfig() *node.Config {
	return &node.Config{
		AdminSecret: "secret",
	}
}

func newAdminTestNode() *node.Node {
	conf := newAdminTestConfig()
	return NewTestNodeWithConfig(conf)
}

func newTestAdminClient() (conns.AdminConn, error) {
	n := newAdminTestNode()
	c, err := New(n, &testAdminSession{})
	return c, err
}

func newInsecureTestAdminClient() (conns.AdminConn, error) {
	n := newAdminTestNode()
	conf := n.Config()
	conf.InsecureAdmin = true
	n.SetConfig(&conf)
	c, err := New(n, &testAdminSession{})
	return c, err
}

func TestAdminClient(t *testing.T) {
	c, err := newTestAdminClient()
	assert.Equal(t, nil, err)
	assert.NotEqual(t, c.UID(), "")
	err = c.Send([]byte("message"))
	assert.Equal(t, nil, err)
}

func TestAdminClientMessageHandling(t *testing.T) {
	c, err := newTestAdminClient()
	assert.Equal(t, nil, err)
	emptyMsg := ""
	err = c.Handle([]byte(emptyMsg))
	assert.Equal(t, nil, err)
	malformedMsg := "ooops"
	err = c.Handle([]byte(malformedMsg))
	assert.NotEqual(t, nil, err)
	emptyAuthMethod := "{\"method\":\"connect\", \"params\": {\"watch\": true}}"
	err = c.Handle([]byte(emptyAuthMethod))
	assert.Equal(t, proto.ErrUnauthorized, err)
	token, _ := auth.GenerateAdminToken("secret")
	correctAuthMethod := "{\"method\":\"connect\", \"params\": {\"token\":\"" + token + "\", \"watch\": true}}"
	err = c.Handle([]byte(correctAuthMethod))
	assert.Equal(t, nil, err)
	unknownMsg := "{\"method\":\"unknown\", \"params\": {}}"
	err = c.Handle([]byte(unknownMsg))
	assert.Equal(t, proto.ErrMethodNotFound, err)
	infoCommand := "{\"method\":\"info\", \"params\": {}}"
	err = c.Handle([]byte(infoCommand))
	assert.Equal(t, nil, err)
	pingCommand := "{\"method\":\"ping\", \"params\": {}}"
	err = c.Handle([]byte(pingCommand))
	assert.Equal(t, nil, err)
}

func TestAdminClientAuthentication(t *testing.T) {
	c, err := newTestAdminClient()
	assert.Equal(t, nil, err)
	infoCommand := "{\"method\":\"info\", \"params\": {}}"
	err = c.Handle([]byte(infoCommand))
	assert.Equal(t, proto.ErrUnauthorized, err)
}

func TestAdminClientInsecure(t *testing.T) {
	c, err := newInsecureTestAdminClient()
	assert.Equal(t, nil, err)
	infoCommand := "{\"method\":\"info\", \"params\": {}}"
	err = c.Handle([]byte(infoCommand))
	assert.Equal(t, nil, err)
}

func TestAdminClientNotWatching(t *testing.T) {
	c, err := newTestAdminClient()
	assert.Equal(t, nil, err)
	token, _ := auth.GenerateAdminToken("secret")
	correctAuthMethod := "{\"method\":\"connect\", \"params\": {\"token\":\"" + token + "\"}}"
	err = c.Handle([]byte(correctAuthMethod))
	assert.Equal(t, nil, err)
	assert.Equal(t, false, c.(*adminClient).watch)
}

func TestAdminAuthToken(t *testing.T) {
	app := NewTestNode()
	// first without secret set
	err := checkAdminAuthToken(app, "")
	assert.Equal(t, proto.ErrUnauthorized, err)

	conf := app.Config()
	conf.AdminSecret = "secret"
	app.SetConfig(&conf)

	err = checkAdminAuthToken(app, "")
	assert.Equal(t, proto.ErrUnauthorized, err)

	token, err := auth.GenerateAdminToken("secret")
	assert.Equal(t, nil, err)
	assert.True(t, len(token) > 0)
	err = checkAdminAuthToken(app, token)
	assert.Equal(t, nil, err)

}
