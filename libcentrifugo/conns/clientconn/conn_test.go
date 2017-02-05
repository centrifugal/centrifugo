package clientconn

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/libcentrifugo/auth"
	"github.com/centrifugal/centrifugo/libcentrifugo/conns"
	"github.com/centrifugal/centrifugo/libcentrifugo/engine/enginememory"
	"github.com/centrifugal/centrifugo/libcentrifugo/node"
	"github.com/centrifugal/centrifugo/libcentrifugo/proto"
	"github.com/stretchr/testify/assert"
)

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

func TestUnauthenticatedClient(t *testing.T) {
	app := NewTestNode()
	c, err := New(app, NewTestSession())
	assert.Equal(t, nil, err)
	assert.NotEqual(t, "", c.UID())

	// user not set before connect command success
	assert.Equal(t, "", c.User())

	assert.Equal(t, false, c.(*client).authenticated)
	assert.Equal(t, []string{}, c.Channels())

	// check that unauthenticated client can be cleaned correctly
	err = c.Close(nil)
	assert.Equal(t, nil, err)
}

func TestCloseUnauthenticatedClient(t *testing.T) {
	app := NewTestNode()
	conf := app.Config()
	conf.StaleConnectionCloseDelay = 50 * time.Microsecond
	app.SetConfig(&conf)
	c, err := New(app, NewTestSession())
	assert.Equal(t, nil, err)
	select {
	case <-c.(*client).closeCh:
		return
	case <-time.After(5 * time.Second):
		assert.True(t, false, "stale connection must be closed")
	}
}

func TestClientMessage(t *testing.T) {
	app := NewTestNode()
	c, err := New(app, NewTestSession())
	assert.Equal(t, nil, err)

	// empty message
	err = c.Handle([]byte{})
	assert.Equal(t, proto.ErrInvalidMessage, err)

	c, err = New(app, NewTestSession())
	assert.Equal(t, nil, err)

	// malformed message
	err = c.Handle([]byte("wroooong"))
	assert.NotEqual(t, nil, err)

	c, err = New(app, NewTestSession())
	assert.Equal(t, nil, err)

	// client request exceeds allowed size
	b := make([]byte, 1024*65)
	err = c.Handle(b)
	assert.Equal(t, proto.ErrLimitExceeded, err)

	c, err = New(app, NewTestSession())
	assert.Equal(t, nil, err)

	var cmds []proto.ClientCommand

	nonConnectFirstCmd := proto.ClientCommand{
		Method: "subscribe",
		Params: []byte("{}"),
	}

	cmds = append(cmds, nonConnectFirstCmd)
	cmdBytes, err := json.Marshal(cmds)
	assert.Equal(t, nil, err)
	err = c.Handle(cmdBytes)
	assert.Equal(t, proto.ErrUnauthorized, err)
}

func TestSingleObjectMessage(t *testing.T) {
	app := NewTestNode()
	c, err := New(app, NewTestSession())
	assert.Equal(t, nil, err)

	nonConnectFirstCmd := proto.ClientCommand{
		Method: "subscribe",
		Params: []byte("{}"),
	}

	cmdBytes, err := json.Marshal(nonConnectFirstCmd)
	assert.Equal(t, nil, err)
	err = c.Handle(cmdBytes)
	assert.Equal(t, proto.ErrUnauthorized, err)
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

func testRefreshCmd(timestamp string) proto.ClientCommand {
	token := auth.GenerateClientToken("secret", "user1", timestamp, "")
	refreshCmd := proto.RefreshClientCommand{
		Timestamp: timestamp,
		User:      string("user1"),
		Info:      "",
		Token:     token,
	}
	cmdBytes, _ := json.Marshal(refreshCmd)
	cmd := proto.ClientCommand{
		Method: "refresh",
		Params: cmdBytes,
	}
	return cmd
}

func testChannelSign(client string, ch string) string {
	return auth.GenerateChannelSign("secret", string(client), string(ch), "")
}

func testSubscribePrivateCmd(ch string, client string) proto.ClientCommand {
	subscribeCmd := proto.SubscribeClientCommand{
		Channel: string(ch),
		Client:  client,
		Info:    "",
		Sign:    testChannelSign(client, ch),
	}
	cmdBytes, _ := json.Marshal(subscribeCmd)
	cmd := proto.ClientCommand{
		Method: "subscribe",
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

func testUnsubscribeCmd(channel string) proto.ClientCommand {
	unsubscribeCmd := proto.UnsubscribeClientCommand{
		Channel: string(channel),
	}
	cmdBytes, _ := json.Marshal(unsubscribeCmd)
	cmd := proto.ClientCommand{
		Method: "unsubscribe",
		Params: cmdBytes,
	}
	return cmd
}

func testPresenceCmd(channel string) proto.ClientCommand {
	presenceCmd := proto.PresenceClientCommand{
		Channel: string(channel),
	}
	cmdBytes, _ := json.Marshal(presenceCmd)
	cmd := proto.ClientCommand{
		Method: "presence",
		Params: cmdBytes,
	}
	return cmd
}

func testHistoryCmd(channel string) proto.ClientCommand {
	historyCmd := proto.HistoryClientCommand{
		Channel: string(channel),
	}
	cmdBytes, _ := json.Marshal(historyCmd)
	cmd := proto.ClientCommand{
		Method: "history",
		Params: cmdBytes,
	}
	return cmd
}

func testPublishCmd(channel string) proto.ClientCommand {
	publishCmd := proto.PublishClientCommand{
		Channel: string(channel),
		Data:    []byte("{}"),
	}
	cmdBytes, _ := json.Marshal(publishCmd)
	cmd := proto.ClientCommand{
		Method: "publish",
		Params: cmdBytes,
	}
	return cmd
}

func testPingCmd() proto.ClientCommand {
	cmd := proto.ClientCommand{
		Method: "ping",
		Params: []byte("{}"),
	}
	return cmd
}

func TestClientConnect(t *testing.T) {
	app := NewTestNode()
	c, err := New(app, NewTestSession())
	assert.Equal(t, nil, err)

	var cmd proto.ClientCommand
	var cmds []proto.ClientCommand

	cmd = proto.ClientCommand{
		Method: "connect",
		Params: []byte(`{"project": "test1"}`),
	}
	cmds = []proto.ClientCommand{cmd}
	err = c.(*client).handleCommands(cmds)
	assert.Equal(t, proto.ErrInvalidToken, err)

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	cmds = []proto.ClientCommand{testConnectCmd(timestamp)}
	err = c.(*client).handleCommands(cmds)
	assert.Equal(t, nil, err)
	assert.Equal(t, true, c.(*client).authenticated)
	ts, err := strconv.Atoi(timestamp)
	assert.Equal(t, int64(ts), c.(*client).timestamp)

	clientInfo := c.(*client).info(string(""))
	assert.Equal(t, "user1", clientInfo.User)

	assert.Equal(t, 1, app.ClientHub().NumClients())

	assert.NotEqual(t, "", c.UID(), "uid must be already set")
	assert.NotEqual(t, "", c.User(), "user must be already set")

	err = c.Close(nil)
	assert.Equal(t, nil, err)

	assert.Equal(t, 0, app.ClientHub().NumClients())
}

func TestClientRefresh(t *testing.T) {
	app := NewTestNode()
	c, err := New(app, NewTestSession())
	assert.Equal(t, nil, err)

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	cmds := []proto.ClientCommand{testConnectCmd(timestamp), testSubscribeCmd("test")}
	err = c.(*client).handleCommands(cmds)
	assert.Equal(t, nil, err)

	cmds = []proto.ClientCommand{testRefreshCmd(timestamp)}
	err = c.(*client).handleCommands(cmds)
	assert.Equal(t, nil, err)
}

func TestClientPublish(t *testing.T) {
	app := NewTestNode()
	c, err := New(app, NewTestSession())
	assert.Equal(t, nil, err)

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	cmds := []proto.ClientCommand{testConnectCmd(timestamp), testSubscribeCmd("test")}
	err = c.(*client).handleCommands(cmds)
	assert.Equal(t, nil, err)

	cmd := testPublishCmd("not_subscribed_on_this")
	resp, err := c.(*client).handleCmd(cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, proto.ErrPermissionDenied, resp.(*proto.ClientPublishResponse).ResponseError.Err)

	cmd = testPublishCmd("test")
	resp, err = c.(*client).handleCmd(cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, resp.(*proto.ClientPublishResponse).ResponseError.Err)
}

func TestClientSubscribe(t *testing.T) {
	app := NewTestNode()
	c, err := New(app, NewTestSession())
	assert.Equal(t, nil, err)

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	cmds := []proto.ClientCommand{testConnectCmd(timestamp), testSubscribeCmd("test")}
	err = c.(*client).handleCommands(cmds)
	assert.Equal(t, nil, err)
	assert.Equal(t, 1, len(c.Channels()))

	assert.Equal(t, 1, app.ClientHub().NumChannels())
	assert.Equal(t, 1, len(c.Channels()))

	err = c.Close(nil)
	assert.Equal(t, nil, err)

	assert.Equal(t, 0, app.ClientHub().NumChannels())
}

func TestClientSubscribePrivate(t *testing.T) {
	app := NewTestNode()
	c, err := New(app, NewTestSession())
	assert.Equal(t, nil, err)

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	cmds := []proto.ClientCommand{testConnectCmd(timestamp)}
	_ = c.(*client).handleCommands(cmds)

	resp, err := c.(*client).handleCmd(testSubscribeCmd("$test"))
	assert.Equal(t, nil, err)
	assert.Equal(t, proto.ErrPermissionDenied, resp.(*proto.ClientSubscribeResponse).ResponseError.Err)

	resp, err = c.(*client).handleCmd(testSubscribePrivateCmd("$test", c.UID()))
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, resp.(*proto.ClientSubscribeResponse).ResponseError.Err)

}

func TestClientSubscribeLimits(t *testing.T) {
	app := NewTestNode()
	c, err := New(app, NewTestSession())
	assert.Equal(t, nil, err)

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	cmds := []proto.ClientCommand{testConnectCmd(timestamp)}
	err = c.(*client).handleCommands(cmds)
	assert.Equal(t, nil, err)

	// generate long channel and try to subscribe on it.
	b := make([]string, 1000)
	for i := 0; i < 1000; i++ {
		b[i] = "a"
	}
	ch := strings.Join(b, "")

	resp, err := c.(*client).subscribeCmd(&proto.SubscribeClientCommand{Channel: string(ch)})
	assert.Equal(t, nil, err)
	assert.Equal(t, proto.ErrLimitExceeded, resp.(*proto.ClientSubscribeResponse).ResponseError.Err)
	assert.Equal(t, 0, len(c.Channels()))

	conf := app.Config()
	conf.ClientChannelLimit = 10
	app.SetConfig(&conf)

	for i := 0; i < 10; i++ {
		resp, err := c.(*client).subscribeCmd(&proto.SubscribeClientCommand{Channel: string(fmt.Sprintf("test%d", i))})
		assert.Equal(t, nil, err)
		assert.Equal(t, nil, resp.(*proto.ClientSubscribeResponse).ResponseError.Err)
		assert.Equal(t, i+1, len(c.Channels()))
	}

	// one more to exceed limit.
	resp, err = c.(*client).subscribeCmd(&proto.SubscribeClientCommand{Channel: string("test")})
	assert.Equal(t, nil, err)
	assert.Equal(t, proto.ErrLimitExceeded, resp.(*proto.ClientSubscribeResponse).ResponseError.Err)
	assert.Equal(t, 10, len(c.Channels()))

}

func TestClientUnsubscribe(t *testing.T) {
	app := NewTestNode()
	c, err := New(app, NewTestSession())
	assert.Equal(t, nil, err)

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	cmds := []proto.ClientCommand{testConnectCmd(timestamp), testSubscribeCmd("test")}
	err = c.(*client).handleCommands(cmds)
	assert.Equal(t, nil, err)

	cmds = []proto.ClientCommand{testUnsubscribeCmd("test")}
	err = c.(*client).handleCommands(cmds)
	assert.Equal(t, nil, err)

	cmds = []proto.ClientCommand{testSubscribeCmd("test"), testUnsubscribeCmd("test")}
	err = c.(*client).handleCommands(cmds)
	assert.Equal(t, nil, err)

	assert.Equal(t, 0, app.ClientHub().NumChannels())
}

func TestClientUnsubscribeExternal(t *testing.T) {
	app := NewTestNode()
	c, err := New(app, NewTestSession())
	assert.Equal(t, nil, err)

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	cmds := []proto.ClientCommand{testConnectCmd(timestamp), testSubscribeCmd("test")}
	err = c.(*client).handleCommands(cmds)
	assert.Equal(t, nil, err)

	err = c.(*client).Unsubscribe(string("test"))
	assert.Equal(t, nil, err)
	assert.Equal(t, 0, app.ClientHub().NumChannels())
	assert.Equal(t, 0, len(c.Channels()))
}

func TestClientPresence(t *testing.T) {
	app := NewTestNode()
	c, err := New(app, NewTestSession())
	assert.Equal(t, nil, err)

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	cmds := []proto.ClientCommand{testConnectCmd(timestamp)}
	err = c.(*client).handleCommands(cmds)
	assert.Equal(t, nil, err)

	resp, err := c.(*client).handleCmd(testPresenceCmd("test"))
	assert.Equal(t, nil, err)
	assert.Equal(t, proto.ErrPermissionDenied, resp.(*proto.ClientPresenceResponse).ResponseError.Err)

	_, _ = c.(*client).handleCmd(testSubscribeCmd("test"))
	resp, err = c.(*client).handleCmd(testPresenceCmd("test"))
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, resp.(*proto.ClientPresenceResponse).ResponseError.Err)
}

func TestClientUpdatePresence(t *testing.T) {
	app := NewTestNode()
	c, err := New(app, NewTestSession())
	assert.Equal(t, nil, err)

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	cmds := []proto.ClientCommand{testConnectCmd(timestamp), testSubscribeCmd("test1"), testSubscribeCmd("test2")}
	err = c.(*client).handleCommands(cmds)
	assert.Equal(t, nil, err)
	assert.Equal(t, 2, len(c.Channels()))

	assert.NotEqual(t, nil, c.(*client).presenceTimer)
	timer := c.(*client).presenceTimer
	c.(*client).updatePresence()
	assert.NotEqual(t, timer, c.(*client).presenceTimer)
}

func TestClientHistory(t *testing.T) {
	app := NewTestNode()
	c, err := New(app, NewTestSession())
	assert.Equal(t, nil, err)

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	cmds := []proto.ClientCommand{testConnectCmd(timestamp)}
	err = c.(*client).handleCommands(cmds)
	assert.Equal(t, nil, err)

	resp, err := c.(*client).handleCmd(testHistoryCmd("test"))
	assert.Equal(t, nil, err)
	assert.Equal(t, proto.ErrPermissionDenied, resp.(*proto.ClientHistoryResponse).ResponseError.Err)

	_, _ = c.(*client).handleCmd(testSubscribeCmd("test"))
	resp, err = c.(*client).handleCmd(testHistoryCmd("test"))
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, resp.(*proto.ClientHistoryResponse).ResponseError.Err)
}

func TestClientPing(t *testing.T) {
	app := NewTestNode()
	c, err := New(app, NewTestSession())
	assert.Equal(t, nil, err)

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	cmds := []proto.ClientCommand{testConnectCmd(timestamp)}
	err = c.(*client).handleCommands(cmds)
	assert.Equal(t, nil, err)

	resp, err := c.(*client).handleCmd(testPingCmd())
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, resp.(*proto.ClientPingResponse).ResponseError.Err)
}

func testSubscribeRecoverCmd(channel string, last string, rec bool) proto.ClientCommand {
	subscribeCmd := proto.SubscribeClientCommand{
		Channel: string(channel),
		Last:    last,
		Recover: rec,
	}
	cmdBytes, _ := json.Marshal(subscribeCmd)
	cmd := proto.ClientCommand{
		Method: "subscribe",
		Params: cmdBytes,
	}
	return cmd
}

func TestSubscribeRecover(t *testing.T) {
	app := NewTestMemoryNode()
	conf := app.Config()
	conf.Recover = true
	conf.HistoryLifetime = 30
	conf.HistorySize = 5
	app.SetConfig(&conf)

	c, err := New(app, NewTestSession())
	assert.Equal(t, nil, err)

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	cmds := []proto.ClientCommand{testConnectCmd(timestamp), testSubscribeCmd("test")}
	err = c.(*client).handleCommands(cmds)
	assert.Equal(t, nil, err)

	data, _ := json.Marshal(map[string]string{"input": "test"})

	msg := proto.NewMessage("test", data, "", nil)
	err = <-app.Publish(msg, nil)
	assert.Equal(t, nil, err)

	messages, _ := app.History("test")
	assert.Equal(t, 1, len(messages))
	message := messages[0]
	last := message.UID

	// test setting last message uid when no uid provided
	c, _ = New(app, NewTestSession())
	cmds = []proto.ClientCommand{testConnectCmd(timestamp)}
	err = c.(*client).handleCommands(cmds)
	assert.Equal(t, nil, err)
	subscribeCmd := testSubscribeCmd("test")
	resp, err := c.(*client).handleCmd(subscribeCmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, last, resp.(*proto.ClientSubscribeResponse).Body.Last)

	// publish 2 messages since last
	data, _ = json.Marshal(map[string]string{"input": "test1"})
	msg = proto.NewMessage("test", data, "", nil)
	err = <-app.Publish(msg, nil)
	assert.Equal(t, nil, err)
	data, _ = json.Marshal(map[string]string{"input": "test2"})
	msg = proto.NewMessage("test", data, "", nil)
	err = <-app.Publish(msg, nil)
	assert.Equal(t, nil, err)

	// test no messages recovered when recover is false in subscribe cmd
	c, _ = New(app, NewTestSession())
	cmds = []proto.ClientCommand{testConnectCmd(timestamp)}
	err = c.(*client).handleCommands(cmds)
	assert.Equal(t, nil, err)
	subscribeLastCmd := testSubscribeRecoverCmd("test", last, false)
	resp, err = c.(*client).handleCmd(subscribeLastCmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, 0, len(resp.(*proto.ClientSubscribeResponse).Body.Messages))
	assert.NotEqual(t, last, resp.(*proto.ClientSubscribeResponse).Body.Last)

	// test normal recover
	c, _ = New(app, NewTestSession())
	cmds = []proto.ClientCommand{testConnectCmd(timestamp)}
	err = c.(*client).handleCommands(cmds)
	assert.Equal(t, nil, err)
	subscribeLastCmd = testSubscribeRecoverCmd("test", last, true)
	resp, err = c.(*client).handleCmd(subscribeLastCmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, 2, len(resp.(*proto.ClientSubscribeResponse).Body.Messages))
	assert.Equal(t, true, resp.(*proto.ClientSubscribeResponse).Body.Recovered)
	assert.Equal(t, "", resp.(*proto.ClientSubscribeResponse).Body.Last)
	messages = resp.(*proto.ClientSubscribeResponse).Body.Messages
	m0, _ := json.Marshal(messages[0].Data)
	m1, _ := json.Marshal(messages[1].Data)
	// in reversed order in history
	assert.Equal(t, strings.Contains(string(m0), "test2"), true)
	assert.Equal(t, strings.Contains(string(m1), "test1"), true)

	// test part recover - when Centrifugo can not recover all missed messages
	for i := 0; i < 10; i++ {
		data, _ = json.Marshal(map[string]string{"input": "test1"})
		msg = proto.NewMessage("test", data, "", nil)
		err = <-app.Publish(msg, nil)
		assert.Equal(t, nil, err)
	}
	c, _ = New(app, NewTestSession())
	cmds = []proto.ClientCommand{testConnectCmd(timestamp)}
	err = c.(*client).handleCommands(cmds)
	assert.Equal(t, nil, err)
	subscribeLastCmd = testSubscribeRecoverCmd("test", last, true)
	resp, err = c.(*client).handleCmd(subscribeLastCmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, 5, len(resp.(*proto.ClientSubscribeResponse).Body.Messages))
	assert.Equal(t, false, resp.(*proto.ClientSubscribeResponse).Body.Recovered)
}
