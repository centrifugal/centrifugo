package node

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/libcentrifugo/auth"
	"github.com/centrifugal/centrifugo/libcentrifugo/proto"
	"github.com/stretchr/testify/assert"
)

func TestUnauthenticatedClient(t *testing.T) {
	app := testNode()
	c, err := app.NewClient(&testSession{}, nil)
	assert.Equal(t, nil, err)
	assert.NotEqual(t, "", c.UID())

	// user not set before connect command success
	assert.Equal(t, proto.UserID(""), c.User())

	assert.Equal(t, false, c.(*client).authenticated)
	assert.Equal(t, []proto.Channel{}, c.Channels())

	// check that unauthenticated client can be cleaned correctly
	err = c.Close("")
	assert.Equal(t, nil, err)
}

func TestCloseUnauthenticatedClient(t *testing.T) {
	app := testNode()
	app.config.StaleConnectionCloseDelay = 50 * time.Microsecond
	c, err := app.NewClient(&testSession{}, nil)
	assert.Equal(t, nil, err)
	assert.NotEqual(t, "", c.UID())
	time.Sleep(time.Millisecond)
	assert.Equal(t, true, c.(*client).messages.Closed())
}

func TestClientMessage(t *testing.T) {
	app := testNode()
	c, err := app.NewClient(&testSession{}, nil)
	assert.Equal(t, nil, err)

	// empty message
	err = c.Handle([]byte{})
	assert.Equal(t, ErrInvalidMessage, err)

	// malformed message
	err = c.Handle([]byte("wroooong"))
	assert.NotEqual(t, nil, err)

	// client request exceeds allowed size
	b := make([]byte, 1024*65)
	err = c.Handle(b)
	assert.Equal(t, ErrLimitExceeded, err)

	var cmds []proto.ClientCommand

	nonConnectFirstCmd := proto.ClientCommand{
		Method: "subscribe",
		Params: []byte("{}"),
	}

	cmds = append(cmds, nonConnectFirstCmd)
	cmdBytes, err := json.Marshal(cmds)
	assert.Equal(t, nil, err)
	err = c.Handle(cmdBytes)
	assert.Equal(t, ErrUnauthorized, err)
}

func TestSingleObjectMessage(t *testing.T) {
	app := testNode()
	c, err := app.NewClient(&testSession{}, nil)
	assert.Equal(t, nil, err)

	nonConnectFirstCmd := proto.ClientCommand{
		Method: "subscribe",
		Params: []byte("{}"),
	}

	cmdBytes, err := json.Marshal(nonConnectFirstCmd)
	assert.Equal(t, nil, err)
	err = c.Handle(cmdBytes)
	assert.Equal(t, ErrUnauthorized, err)
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

func testRefreshCmd(timestamp string) proto.ClientCommand {
	token := auth.GenerateClientToken("secret", "user1", timestamp, "")
	refreshCmd := proto.RefreshClientCommand{
		Timestamp: timestamp,
		User:      proto.UserID("user1"),
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

func testChannelSign(client proto.ConnID, ch proto.Channel) string {
	return auth.GenerateChannelSign("secret", string(client), string(ch), "")
}

func testSubscribePrivateCmd(ch proto.Channel, client proto.ConnID) proto.ClientCommand {
	subscribeCmd := proto.SubscribeClientCommand{
		Channel: proto.Channel(ch),
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
		Channel: proto.Channel(channel),
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
		Channel: proto.Channel(channel),
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
		Channel: proto.Channel(channel),
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
		Channel: proto.Channel(channel),
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
		Channel: proto.Channel(channel),
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
	app := testNode()
	c, err := app.NewClient(&testSession{}, nil)
	assert.Equal(t, nil, err)

	var cmd proto.ClientCommand
	var cmds []proto.ClientCommand

	cmd = proto.ClientCommand{
		Method: "connect",
		Params: []byte(`{"project": "test1"}`),
	}
	cmds = []proto.ClientCommand{cmd}
	err = c.(*client).handleCommands(cmds)
	assert.Equal(t, ErrInvalidToken, err)

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	cmds = []proto.ClientCommand{testConnectCmd(timestamp)}
	err = c.(*client).handleCommands(cmds)
	assert.Equal(t, nil, err)
	assert.Equal(t, true, c.(*client).authenticated)
	ts, err := strconv.Atoi(timestamp)
	assert.Equal(t, int64(ts), c.(*client).timestamp)

	clientInfo := c.(*client).info(proto.Channel(""))
	assert.Equal(t, "user1", clientInfo.User)

	assert.Equal(t, 1, app.clients.NumClients())

	assert.NotEqual(t, "", c.UID(), "uid must be already set")
	assert.NotEqual(t, "", c.User(), "user must be already set")

	err = c.Close("")
	assert.Equal(t, nil, err)

	assert.Equal(t, 0, app.clients.NumClients())
}

func TestClientRefresh(t *testing.T) {
	app := testNode()
	c, err := app.NewClient(&testSession{}, nil)
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
	app := testNode()
	c, err := app.NewClient(&testSession{}, nil)
	assert.Equal(t, nil, err)

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	cmds := []proto.ClientCommand{testConnectCmd(timestamp), testSubscribeCmd("test")}
	err = c.(*client).handleCommands(cmds)
	assert.Equal(t, nil, err)

	cmd := testPublishCmd("not_subscribed_on_this")
	resp, err := c.(*client).handleCmd(cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, ErrPermissionDenied, resp.(*proto.ClientPublishResponse).ResponseError.Err)

	cmd = testPublishCmd("test")
	resp, err = c.(*client).handleCmd(cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, resp.(*proto.ClientPublishResponse).ResponseError.Err)
}

func TestClientSubscribe(t *testing.T) {
	app := testNode()
	c, err := app.NewClient(&testSession{}, nil)
	assert.Equal(t, nil, err)

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	cmds := []proto.ClientCommand{testConnectCmd(timestamp), testSubscribeCmd("test")}
	err = c.(*client).handleCommands(cmds)
	assert.Equal(t, nil, err)
	assert.Equal(t, 1, len(c.Channels()))

	assert.Equal(t, 1, app.clients.NumChannels())
	assert.Equal(t, 1, len(c.Channels()))

	err = c.Close("")
	assert.Equal(t, nil, err)

	assert.Equal(t, 0, app.clients.NumChannels())
}

func TestClientSubscribePrivate(t *testing.T) {
	app := testNode()
	c, err := app.NewClient(&testSession{}, nil)
	assert.Equal(t, nil, err)

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	cmds := []proto.ClientCommand{testConnectCmd(timestamp)}
	_ = c.(*client).handleCommands(cmds)

	resp, err := c.(*client).handleCmd(testSubscribeCmd("$test"))
	assert.Equal(t, nil, err)
	assert.Equal(t, ErrPermissionDenied, resp.(*proto.ClientSubscribeResponse).ResponseError.Err)

	resp, err = c.(*client).handleCmd(testSubscribePrivateCmd("$test", c.UID()))
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, resp.(*proto.ClientSubscribeResponse).ResponseError.Err)

}

func TestClientSubscribeLimits(t *testing.T) {
	app := testNode()
	c, err := app.NewClient(&testSession{}, nil)
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

	resp, err := c.(*client).subscribeCmd(&proto.SubscribeClientCommand{Channel: proto.Channel(ch)})
	assert.Equal(t, nil, err)
	assert.Equal(t, ErrLimitExceeded, resp.(*proto.ClientSubscribeResponse).ResponseError.Err)
	assert.Equal(t, 0, len(c.Channels()))

	c.(*client).app.config.ClientChannelLimit = 10

	for i := 0; i < 10; i++ {
		resp, err := c.(*client).subscribeCmd(&proto.SubscribeClientCommand{Channel: proto.Channel(fmt.Sprintf("test%d", i))})
		assert.Equal(t, nil, err)
		assert.Equal(t, nil, resp.(*proto.ClientSubscribeResponse).ResponseError.Err)
		assert.Equal(t, i+1, len(c.Channels()))
	}

	// one more to exceed limit.
	resp, err = c.(*client).subscribeCmd(&proto.SubscribeClientCommand{Channel: proto.Channel("test")})
	assert.Equal(t, nil, err)
	assert.Equal(t, ErrLimitExceeded, resp.(*proto.ClientSubscribeResponse).ResponseError.Err)
	assert.Equal(t, 10, len(c.Channels()))

}

func TestClientUnsubscribe(t *testing.T) {
	app := testNode()
	c, err := app.NewClient(&testSession{}, nil)
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

	assert.Equal(t, 0, app.clients.NumChannels())
}

func TestClientUnsubscribeExternal(t *testing.T) {
	app := testNode()
	c, err := app.NewClient(&testSession{}, nil)
	assert.Equal(t, nil, err)

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	cmds := []proto.ClientCommand{testConnectCmd(timestamp), testSubscribeCmd("test")}
	err = c.(*client).handleCommands(cmds)
	assert.Equal(t, nil, err)

	err = c.(*client).Unsubscribe(proto.Channel("test"))
	assert.Equal(t, nil, err)
	assert.Equal(t, 0, app.clients.NumChannels())
	assert.Equal(t, 0, len(c.Channels()))
}

func TestClientPresence(t *testing.T) {
	app := testNode()
	c, err := app.NewClient(&testSession{}, nil)
	assert.Equal(t, nil, err)

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	cmds := []proto.ClientCommand{testConnectCmd(timestamp)}
	err = c.(*client).handleCommands(cmds)
	assert.Equal(t, nil, err)

	resp, err := c.(*client).handleCmd(testPresenceCmd("test"))
	assert.Equal(t, nil, err)
	assert.Equal(t, ErrPermissionDenied, resp.(*proto.ClientPresenceResponse).ResponseError.Err)

	_, _ = c.(*client).handleCmd(testSubscribeCmd("test"))
	resp, err = c.(*client).handleCmd(testPresenceCmd("test"))
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, resp.(*proto.ClientPresenceResponse).ResponseError.Err)
}

func TestClientUpdatePresence(t *testing.T) {
	app := testNode()
	c, err := app.NewClient(&testSession{}, nil)
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
	app := testNode()
	c, err := app.NewClient(&testSession{}, nil)
	assert.Equal(t, nil, err)

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	cmds := []proto.ClientCommand{testConnectCmd(timestamp)}
	err = c.(*client).handleCommands(cmds)
	assert.Equal(t, nil, err)

	resp, err := c.(*client).handleCmd(testHistoryCmd("test"))
	assert.Equal(t, nil, err)
	assert.Equal(t, ErrPermissionDenied, resp.(*proto.ClientHistoryResponse).ResponseError.Err)

	_, _ = c.(*client).handleCmd(testSubscribeCmd("test"))
	resp, err = c.(*client).handleCmd(testHistoryCmd("test"))
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, resp.(*proto.ClientHistoryResponse).ResponseError.Err)
}

func TestClientPing(t *testing.T) {
	app := testNode()
	c, err := app.NewClient(&testSession{}, nil)
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
		Channel: proto.Channel(channel),
		Last:    proto.MessageID(last),
		Recover: rec,
	}
	cmdBytes, _ := json.Marshal(subscribeCmd)
	cmd := proto.ClientCommand{
		Method: "subscribe",
		Params: cmdBytes,
	}
	return cmd
}

/*
func TestSubscribeRecover(t *testing.T) {
	app := testMemoryApp()
	app.config.Recover = true
	app.config.HistoryLifetime = 30
	app.config.HistorySize = 5

	c, err := app.NewClient(&testSession{}, nil)
	assert.Equal(t, nil, err)

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	cmds := []clientCommand{testConnectCmd(timestamp), testSubscribeCmd("test")}
	err = c.(*client).handleCommands(cmds)
	assert.Equal(t, nil, err)

	data, _ := json.Marshal(map[string]string{"input": "test"})
	err = app.Publish(Channel("test"), data, ConnID(""), nil)
	assert.Equal(t, nil, err)
	assert.Equal(t, int64(1), metricsRegistry.Counters.LoadRaw("num_msg_published"))

	messages, _ := app.History(proto.Channel("test"))
	assert.Equal(t, 1, len(messages))
	message := messages[0]
	last := message.UID

	// test setting last message uid when no uid provided
	c, _ = app.NewClient(&testSession{}, nil)
	cmds = []proto.ClientCommand{testConnectCmd(timestamp)}
	err = c.(*client).handleCommands(cmds)
	assert.Equal(t, nil, err)
	subscribeCmd := testSubscribeCmd("test")
	resp, err := c.(*client).handleCmd(subscribeCmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, last, string(resp.(*proto.ClientSubscribeResponse).Body.Last))

	// publish 2 messages since last
	data, _ = json.Marshal(map[string]string{"input": "test1"})
	err = app.Publish(Channel("test"), data, ConnID(""), nil)
	assert.Equal(t, nil, err)
	data, _ = json.Marshal(map[string]string{"input": "test2"})
	err = app.Publish(Channel("test"), data, ConnID(""), nil)
	assert.Equal(t, nil, err)

	assert.Equal(t, int64(3), app.metrics.NumMsgPublished.LoadRaw())

	// test no messages recovered when recover is false in subscribe cmd
	c, _ = app.NewClient(&testSession{}, nil)
	cmds = []proto.ClientCommand{testConnectCmd(timestamp)}
	err = c.(*client).handleCommands(cmds)
	assert.Equal(t, nil, err)
	subscribeLastCmd := testSubscribeRecoverCmd("test", last, false)
	resp, err = c.(*client).handleCmd(subscribeLastCmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, 0, len(resp.(*proto.ClientSubscribeResponse).Body.Messages))
	assert.NotEqual(t, last, resp.(*proto.ClientSubscribeResponse).Body.Last)

	// test normal recover
	c, _ = app.NewClient(&testSession{}, nil)
	cmds = []proto.ClientCommand{testConnectCmd(timestamp)}
	err = c.(*client).handleCommands(cmds)
	assert.Equal(t, nil, err)
	subscribeLastCmd = testSubscribeRecoverCmd("test", last, true)
	resp, err = c.(*client).handleCmd(subscribeLastCmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, 2, len(resp.(*proto.ClientSubscribeResponse).Body.Messages))
	assert.Equal(t, true, resp.(*proto.ClientSubscribeResponse).Body.Recovered)
	assert.Equal(t, MessageID(""), resp.(*proto.ClientSubscribeResponse).Body.Last)
	messages = resp.(*proto.ClientSubscribeResponse).Body.Messages
	m0, _ := json.Marshal(messages[0].Data)
	m1, _ := json.Marshal(messages[1].Data)
	// in reversed order in history
	assert.Equal(t, strings.Contains(string(m0), "test2"), true)
	assert.Equal(t, strings.Contains(string(m1), "test1"), true)

	// test part recover - when Centrifugo can not recover all missed messages
	for i := 0; i < 10; i++ {
		data, _ = json.Marshal(map[string]string{"input": "test1"})
		err = app.Publish(proto.Channel("test"), data, proto.ConnID(""), nil)
		assert.Equal(t, nil, err)
	}
	c, _ = app.NewClient(&testSession{}, nil)
	cmds = []proto.ClientCommand{testConnectCmd(timestamp)}
	err = c.(*client).handleCommands(cmds)
	assert.Equal(t, nil, err)
	subscribeLastCmd = testSubscribeRecoverCmd("test", last, true)
	resp, err = c.(*client).handleCmd(subscribeLastCmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, 5, len(resp.(*proto.ClientSubscribeResponse).Body.Messages))
	assert.Equal(t, false, resp.(*proto.ClientSubscribeResponse).Body.Recovered)
}
*/
