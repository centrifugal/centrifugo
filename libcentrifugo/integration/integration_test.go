package integration

import (
	"encoding/json"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/libcentrifugo/api/v1"
	"github.com/centrifugal/centrifugo/libcentrifugo/auth"
	"github.com/centrifugal/centrifugo/libcentrifugo/conns"
	"github.com/centrifugal/centrifugo/libcentrifugo/conns/clientconn"
	"github.com/centrifugal/centrifugo/libcentrifugo/engine/enginememory"
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
		Watch:           false,
		Publish:         true,
		Presence:        true,
		HistorySize:     10,
		HistoryLifetime: 10,
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
	e, _ := enginememory.New(n, nil)
	err := n.Run(&node.RunOptions{Engine: e})
	if err != nil {
		panic(err)
	}
	return n
}

func NewTestMemoryNodeWithConfig(c *node.Config) *node.Node {
	n := node.New("", c)
	e, _ := enginememory.New(n, nil)
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
	c, _ := clientconn.New(n, sess)
	return c
}

func createTestClients(n *node.Node, nChannels, nChannelClients int, sink chan []byte) {
	config := n.Config()
	config.Insecure = true
	n.SetConfig(&config)

	// prepare subscribe commands.
	subscribeBytes := make([][]byte, nChannels)
	for j := 0; j < nChannels; j++ {
		subscribeBytes[j] = []byte(`{"method": "subscribe", "params": {"channel": "` + fmt.Sprintf("channel-%d", j) + `"}}`)
	}

	for i := 0; i < nChannelClients; i++ {
		sess := NewTestSession()
		if sink != nil {
			sess.sink = sink
		}
		c := newTestClient(n, sess)

		connectBytes := []byte(`{"method": "connect", "params": {"user": "` + fmt.Sprintf("user-%d", i) + `"}}`)

		err := c.Handle(connectBytes)
		if err != nil {
			panic(err)
		}
		for j := 0; j < nChannels; j++ {
			err := c.Handle(subscribeBytes[j])
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
	c, _ := clientconn.New(app, NewTestSession())

	messagePoolSize := 1000

	messagePool := make([][]byte, messagePoolSize)

	for i := 0; i < len(messagePool); i++ {
		channel := string("test" + strconv.Itoa(i))
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
		err = app.ClientMsg(&msg)
		if err != nil {
			panic(err)
		}
	}
}

func testConnectCmd(timestamp string) proto.ClientCommand {
	token := auth.GenerateClientToken("secret", "user1", timestamp, "")
	connectCmd := proto.ConnectClientCommand{
		Timestamp: timestamp,
		User:      string("user1"),
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
		Channel: string(channel),
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
	c, err := clientconn.New(app, NewTestSession())
	assert.Equal(t, nil, err)
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	cmds := []proto.ClientCommand{testConnectCmd(timestamp), testSubscribeCmd("test")}
	cmdBytes, _ := json.Marshal(cmds)
	err = c.Handle(cmdBytes)
	assert.Equal(t, nil, err)
	assert.Equal(t, 1, len(c.Channels()))
	app.Unsubscribe(string("user1"), string("test"))
	assert.Equal(t, 0, len(c.Channels()))
}

// BenchmarkClientMsg allows to measue performance of marshaling messages into client response JSON.
func BenchmarkClientMsg(b *testing.B) {
	app := NewTestMemoryNode()
	// create one client so clientMsg really marshal into client response JSON.
	c, _ := clientconn.New(app, NewTestSession())
	messagePoolSize := 1000
	messagePool := make([]*proto.Message, messagePoolSize)

	for i := 0; i < len(messagePool); i++ {
		channel := string("test" + strconv.Itoa(i))
		// subscribe client to channel so we need to encode message to JSON
		app.ClientHub().AddSub(channel, c)
		// add message to pool so we have messages for different channels.
		testMsg := proto.NewMessage(channel, []byte("{\"hello world\": true}"), "", nil)
		messagePool[i] = testMsg
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := app.ClientMsg(messagePool[i%len(messagePool)])
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
		channel := string("test" + strconv.Itoa(i))
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
		ch   string
		data *proto.Message
	}

	var inputData []received

	for i := 0; i < nCommands; i++ {
		suffix := i % nChannels
		ch := string(fmt.Sprintf("channel-%d", suffix))
		msg := proto.NewMessage(ch, []byte("{}"), "", nil)
		inputData = append(inputData, received{ch, msg})
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
				app.ClientMsg(item.data)
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
	err := <-app.Publish(proto.NewMessage("channel-0", data, "", nil), nil)
	assert.Nil(t, err)

	// Check publish to subscribed channels did result in saved history
	hist, err := app.History("channel-0")
	assert.Nil(t, err)
	assert.Equal(t, 1, len(hist))

	// Publishing to a channel no one is subscribed to should be a no-op
	err = <-app.Publish(proto.NewMessage("some-other-channel", data, "", nil), nil)
	assert.Nil(t, err)

	hist, err = app.History("some-other-channel")
	assert.Nil(t, err)
	assert.Equal(t, 0, len(hist))
}

func TestAPIChannels(t *testing.T) {
	app := NewTestMemoryNode()
	createTestClients(app, 10, 1, nil)
	resp, err := apiv1.ChannelsCmd(app)
	assert.Equal(t, nil, err)
	body := resp.(*proto.APIChannelsResponse).Body
	assert.Equal(t, 10, len(body.Data))
}

func getNPublishJSON(channel string, n int) []byte {
	commands := make([]map[string]interface{}, n)
	command := map[string]interface{}{
		"method": "publish",
		"params": map[string]interface{}{
			"channel": channel,
			"data":    map[string]bool{"benchmarking": true},
		},
	}
	for i := 0; i < n; i++ {
		commands[i] = command
	}
	jsonData, _ := json.Marshal(commands)
	return jsonData
}

func getPublishJSON(channel string) []byte {
	commands := make([]map[string]interface{}, 1)
	command := map[string]interface{}{
		"method": "publish",
		"params": map[string]interface{}{
			"channel": channel,
			"data":    map[string]bool{"benchmarking": true},
		},
	}
	commands[0] = command
	jsonData, _ := json.Marshal(commands)
	return jsonData
}

func getNChannelsBroadcastJSON(n int) []byte {
	channels := make([]string, n)
	for i := 0; i < n; i++ {
		channels[i] = fmt.Sprintf("channel-%d", i)
	}
	commands := make([]map[string]interface{}, 1)
	command := map[string]interface{}{
		"method": "broadcast",
		"params": map[string]interface{}{
			"channels": channels,
			"data":     map[string]bool{"benchmarking": true},
		},
	}
	commands[0] = command
	jsonData, _ := json.Marshal(commands)
	return jsonData
}

func getManyNChannelsBroadcastJSON(nChannels int, nCommands int) []byte {
	channels := make([]string, nChannels)
	for i := 0; i < nChannels; i++ {
		channels[i] = fmt.Sprintf("channel-%d", i)
	}
	commands := make([]map[string]interface{}, nCommands)
	command := map[string]interface{}{
		"method": "broadcast",
		"params": map[string]interface{}{
			"channels": channels,
			"data":     map[string]bool{"benchmarking": true},
		},
	}
	for i := 0; i < nCommands; i++ {
		commands[i] = command
	}
	jsonData, _ := json.Marshal(commands)
	return jsonData
}

// BenchmarkAPIRequestPublish allows to bench processing API request data containing single
// publish command.
func BenchmarkAPIRequestPublish(b *testing.B) {
	app := NewTestMemoryNode()
	jsonData := getPublishJSON("channel")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := apiv1.ProcessAPIData(app, jsonData)
		if err != nil {
			b.Error(err)
		}
	}
}

// BenchmarkAPIRequestPublishParallel allows to bench processing API request data containing single
// publish command running in parallel.
func BenchmarkAPIRequestPublishParallel(b *testing.B) {
	app := NewTestMemoryNode()
	jsonData := getPublishJSON("channel")
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := apiv1.ProcessAPIData(app, jsonData)
			if err != nil {
				b.Error(err)
			}
		}
	})
}

// BenchmarkAPIRequestPublishMany allows to bench processing API request data containing many
// publish commands as array.
func BenchmarkAPIRequestPublishMany(b *testing.B) {
	app := NewTestMemoryNode()
	jsonData := getNPublishJSON("channel", 1000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := apiv1.ProcessAPIData(app, jsonData)
		if err != nil {
			b.Error(err)
		}
	}
}

// BenchmarkAPIRequestPublishManyParallel allows to bench processing API request data containing many
// publish commands as array.
func BenchmarkAPIRequestPublishManyParallel(b *testing.B) {
	app := NewTestMemoryNode()
	jsonData := getNPublishJSON("channel", 1000)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := apiv1.ProcessAPIData(app, jsonData)
			if err != nil {
				b.Error(err)
			}
		}
	})
}

// BenchmarkAPIRequestBroadcast allows to bench processing API request data containing single
// broadcast command into many channels.
func BenchmarkAPIRequestBroadcast(b *testing.B) {
	app := NewTestMemoryNode()
	jsonData := getNChannelsBroadcastJSON(1000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := apiv1.ProcessAPIData(app, jsonData)
		if err != nil {
			b.Error(err)
		}
	}
}

// BenchmarkAPIRequestBroadcastMany allows to bench processing API request data containing many
// broadcast commands into many channels.
func BenchmarkAPIRequestBroadcastMany(b *testing.B) {
	app := NewTestMemoryNode()
	jsonData := getManyNChannelsBroadcastJSON(100, 100)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := apiv1.ProcessAPIData(app, jsonData)
		if err != nil {
			b.Error(err)
		}
	}
}
