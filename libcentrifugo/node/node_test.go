package node

import (
	"encoding/json"
	"fmt"
	"strconv"
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

func (e *TestEngine) PublishMessage(ch proto.Channel, message *proto.Message, opts *proto.ChannelOptions) <-chan error {
	eChan := make(chan error, 1)
	eChan <- nil
	return eChan
}

func (e *TestEngine) PublishJoin(ch proto.Channel, message *proto.JoinMessage) <-chan error {
	eChan := make(chan error, 1)
	eChan <- nil
	return eChan
}

func (e *TestEngine) PublishLeave(ch proto.Channel, message *proto.LeaveMessage) <-chan error {
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

func (e *TestEngine) Subscribe(ch proto.Channel) error {
	return nil
}

func (e *TestEngine) Unsubscribe(ch proto.Channel) error {
	return nil
}

func (e *TestEngine) AddPresence(ch proto.Channel, uid proto.ConnID, info proto.ClientInfo, expire int) error {
	return nil
}

func (e *TestEngine) RemovePresence(ch proto.Channel, uid proto.ConnID) error {
	return nil
}

func (e *TestEngine) Presence(ch proto.Channel) (map[proto.ConnID]proto.ClientInfo, error) {
	return map[proto.ConnID]proto.ClientInfo{}, nil
}

func (e *TestEngine) History(ch proto.Channel, limit int) ([]proto.Message, error) {
	return []proto.Message{}, nil
}

func (e *TestEngine) Channels() ([]proto.Channel, error) {
	return []proto.Channel{}, nil
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

func (t *testSession) Close(status uint32, reason string) error {
	t.closed = true
	return nil
}

func testNode() *Node {
	c := newTestConfig()
	n := New(&c)
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
	n := New(c)
	n.engine = NewTestEngine()
	return n
}

func newTestClient(n *Node, sess conns.Session) conns.ClientConn {
	c, _ := n.NewClient(sess, nil)
	return c
}

func createTestClients(n *Node, nChannels, nChannelClients int, sink chan []byte) {
	n.config.Insecure = true
	for i := 0; i < nChannelClients; i++ {
		sess := &testSession{}
		if sink != nil {
			sess.sink = sink
		}
		c := newTestClient(n, sess)
		cmd := proto.ConnectClientCommand{
			User: proto.UserID(fmt.Sprintf("user-%d", i)),
		}
		resp, err := c.(*client).connectCmd(&cmd)
		if err != nil {
			panic(err)
		}
		if resp.(*proto.ClientConnectResponse).ResponseError.Err != nil {
			panic(resp.(*proto.ClientConnectResponse).ResponseError.Err)
		}
		for j := 0; j < nChannels; j++ {
			cmd := proto.SubscribeClientCommand{
				Channel: proto.Channel(fmt.Sprintf("channel-%d", j)),
			}
			resp, err = c.(*client).subscribeCmd(&cmd)
			if err != nil {
				panic(err)
			}
			if resp.(*proto.ClientSubscribeResponse).ResponseError.Err != nil {
				panic(resp.(*proto.ClientSubscribeResponse).ResponseError.Err)
			}
		}
	}
}

func TestUserAllowed(t *testing.T) {
	app := testNode()
	assert.Equal(t, true, app.userAllowed("channel#1", "1"))
	assert.Equal(t, true, app.userAllowed("channel", "1"))
	assert.Equal(t, false, app.userAllowed("channel#1", "2"))
	assert.Equal(t, true, app.userAllowed("channel#1,2", "1"))
	assert.Equal(t, true, app.userAllowed("channel#1,2", "2"))
	assert.Equal(t, false, app.userAllowed("channel#1,2", "3"))
}

func TestSetConfig(t *testing.T) {
	app := testNode()
	c := newTestConfig()
	app.SetConfig(&c)
}

func TestAdminAuthToken(t *testing.T) {
	app := testNode()
	// first without secret set
	err := app.checkAdminAuthToken("")
	assert.Equal(t, ErrUnauthorized, err)

	// no secret set
	token, err := AdminAuthToken(app.config.AdminSecret)
	assert.Equal(t, ErrInternalServerError, err)

	app.Lock()
	app.config.AdminSecret = "secret"
	app.Unlock()

	err = app.checkAdminAuthToken("")
	assert.Equal(t, ErrUnauthorized, err)

	token, err = AdminAuthToken("secret")
	assert.Equal(t, nil, err)
	assert.True(t, len(token) > 0)
	err = app.checkAdminAuthToken(token)
	assert.Equal(t, nil, err)

}

func TestClientAllowed(t *testing.T) {
	app := testNode()
	assert.Equal(t, true, app.clientAllowed("channel&67330d48-f668-4916-758b-f4eb1dd5b41d", proto.ConnID("67330d48-f668-4916-758b-f4eb1dd5b41d")))
	assert.Equal(t, true, app.clientAllowed("channel", proto.ConnID("67330d48-f668-4916-758b-f4eb1dd5b41d")))
	assert.Equal(t, false, app.clientAllowed("channel&long-client-id", proto.ConnID("wrong-client-id")))
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
	info := app.node()
	assert.Equal(t, int64(0), info.Metrics["num_clients"])
	assert.NotEqual(t, 0, info.Started)
}

func BenchmarkNamespaceKey(b *testing.B) {
	app := testNode()
	ch := proto.Channel("test")
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
	assert.Equal(t, ErrInvalidMessage, err)
	err = app.ControlMsg(testUnsubscribeControlCmd("another node"))
	assert.Equal(t, nil, err)
	err = app.ControlMsg(testDisconnectControlCmd("another node"))
	assert.Equal(t, nil, err)
}

func TestUnsubscribe(t *testing.T) {
	app := testNode()
	c, err := app.NewClient(&testSession{}, nil)
	assert.Equal(t, nil, err)
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	cmds := []proto.ClientCommand{testConnectCmd(timestamp), testSubscribeCmd("test")}
	err = c.(*client).handleCommands(cmds)
	assert.Equal(t, nil, err)
	assert.Equal(t, 1, len(c.Channels()))
	app.unsubscribeUser(proto.UserID("user1"), proto.Channel("test"))
	assert.Equal(t, 0, len(c.Channels()))
}

func TestPublishJoinLeave(t *testing.T) {
	app := testNode()
	err := app.pubJoin(proto.Channel("channel-0"), proto.ClientInfo{})
	assert.Equal(t, nil, err)
	err = app.pubLeave(proto.Channel("channel-0"), proto.ClientInfo{})
	assert.Equal(t, nil, err)
}

func TestUpdateMetrics(t *testing.T) {
	app := testNode()
	data, _ := json.Marshal(map[string]string{"test": "publish"})
	err := app.Publish(proto.Channel("channel-0"), data, proto.ConnID(""), nil)
	assert.Equal(t, nil, err)

	config := app.Config()
	config.NodeMetricsInterval = 1 * time.Millisecond
	app.SetConfig(&config)

	app.updateMetricsOnce()

	// Absolute metrics should be updated
	assert.Equal(t, int64(1), metricsRegistry.Counters.LoadValues()["num_msg_published"])
}
