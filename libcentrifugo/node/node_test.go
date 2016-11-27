package node

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/libcentrifugo/conns"
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

type testSession struct {
	sink   chan []byte
	closed bool
}

func (t *testSession) Send(msg []byte) error {
	if t.sink != nil {
		t.sink <- msg
	}
	return nil
}

func (t *testSession) Close(adv *conns.DisconnectAdvice) error {
	t.closed = true
	return nil
}

func testNode() *Node {
	c := newTestConfig()
	n := New("", &c)
	err := n.Run(&RunOptions{Engine: NewTestEngine()})
	if err != nil {
		panic(err)
	}
	return n
}

func testNodeWithConfig(c *Config) *Node {
	if c == nil {
		conf := newTestConfig()
		c = &conf
	}
	n := New("", c)
	n.engine = NewTestEngine()
	return n
}

func TestUserAllowed(t *testing.T) {
	app := testNode()
	assert.Equal(t, true, app.UserAllowed("channel#1", "1"))
	assert.Equal(t, true, app.UserAllowed("channel", "1"))
	assert.Equal(t, false, app.UserAllowed("channel#1", "2"))
	assert.Equal(t, true, app.UserAllowed("channel#1,2", "1"))
	assert.Equal(t, true, app.UserAllowed("channel#1,2", "2"))
	assert.Equal(t, false, app.UserAllowed("channel#1,2", "3"))
}

func TestSetConfig(t *testing.T) {
	app := testNode()
	c := newTestConfig()
	app.SetConfig(&c)
}

func TestClientAllowed(t *testing.T) {
	app := testNode()
	assert.Equal(t, true, app.ClientAllowed("channel&67330d48-f668-4916-758b-f4eb1dd5b41d", string("67330d48-f668-4916-758b-f4eb1dd5b41d")))
	assert.Equal(t, true, app.ClientAllowed("channel", string("67330d48-f668-4916-758b-f4eb1dd5b41d")))
	assert.Equal(t, false, app.ClientAllowed("channel&long-client-id", string("wrong-client-id")))
}

func TestNamespaceKey(t *testing.T) {
	app := testNode()
	assert.Equal(t, NamespaceKey("ns"), app.namespaceKey("ns:channel"))
	assert.Equal(t, NamespaceKey(""), app.namespaceKey("channel"))
	assert.Equal(t, NamespaceKey("ns"), app.namespaceKey("ns:channel:opa"))
	assert.Equal(t, NamespaceKey("ns"), app.namespaceKey("ns::channel"))
}

func TestApplicationNode(t *testing.T) {
	app := testNode()
	err := app.Run(&RunOptions{Engine: NewTestEngine()})
	assert.Equal(t, nil, err)
	info := app.Node()
	assert.Equal(t, int64(0), info.Metrics["num_clients"])
	assert.NotEqual(t, 0, info.Started)
}

func BenchmarkNamespaceKey(b *testing.B) {
	app := testNode()
	ch := string("test")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		app.namespaceKey(ch)
	}
}

func testPingControlCmd(uid string) *proto.ControlMessage {
	return proto.NewControlMessage(uid, "ping", []byte("{}"))
}

func testUnsubscribeControlCmd(uid string) *proto.ControlMessage {
	return proto.NewControlMessage(uid, "unsubscribe", []byte("{}"))
}

func testDisconnectControlCmd(uid string) *proto.ControlMessage {
	return proto.NewControlMessage(uid, "disconnect", []byte("{}"))
}

func testWrongControlCmd(uid string) *proto.ControlMessage {
	return proto.NewControlMessage(uid, "wrong", []byte("{}"))
}

func TestControlMessages(t *testing.T) {
	app := testNode()
	// command from this node
	cmd := testPingControlCmd(app.uid)
	err := app.ControlMsg(cmd)
	assert.Equal(t, nil, err)
	cmd = testPingControlCmd("another_node")
	err = app.ControlMsg(cmd)
	assert.Equal(t, nil, err)
	err = app.ControlMsg(testWrongControlCmd("another node"))
	assert.Equal(t, proto.ErrInvalidMessage, err)
	err = app.ControlMsg(testUnsubscribeControlCmd("another node"))
	assert.Equal(t, nil, err)
	err = app.ControlMsg(testDisconnectControlCmd("another node"))
	assert.Equal(t, nil, err)
}

func TestPublishJoinLeave(t *testing.T) {
	app := testNode()
	err := <-app.PublishJoin(proto.NewJoinMessage("channel-0", proto.ClientInfo{}), nil)
	assert.Equal(t, nil, err)
	err = <-app.PublishLeave(proto.NewLeaveMessage("channel-0", proto.ClientInfo{}), nil)
	assert.Equal(t, nil, err)
}

func TestUpdateMetrics(t *testing.T) {
	app := testNode()
	data, _ := json.Marshal(map[string]string{"test": "publish"})
	err := <-app.Publish(proto.NewMessage("channel-0", data, "", nil), nil)
	assert.Equal(t, nil, err)

	config := app.Config()
	config.NodeMetricsInterval = 1 * time.Millisecond
	app.SetConfig(&config)

	app.updateMetricsOnce()

	// Absolute metrics should be updated
	assert.True(t, metricsRegistry.Counters.LoadValues()["node_num_client_msg_published"] > 0)
}

func TestNodeRegistry(t *testing.T) {
	registry := newNodeRegistry("node1")
	nodeInfo1 := proto.NodeInfo{UID: "node1"}
	nodeInfo2 := proto.NodeInfo{UID: "node2"}
	registry.add(nodeInfo1)
	registry.add(nodeInfo2)
	assert.Equal(t, 2, len(registry.list()))
	info := registry.get("node1")
	assert.Equal(t, "node1", info.UID)
	registry.clean(10 * time.Second)
	time.Sleep(2 * time.Second)
	registry.clean(time.Second)
	// Current node info should still be in node registry - we never delete it.
	assert.Equal(t, 1, len(registry.list()))
}
