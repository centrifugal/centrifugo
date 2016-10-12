package node

import (
	"encoding/json"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/libcentrifugo/engine"
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
	n, _ := New(&c)
	n.engine = engine.NewTestEngine()
	return n
}

func testNodeWithConfig(c *Config) *Node {
	if c == nil {
		conf := newTestConfig()
		c = &conf
	}
	n, _ := New(c)
	n.engine = engine.NewTestEngine()
	return n
}

func newTestClient(n *Node, sess session) *client {
	c, _ := newClient(n, sess)
	return c
}

func createTestClients(n *Node, nChannels, nChannelClients int, sink chan []byte) {
	n.config.Insecure = true
	for i := 0; i < nChannelClients; i++ {
		sess := &testSession{}
		if sink != nil {
			sess.sink = sink
		}
		c := newTestClient(app, sess)
		cmd := connectClientCommand{
			User: UserID(fmt.Sprintf("user-%d", i)),
		}
		resp, err := c.connectCmd(&cmd)
		if err != nil {
			panic(err)
		}
		if resp.(*clientConnectResponse).err != nil {
			panic(resp.(*clientConnectResponse).err)
		}
		for j := 0; j < nChannels; j++ {
			cmd := subscribeClientCommand{
				Channel: Channel(fmt.Sprintf("channel-%d", j)),
			}
			resp, err = c.subscribeCmd(&cmd)
			if err != nil {
				panic(err)
			}
			if resp.(*clientSubscribeResponse).err != nil {
				panic(resp.(*clientSubscribeResponse).err)
			}
		}
	}
}

func testMemoryAppWithClients(nChannels int, nChannelClients int) *Application {
	app := testMemoryApp()
	createTestClients(app, nChannels, nChannelClients, nil)
	return app
}

func TestUserAllowed(t *testing.T) {
	app := testApp()
	assert.Equal(t, true, app.userAllowed("channel#1", "1"))
	assert.Equal(t, true, app.userAllowed("channel", "1"))
	assert.Equal(t, false, app.userAllowed("channel#1", "2"))
	assert.Equal(t, true, app.userAllowed("channel#1,2", "1"))
	assert.Equal(t, true, app.userAllowed("channel#1,2", "2"))
	assert.Equal(t, false, app.userAllowed("channel#1,2", "3"))
}

func TestSetConfig(t *testing.T) {
	app := testApp()
	c := newTestConfig()
	app.SetConfig(&c)
}

func TestAdminAuthToken(t *testing.T) {
	app := testApp()
	// first without secret set
	err := app.checkAdminAuthToken("")
	assert.Equal(t, ErrUnauthorized, err)

	// no web secret set
	token, err := app.adminAuthToken()
	assert.Equal(t, ErrInternalServerError, err)

	app.Lock()
	app.config.AdminSecret = "secret"
	app.Unlock()

	err = app.checkAdminAuthToken("")
	assert.Equal(t, ErrUnauthorized, err)

	token, err = app.adminAuthToken()
	assert.Equal(t, nil, err)
	assert.True(t, len(token) > 0)
	err = app.checkAdminAuthToken(token)
	assert.Equal(t, nil, err)

}

func TestClientAllowed(t *testing.T) {
	app := testApp()
	assert.Equal(t, true, app.clientAllowed("channel&67330d48-f668-4916-758b-f4eb1dd5b41d", ConnID("67330d48-f668-4916-758b-f4eb1dd5b41d")))
	assert.Equal(t, true, app.clientAllowed("channel", ConnID("67330d48-f668-4916-758b-f4eb1dd5b41d")))
	assert.Equal(t, false, app.clientAllowed("channel&long-client-id", ConnID("wrong-client-id")))
}

func TestNamespaceKey(t *testing.T) {
	app := testApp()
	assert.Equal(t, NamespaceKey("ns"), app.namespaceKey("ns:channel"))
	assert.Equal(t, NamespaceKey(""), app.namespaceKey("channel"))
	assert.Equal(t, NamespaceKey("ns"), app.namespaceKey("ns:channel:opa"))
	assert.Equal(t, NamespaceKey("ns"), app.namespaceKey("ns::channel"))
}

func TestApplicationNode(t *testing.T) {
	app := testApp()
	info := app.node()
	assert.Equal(t, 0, info.Clients)
	assert.NotEqual(t, 0, info.Started)
}

func BenchmarkNamespaceKey(b *testing.B) {
	app := testApp()
	ch := Channel("test")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		app.namespaceKey(ch)
	}
}

func testPingControlCmd(uid string) *ControlMessage {
	return newControlMessage(uid, "ping", []byte("{}"))
}

func testUnsubscribeControlCmd(uid string) *ControlMessage {
	return newControlMessage(uid, "unsubscribe", []byte("{}"))
}

func testDisconnectControlCmd(uid string) *ControlMessage {
	return newControlMessage(uid, "disconnect", []byte("{}"))
}

func testWrongControlCmd(uid string) *ControlMessage {
	return newControlMessage(uid, "wrong", []byte("{}"))
}

func TestPublish(t *testing.T) {
	// Custom config
	c := newTestConfig()

	// Set custom options for default namespace
	c.ChannelOptions.HistoryLifetime = 10
	c.ChannelOptions.HistorySize = 2
	c.ChannelOptions.HistoryDropInactive = true

	app := testMemoryAppWithConfig(&c)
	createTestClients(app, 10, 1, nil)
	data, _ := json.Marshal(map[string]string{"test": "publish"})
	err := app.Publish(Channel("channel-0"), data, ConnID(""), nil)
	assert.Nil(t, err)

	// Check publish to subscribed channels did result in saved history
	hist, err := app.History(Channel("channel-0"))
	assert.Nil(t, err)
	assert.Equal(t, 1, len(hist))

	// Publishing to a channel no one is subscribed to should be a no-op
	err = app.Publish(Channel("some-other-channel"), data, ConnID(""), nil)
	assert.Nil(t, err)

	hist, err = app.History(Channel("some-other-channel"))
	assert.Nil(t, err)
	assert.Equal(t, 0, len(hist))

}

func TestPublishJoinLeave(t *testing.T) {
	app := testMemoryApp()
	createTestClients(app, 10, 1, nil)
	err := app.pubJoin(Channel("channel-0"), ClientInfo{})
	assert.Equal(t, nil, err)
	err = app.pubLeave(Channel("channel-0"), ClientInfo{})
	assert.Equal(t, nil, err)
}

func TestControlMessages(t *testing.T) {
	app := testApp()
	app.Run()
	// command from this node
	cmd := testPingControlCmd(app.uid)
	err := app.controlMsg(cmd)
	assert.Equal(t, nil, err)
	cmd = testPingControlCmd("another_node")
	err = app.controlMsg(cmd)
	assert.Equal(t, nil, err)
	err = app.controlMsg(testWrongControlCmd("another node"))
	assert.Equal(t, ErrInvalidMessage, err)
	err = app.controlMsg(testUnsubscribeControlCmd("another node"))
	assert.Equal(t, nil, err)
	err = app.controlMsg(testDisconnectControlCmd("another node"))
	assert.Equal(t, nil, err)
}

func TestUpdateMetrics(t *testing.T) {
	app := testMemoryApp()
	createTestClients(app, 10, 1, nil)
	data, _ := json.Marshal(map[string]string{"test": "publish"})
	err := app.Publish(Channel("channel-0"), data, ConnID(""), nil)
	assert.Equal(t, nil, err)
	app.config.NodeMetricsInterval = 1 * time.Millisecond
	app.updateMetricsOnce()

	// Absolute metrics should be updated
	assert.Equal(t, int64(1), app.metrics.NumMsgPublished.LoadRaw())
}

func TestUnsubscribe(t *testing.T) {
	app := testApp()
	c, err := newClient(app, &testSession{})
	assert.Equal(t, nil, err)
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	cmds := []clientCommand{testConnectCmd(timestamp), testSubscribeCmd("test")}
	err = c.handleCommands(cmds)
	assert.Equal(t, nil, err)
	assert.Equal(t, 1, len(c.channels()))
	app.unsubscribeUser(UserID("user1"), Channel("test"))
	assert.Equal(t, 0, len(c.channels()))
}

// BenchmarkPubSubMessageReceive allows to estimate how many new messages we can convert to client JSON messages.
func BenchmarkPubSubMessageReceive(b *testing.B) {
	app := testMemoryApp()

	// create one client so clientMsg really marshal into client response JSON.
	c, _ := newClient(app, &testSession{})

	messagePoolSize := 1000

	messagePool := make([][]byte, messagePoolSize)

	for i := 0; i < len(messagePool); i++ {
		channel := Channel("test" + strconv.Itoa(i))
		// subscribe client to channel so we need to encode message to JSON
		app.clients.addSub(channel, c)
		// add message to pool so we have messages for different channels.
		testMsg := newMessage(channel, []byte("{\"hello world\": true}"), "", nil)
		byteMessage, _ := testMsg.Marshal() // protobuf
		messagePool[i] = byteMessage
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var msg Message
		err := msg.Unmarshal(messagePool[i%len(messagePool)]) // unmarshal from protobuf
		if err != nil {
			panic(err)
		}
		err = app.clientMsg(Channel("test"+strconv.Itoa(i%len(messagePool))), &msg)
		if err != nil {
			panic(err)
		}
	}
}

// BenchmarkClientMsg allows to measue performance of marshaling messages into client response JSON.
func BenchmarkClientMsg(b *testing.B) {
	app := testMemoryApp()
	// create one client so clientMsg really marshal into client response JSON.
	c, _ := newClient(app, &testSession{})
	messagePoolSize := 1000
	messagePool := make([]*Message, messagePoolSize)

	for i := 0; i < len(messagePool); i++ {
		channel := Channel("test" + strconv.Itoa(i))
		// subscribe client to channel so we need to encode message to JSON
		app.clients.addSub(channel, c)
		// add message to pool so we have messages for different channels.
		testMsg := newMessage(channel, []byte("{\"hello world\": true}"), "", nil)
		messagePool[i] = testMsg
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := app.clientMsg(Channel("test"+strconv.Itoa(i%len(messagePool))), messagePool[i%len(messagePool)])
		if err != nil {
			panic(err)
		}
	}
}

// BenchmarkEngineMessageUnmarshal shows how fast we can decode messages coming from engine PUB/SUB.
func BenchmarkEngineMessageUnmarshal(b *testing.B) {
	messagePoolSize := 1000
	messagePool := make([][]byte, messagePoolSize)

	for i := 0; i < len(messagePool); i++ {
		channel := Channel("test" + strconv.Itoa(i))
		// add message to pool so we have messages for different channels.
		testMsg := newMessage(channel, []byte("{\"hello world\": true}"), "", nil)
		byteMessage, _ := testMsg.Marshal() // protobuf
		messagePool[i] = byteMessage
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var msg Message
		err := msg.Unmarshal(messagePool[i%len(messagePool)]) // unmarshal from protobuf
		if err != nil {
			panic(err)
		}
	}
}

// BenchmarkReceiveBroadcast measures how fast we can broadcast messages received
// from engine into client channels in case of reasonably large different channel
// amount.
func BenchmarkReceiveBroadcast(b *testing.B) {
	nChannels := 1000
	nClients := 1000
	nCommands := 10000
	nMessages := nCommands * nClients
	sink := make(chan []byte, nMessages)
	app := testMemoryApp()
	// Use very large initial capacity so that queue resizes do not affect benchmark.
	app.config.ClientQueueInitialCapacity = 4000
	app.config.ClientChannelLimit = 1000
	createTestClients(app, nChannels, nClients, sink)

	type received struct {
		ch   Channel
		data Message
	}

	var inputData []received

	for i := 0; i < nCommands; i++ {
		suffix := i % nChannels
		ch := Channel(fmt.Sprintf("channel-%d", suffix))
		msg := newMessage(ch, []byte("{}"), "", nil)
		inputData = append(inputData, received{ch, *msg})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {

		done := make(chan struct{})

		go func() {
			count := 0
			for {
				select {
				case <-sink:
					count++
				}
				if count == nMessages {
					close(done)
					return
				}
			}
		}()

		go func() {
			for _, item := range inputData {
				app.clientMsg(item.ch, &item.data)
			}
		}()

		<-done
	}
	b.StopTimer()
}
