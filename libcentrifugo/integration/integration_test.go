package integration

import (
	"encoding/json"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/libcentrifugo/engine/enginememory"
	//"github.com/centrifugal/centrifugo/libcentrifugo/engine/engineredis"
	"github.com/centrifugal/centrifugo/libcentrifugo/auth"
	"github.com/centrifugal/centrifugo/libcentrifugo/conns"
	"github.com/centrifugal/centrifugo/libcentrifugo/conns/clientconn"
	"github.com/centrifugal/centrifugo/libcentrifugo/node"
	"github.com/centrifugal/centrifugo/libcentrifugo/proto"
	"github.com/centrifugal/centrifugo/libcentrifugo/raw"
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

func NewTestMemoryNode() *node.Node {
	c := NewTestConfig()
	n := node.New("", c)
	e, _ := enginememory.NewMemoryEngine(n, nil)
	err := n.Run(&node.RunOptions{Engine: e})
	if err != nil {
		panic(err)
	}
	return n
}

func NewTestMemoryNodeWithConfig(c *node.Config) *node.Node {
	n := node.New("", c)
	e, _ := enginememory.NewMemoryEngine(n, nil)
	err := n.Run(&node.RunOptions{Engine: e})
	if err != nil {
		panic(err)
	}
	return n
}

func testMemoryNodeWithClients(nChannels int, nChannelClients int) *node.Node {
	n := NewTestMemoryNode()
	createTestClients(n, nChannels, nChannelClients, nil)
	return n
}

func newTestClient(n *node.Node, sess conns.Session) conns.ClientConn {
	c, _ := clientconn.New(n, sess, nil)
	return c
}

func createTestClients(n *node.Node, nChannels, nChannelClients int, sink chan []byte) {
	config := n.Config()
	config.Insecure = true
	n.SetConfig(&config)
	for i := 0; i < nChannelClients; i++ {
		sess := NewTestSession()
		if sink != nil {
			sess.sink = sink
		}
		c := newTestClient(n, sess)
		body := proto.ConnectClientCommand{
			User: proto.UserID(fmt.Sprintf("user-%d", i)),
		}
		bodyBytes, _ := json.Marshal(body)
		rawBytes := raw.Raw(bodyBytes)
		cmd := proto.ClientCommand{
			Method: "connect",
			Params: rawBytes,
		}
		cmdBytes, _ := json.Marshal(&cmd)
		err := c.Handle(cmdBytes)
		if err != nil {
			panic(err)
		}
		for j := 0; j < nChannels; j++ {

			body := proto.SubscribeClientCommand{
				Channel: proto.Channel(fmt.Sprintf("channel-%d", j)),
			}
			bodyBytes, _ := json.Marshal(body)
			cmd := proto.ClientCommand{
				Method: "subscribe",
				Params: raw.Raw(bodyBytes),
			}
			cmdBytes, _ := json.Marshal(&cmd)

			err := c.Handle(cmdBytes)
			if err != nil {
				panic(err)
			}
		}
	}
}

// BenchmarkPubSubMessageReceive allows to estimate how many new messages we can convert to client JSON messages.
func BenchmarkPubSubMessageReceive(b *testing.B) {
	app := NewTestMemoryNode()

	// create one client so clientMsg really marshal into client response JSON.
	c, _ := clientconn.New(app, NewTestSession(), nil)

	messagePoolSize := 1000

	messagePool := make([][]byte, messagePoolSize)

	for i := 0; i < len(messagePool); i++ {
		channel := proto.Channel("test" + strconv.Itoa(i))
		// subscribe client to channel so we need to encode message to JSON
		app.ClientHub().AddSub(channel, c)
		// add message to pool so we have messages for different channels.
		testMsg := proto.NewMessage(channel, []byte("{\"hello world\": true}"), "", nil)
		byteMessage, _ := testMsg.Marshal() // protobuf
		messagePool[i] = byteMessage
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var msg proto.Message
		err := msg.Unmarshal(messagePool[i%len(messagePool)]) // unmarshal from protobuf
		if err != nil {
			panic(err)
		}
		err = app.ClientMsg(proto.Channel("test"+strconv.Itoa(i%len(messagePool))), &msg)
		if err != nil {
			panic(err)
		}
	}
}

func testConnectCmd(timestamp string) proto.ClientCommand {
	token := auth.GenerateClientToken("secret", "user1", timestamp, "")
	connectCmd := proto.ConnectClientCommand{
		Timestamp: timestamp,
		User:      proto.UserID("user1"),
		Info:      "",
		Token:     token,
	}
	cmdBytes, _ := json.Marshal(connectCmd)
	cmd := proto.ClientCommand{
		Method: "connect",
		Params: cmdBytes,
	}
	return cmd
}

func testSubscribeCmd(channel string) proto.ClientCommand {
	subscribeCmd := proto.SubscribeClientCommand{
		Channel: proto.Channel(channel),
	}
	cmdBytes, _ := json.Marshal(subscribeCmd)
	cmd := proto.ClientCommand{
		Method: "subscribe",
		Params: cmdBytes,
	}
	return cmd
}

func TestUnsubscribe(t *testing.T) {
	app := NewTestNode()
	c, err := clientconn.New(app, NewTestSession(), nil)
	assert.Equal(t, nil, err)
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	cmds := []proto.ClientCommand{testConnectCmd(timestamp), testSubscribeCmd("test")}
	cmdBytes, _ := json.Marshal(cmds)
	err = c.Handle(cmdBytes)
	assert.Equal(t, nil, err)
	assert.Equal(t, 1, len(c.Channels()))
	app.Unsubscribe(proto.UserID("user1"), proto.Channel("test"))
	assert.Equal(t, 0, len(c.Channels()))
}

// BenchmarkClientMsg allows to measue performance of marshaling messages into client response JSON.
func BenchmarkClientMsg(b *testing.B) {
	app := NewTestMemoryNode()
	// create one client so clientMsg really marshal into client response JSON.
	c, _ := clientconn.New(app, NewTestSession(), nil)
	messagePoolSize := 1000
	messagePool := make([]*proto.Message, messagePoolSize)

	for i := 0; i < len(messagePool); i++ {
		channel := proto.Channel("test" + strconv.Itoa(i))
		// subscribe client to channel so we need to encode message to JSON
		app.ClientHub().AddSub(channel, c)
		// add message to pool so we have messages for different channels.
		testMsg := proto.NewMessage(channel, []byte("{\"hello world\": true}"), "", nil)
		messagePool[i] = testMsg
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := app.ClientMsg(proto.Channel("test"+strconv.Itoa(i%len(messagePool))), messagePool[i%len(messagePool)])
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
		channel := proto.Channel("test" + strconv.Itoa(i))
		// add message to pool so we have messages for different channels.
		testMsg := proto.NewMessage(channel, []byte("{\"hello world\": true}"), "", nil)
		byteMessage, _ := testMsg.Marshal() // protobuf
		messagePool[i] = byteMessage
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var msg proto.Message
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
	app := NewTestMemoryNode()
	// Use very large initial capacity so that queue resizes do not affect benchmark.
	config := app.Config()
	config.ClientQueueInitialCapacity = 4000
	config.ClientChannelLimit = 1000
	app.SetConfig(&config)
	createTestClients(app, nChannels, nClients, sink)

	type received struct {
		ch   proto.Channel
		data proto.Message
	}

	var inputData []received

	for i := 0; i < nCommands; i++ {
		suffix := i % nChannels
		ch := proto.Channel(fmt.Sprintf("channel-%d", suffix))
		msg := proto.NewMessage(ch, []byte("{}"), "", nil)
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
				app.ClientMsg(item.ch, &item.data)
			}
		}()

		<-done
	}
	b.StopTimer()
}

func TestPublish(t *testing.T) {
	// Custom config
	c := NewTestConfig()

	// Set custom options for default namespace
	c.ChannelOptions.HistoryLifetime = 10
	c.ChannelOptions.HistorySize = 2
	c.ChannelOptions.HistoryDropInactive = true

	app := NewTestMemoryNodeWithConfig(c)
	createTestClients(app, 10, 1, nil)
	data, _ := json.Marshal(map[string]string{"test": "publish"})
	err := app.Publish(proto.Channel("channel-0"), data, proto.ConnID(""), nil)
	assert.Nil(t, err)

	// Check publish to subscribed channels did result in saved history
	hist, err := app.History(proto.Channel("channel-0"))
	assert.Nil(t, err)
	assert.Equal(t, 1, len(hist))

	// Publishing to a channel no one is subscribed to should be a no-op
	err = app.Publish(proto.Channel("some-other-channel"), data, proto.ConnID(""), nil)
	assert.Nil(t, err)

	hist, err = app.History(proto.Channel("some-other-channel"))
	assert.Nil(t, err)
	assert.Equal(t, 0, len(hist))
}
