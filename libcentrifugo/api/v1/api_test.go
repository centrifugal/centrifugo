package apiv1

import (
	"testing"

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

func (t *TestSession) Close(status uint32, reason string) error {
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

func TestAPICmd(t *testing.T) {
	app := NewTestNode()

	cmd := proto.APICommand{
		Method: "nonexistent",
		Params: []byte("{}"),
	}
	_, err := APICmd(app, cmd)
	assert.Equal(t, err, proto.ErrMethodNotFound)

	cmd = proto.APICommand{
		Method: "publish",
		Params: []byte("{}"),
	}
	_, err = APICmd(app, cmd)
	assert.Equal(t, err, proto.ErrInvalidMessage)

	cmd = proto.APICommand{
		Method: "publish",
		Params: []byte("test"),
	}
	_, err = APICmd(app, cmd)
	assert.Equal(t, proto.ErrInvalidMessage, err)

	cmd = proto.APICommand{
		Method: "broadcast",
		Params: []byte("{}"),
	}
	resp, err := APICmd(app, cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, proto.ErrInvalidMessage, resp.(*proto.APIBroadcastResponse).ResponseError.Err)

	cmd = proto.APICommand{
		Method: "broadcast",
		Params: []byte("test"),
	}
	_, err = APICmd(app, cmd)
	assert.Equal(t, proto.ErrInvalidMessage, err)

	cmd = proto.APICommand{
		Method: "unsubscribe",
		Params: []byte("{}"),
	}
	resp, err = APICmd(app, cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, proto.ErrInvalidMessage, resp.(*proto.APIUnsubscribeResponse).ResponseError.Err)

	cmd = proto.APICommand{
		Method: "unsubscribe",
		Params: []byte("test"),
	}
	_, err = APICmd(app, cmd)
	assert.Equal(t, proto.ErrInvalidMessage, err)

	cmd = proto.APICommand{
		Method: "disconnect",
		Params: []byte("{}"),
	}
	resp, err = APICmd(app, cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, proto.ErrInvalidMessage, resp.(*proto.APIDisconnectResponse).ResponseError.Err)

	cmd = proto.APICommand{
		Method: "disconnect",
		Params: []byte("test"),
	}
	_, err = APICmd(app, cmd)
	assert.Equal(t, proto.ErrInvalidMessage, err)

	cmd = proto.APICommand{
		Method: "presence",
		Params: []byte("{}"),
	}
	resp, err = APICmd(app, cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, proto.ErrInvalidMessage, resp.(*proto.APIPresenceResponse).ResponseError.Err)

	cmd = proto.APICommand{
		Method: "presence",
		Params: []byte("test"),
	}
	_, err = APICmd(app, cmd)
	assert.Equal(t, proto.ErrInvalidMessage, err)

	cmd = proto.APICommand{
		Method: "history",
		Params: []byte("{}"),
	}
	resp, err = APICmd(app, cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, proto.ErrInvalidMessage, resp.(*proto.APIHistoryResponse).ResponseError.Err)

	cmd = proto.APICommand{
		Method: "history",
		Params: []byte("test"),
	}
	_, err = APICmd(app, cmd)
	assert.Equal(t, proto.ErrInvalidMessage, err)

	cmd = proto.APICommand{
		Method: "channels",
		Params: []byte("{}"),
	}
	resp, err = APICmd(app, cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, resp.(*proto.APIChannelsResponse).ResponseError.Err)

	cmd = proto.APICommand{
		Method: "stats",
		Params: []byte("{}"),
	}
	resp, err = APICmd(app, cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, resp.(*proto.APIStatsResponse).ResponseError.Err)

	cmd = proto.APICommand{
		Method: "node",
		Params: []byte("{}"),
	}
	resp, err = APICmd(app, cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, resp.(*proto.APINodeResponse).ResponseError.Err)
}

func TestAPIPublish(t *testing.T) {
	app := NewTestNode()
	cmd := proto.PublishAPICommand{
		Channel: "channel",
		Data:    []byte("null"),
	}
	resp, err := PublishCmd(app, cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, resp.(*proto.APIPublishResponse).ResponseError.Err)
	cmd = proto.PublishAPICommand{
		Channel: "nonexistentnamespace:channel-2",
		Data:    []byte("null"),
	}
	resp, err = PublishCmd(app, cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, proto.ErrNamespaceNotFound, resp.(*proto.APIPublishResponse).ResponseError.Err)
}

func TestAPIBroadcast(t *testing.T) {
	app := NewTestNode()
	cmd := proto.BroadcastAPICommand{
		Channels: []string{"channel-1", "channel-2"},
		Data:     []byte("null"),
	}
	resp, err := BroadcastCmd(app, cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, resp.(*proto.APIBroadcastResponse).ResponseError.Err)
	cmd = proto.BroadcastAPICommand{
		Channels: []string{"channel-1", "nonexistentnamespace:channel-2"},
		Data:     []byte("null"),
	}
	resp, err = BroadcastCmd(app, cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, proto.ErrNamespaceNotFound, resp.(*proto.APIBroadcastResponse).ResponseError.Err)
	cmd = proto.BroadcastAPICommand{
		Channels: []string{},
		Data:     []byte("null"),
	}
	resp, err = BroadcastCmd(app, cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, proto.ErrInvalidMessage, resp.(*proto.APIBroadcastResponse).ResponseError.Err)
}

func TestAPIUnsubscribe(t *testing.T) {
	app := NewTestNode()
	cmd := proto.UnsubscribeAPICommand{
		User:    "test user",
		Channel: "channel",
	}
	resp, err := UnsubscribeCmd(app, cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, resp.(*proto.APIUnsubscribeResponse).ResponseError.Err)

	// unsubscribe from all channels
	cmd = proto.UnsubscribeAPICommand{
		User:    "test user",
		Channel: "",
	}
	resp, err = UnsubscribeCmd(app, cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, resp.(*proto.APIUnsubscribeResponse).ResponseError.Err)
}

func TestAPIDisconnect(t *testing.T) {
	app := NewTestNode()
	cmd := proto.DisconnectAPICommand{
		User: "test user",
	}
	resp, err := DisconnectCmd(app, cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, resp.(*proto.APIDisconnectResponse).ResponseError.Err)
}

func TestAPIPresence(t *testing.T) {
	app := NewTestNode()
	cmd := proto.PresenceAPICommand{
		Channel: "channel",
	}
	resp, err := PresenceCmd(app, cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, resp.(*proto.APIPresenceResponse).ResponseError.Err)
}

func TestAPIHistory(t *testing.T) {
	app := NewTestNode()
	cmd := proto.HistoryAPICommand{
		Channel: "channel",
	}
	resp, err := HistoryCmd(app, cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, resp.(*proto.APIHistoryResponse).ResponseError.Err)
}

func TestAPIChannels(t *testing.T) {
	app := NewTestNode()
	resp, err := ChannelsCmd(app)
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, resp.(*proto.APIChannelsResponse).ResponseError.Err)
}

func TestAPIStats(t *testing.T) {
	app := NewTestNode()
	resp, err := StatsCmd(app)
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, resp.(*proto.APIStatsResponse).ResponseError.Err)
}

func TestAPINode(t *testing.T) {
	app := NewTestNode()
	resp, err := NodeCmd(app)
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, resp.(*proto.APINodeResponse).ResponseError.Err)
}
