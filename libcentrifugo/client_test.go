package libcentrifugo

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/libcentrifugo/auth"
	"github.com/stretchr/testify/assert"
)

func TestUnauthenticatedClient(t *testing.T) {
	app := testApp()
	c, err := newClient(app, &testSession{})
	assert.Equal(t, nil, err)
	assert.NotEqual(t, "", c.uid())

	// user not set before connect command success
	assert.Equal(t, UserID(""), c.user())

	assert.Equal(t, false, c.authenticated)
	assert.Equal(t, []Channel{}, c.channels())

	// check that unauthenticated client can be cleaned correctly
	err = c.clean()
	assert.Equal(t, nil, err)
}

func TestCloseUnauthenticatedClient(t *testing.T) {
	app := testApp()
	app.config.StaleConnectionCloseDelay = 50 * time.Microsecond
	c, err := newClient(app, &testSession{})
	assert.Equal(t, nil, err)
	assert.NotEqual(t, "", c.uid())
	time.Sleep(time.Millisecond)
	assert.Equal(t, true, c.messages.Closed())
}

func TestClientMessage(t *testing.T) {
	app := testApp()
	c, err := newClient(app, &testSession{})
	assert.Equal(t, nil, err)

	// empty message
	err = c.message([]byte{})
	assert.Equal(t, ErrInvalidMessage, err)

	// malformed message
	err = c.message([]byte("wroooong"))
	assert.NotEqual(t, nil, err)

	// client request exceeds allowed size
	b := make([]byte, 1024*65)
	err = c.message(b)
	assert.Equal(t, ErrLimitExceeded, err)

	var cmds []clientCommand

	nonConnectFirstCmd := clientCommand{
		Method: "subscribe",
		Params: []byte("{}"),
	}

	cmds = append(cmds, nonConnectFirstCmd)
	cmdBytes, err := json.Marshal(cmds)
	assert.Equal(t, nil, err)
	err = c.message(cmdBytes)
	assert.Equal(t, ErrUnauthorized, err)
}

func TestSingleObjectMessage(t *testing.T) {
	app := testApp()
	c, err := newClient(app, &testSession{})
	assert.Equal(t, nil, err)

	nonConnectFirstCmd := clientCommand{
		Method: "subscribe",
		Params: []byte("{}"),
	}

	cmdBytes, err := json.Marshal(nonConnectFirstCmd)
	assert.Equal(t, nil, err)
	err = c.message(cmdBytes)
	assert.Equal(t, ErrUnauthorized, err)
}

func testConnectCmd(timestamp string) clientCommand {
	token := auth.GenerateClientToken("secret", "user1", timestamp, "")
	connectCmd := connectClientCommand{
		Timestamp: timestamp,
		User:      UserID("user1"),
		Info:      "",
		Token:     token,
	}
	cmdBytes, _ := json.Marshal(connectCmd)
	cmd := clientCommand{
		Method: "connect",
		Params: cmdBytes,
	}
	return cmd
}

func testRefreshCmd(timestamp string) clientCommand {
	token := auth.GenerateClientToken("secret", "user1", timestamp, "")
	refreshCmd := refreshClientCommand{
		Timestamp: timestamp,
		User:      UserID("user1"),
		Info:      "",
		Token:     token,
	}
	cmdBytes, _ := json.Marshal(refreshCmd)
	cmd := clientCommand{
		Method: "refresh",
		Params: cmdBytes,
	}
	return cmd
}

func testChannelSign(client ConnID, ch Channel) string {
	return auth.GenerateChannelSign("secret", string(client), string(ch), "")
}

func testSubscribePrivateCmd(ch Channel, client ConnID) clientCommand {
	subscribeCmd := subscribeClientCommand{
		Channel: Channel(ch),
		Client:  client,
		Info:    "",
		Sign:    testChannelSign(client, ch),
	}
	cmdBytes, _ := json.Marshal(subscribeCmd)
	cmd := clientCommand{
		Method: "subscribe",
		Params: cmdBytes,
	}
	return cmd
}

func testSubscribeCmd(channel string) clientCommand {
	subscribeCmd := subscribeClientCommand{
		Channel: Channel(channel),
	}
	cmdBytes, _ := json.Marshal(subscribeCmd)
	cmd := clientCommand{
		Method: "subscribe",
		Params: cmdBytes,
	}
	return cmd
}

func testUnsubscribeCmd(channel string) clientCommand {
	unsubscribeCmd := unsubscribeClientCommand{
		Channel: Channel(channel),
	}
	cmdBytes, _ := json.Marshal(unsubscribeCmd)
	cmd := clientCommand{
		Method: "unsubscribe",
		Params: cmdBytes,
	}
	return cmd
}

func testPresenceCmd(channel string) clientCommand {
	presenceCmd := presenceClientCommand{
		Channel: Channel(channel),
	}
	cmdBytes, _ := json.Marshal(presenceCmd)
	cmd := clientCommand{
		Method: "presence",
		Params: cmdBytes,
	}
	return cmd
}

func testHistoryCmd(channel string) clientCommand {
	historyCmd := historyClientCommand{
		Channel: Channel(channel),
	}
	cmdBytes, _ := json.Marshal(historyCmd)
	cmd := clientCommand{
		Method: "history",
		Params: cmdBytes,
	}
	return cmd
}

func testPublishCmd(channel string) clientCommand {
	publishCmd := publishClientCommand{
		Channel: Channel(channel),
		Data:    []byte("{}"),
	}
	cmdBytes, _ := json.Marshal(publishCmd)
	cmd := clientCommand{
		Method: "publish",
		Params: cmdBytes,
	}
	return cmd
}

func testPingCmd() clientCommand {
	cmd := clientCommand{
		Method: "ping",
		Params: []byte("{}"),
	}
	return cmd
}

func TestClientConnect(t *testing.T) {
	app := testApp()
	c, err := newClient(app, &testSession{})
	assert.Equal(t, nil, err)

	var cmd clientCommand
	var cmds []clientCommand

	cmd = clientCommand{
		Method: "connect",
		Params: []byte(`{"project": "test1"}`),
	}
	cmds = []clientCommand{cmd}
	err = c.handleCommands(cmds)
	assert.Equal(t, ErrInvalidToken, err)

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	cmds = []clientCommand{testConnectCmd(timestamp)}
	err = c.handleCommands(cmds)
	assert.Equal(t, nil, err)
	assert.Equal(t, true, c.authenticated)
	ts, err := strconv.Atoi(timestamp)
	assert.Equal(t, int64(ts), c.timestamp)

	clientInfo := c.info(Channel(""))
	assert.Equal(t, "user1", clientInfo.User)

	assert.Equal(t, 1, len(app.clients.conns))

	assert.NotEqual(t, "", c.uid(), "uid must be already set")
	assert.NotEqual(t, "", c.user(), "user must be already set")

	err = c.clean()
	assert.Equal(t, nil, err)

	assert.Equal(t, 0, len(app.clients.conns))
}

func TestClientRefresh(t *testing.T) {
	app := testApp()
	c, err := newClient(app, &testSession{})
	assert.Equal(t, nil, err)

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	cmds := []clientCommand{testConnectCmd(timestamp), testSubscribeCmd("test")}
	err = c.handleCommands(cmds)
	assert.Equal(t, nil, err)

	cmds = []clientCommand{testRefreshCmd(timestamp)}
	err = c.handleCommands(cmds)
	assert.Equal(t, nil, err)
}

func TestClientPublish(t *testing.T) {
	app := testApp()
	c, err := newClient(app, &testSession{})
	assert.Equal(t, nil, err)

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	cmds := []clientCommand{testConnectCmd(timestamp), testSubscribeCmd("test")}
	err = c.handleCommands(cmds)
	assert.Equal(t, nil, err)

	cmd := testPublishCmd("not_subscribed_on_this")
	resp, err := c.handleCmd(cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, ErrPermissionDenied, resp.(*clientPublishResponse).err)

	cmd = testPublishCmd("test")
	resp, err = c.handleCmd(cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, resp.(*clientPublishResponse).err)
}

func TestClientSubscribe(t *testing.T) {
	app := testApp()
	c, err := newClient(app, &testSession{})
	assert.Equal(t, nil, err)

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	cmds := []clientCommand{testConnectCmd(timestamp), testSubscribeCmd("test")}
	err = c.handleCommands(cmds)
	assert.Equal(t, nil, err)
	assert.Equal(t, 1, len(c.channels()))

	assert.Equal(t, 1, len(app.clients.subs))
	assert.Equal(t, 1, len(c.channels()))

	err = c.clean()
	assert.Equal(t, nil, err)

	assert.Equal(t, 0, len(app.clients.subs))
}

func TestClientSubscribePrivate(t *testing.T) {
	app := testApp()
	c, err := newClient(app, &testSession{})
	assert.Equal(t, nil, err)

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	cmds := []clientCommand{testConnectCmd(timestamp)}
	_ = c.handleCommands(cmds)

	resp, err := c.handleCmd(testSubscribeCmd("$test"))
	assert.Equal(t, nil, err)
	assert.Equal(t, ErrPermissionDenied, resp.(*clientSubscribeResponse).err)

	resp, err = c.handleCmd(testSubscribePrivateCmd("$test", c.UID))
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, resp.(*clientSubscribeResponse).err)

}

func TestClientSubscribeLimits(t *testing.T) {
	app := testApp()
	c, err := newClient(app, &testSession{})
	assert.Equal(t, nil, err)

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	cmds := []clientCommand{testConnectCmd(timestamp)}
	err = c.handleCommands(cmds)
	assert.Equal(t, nil, err)

	// generate long channel and try to subscribe on it.
	b := make([]string, 1000)
	for i := 0; i < 1000; i++ {
		b[i] = "a"
	}
	ch := strings.Join(b, "")

	resp, err := c.subscribeCmd(&subscribeClientCommand{Channel: Channel(ch)})
	assert.Equal(t, nil, err)
	assert.Equal(t, ErrLimitExceeded, resp.(*clientSubscribeResponse).err)
	assert.Equal(t, 0, len(c.channels()))

	c.app.config.ClientChannelLimit = 10

	for i := 0; i < 10; i++ {
		resp, err := c.subscribeCmd(&subscribeClientCommand{Channel: Channel(fmt.Sprintf("test%d", i))})
		assert.Equal(t, nil, err)
		assert.Equal(t, nil, resp.(*clientSubscribeResponse).err)
		assert.Equal(t, i+1, len(c.channels()))
	}

	// one more to exceed limit.
	resp, err = c.subscribeCmd(&subscribeClientCommand{Channel: Channel("test")})
	assert.Equal(t, nil, err)
	assert.Equal(t, ErrLimitExceeded, resp.(*clientSubscribeResponse).err)
	assert.Equal(t, 10, len(c.channels()))

}

func TestClientUnsubscribe(t *testing.T) {
	app := testApp()
	c, err := newClient(app, &testSession{})
	assert.Equal(t, nil, err)

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	cmds := []clientCommand{testConnectCmd(timestamp), testSubscribeCmd("test")}
	err = c.handleCommands(cmds)
	assert.Equal(t, nil, err)

	cmds = []clientCommand{testUnsubscribeCmd("test")}
	err = c.handleCommands(cmds)
	assert.Equal(t, nil, err)

	cmds = []clientCommand{testSubscribeCmd("test"), testUnsubscribeCmd("test")}
	err = c.handleCommands(cmds)
	assert.Equal(t, nil, err)

	assert.Equal(t, 0, len(app.clients.subs))
}

func TestClientUnsubscribeExternal(t *testing.T) {
	app := testApp()
	c, err := newClient(app, &testSession{})
	assert.Equal(t, nil, err)

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	cmds := []clientCommand{testConnectCmd(timestamp), testSubscribeCmd("test")}
	err = c.handleCommands(cmds)
	assert.Equal(t, nil, err)

	err = c.unsubscribe(Channel("test"))
	assert.Equal(t, nil, err)
	assert.Equal(t, 0, len(app.clients.subs))
	assert.Equal(t, 0, len(c.channels()))
}

func TestClientPresence(t *testing.T) {
	app := testApp()
	c, err := newClient(app, &testSession{})
	assert.Equal(t, nil, err)

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	cmds := []clientCommand{testConnectCmd(timestamp)}
	err = c.handleCommands(cmds)
	assert.Equal(t, nil, err)

	resp, err := c.handleCmd(testPresenceCmd("test"))
	assert.Equal(t, nil, err)
	assert.Equal(t, ErrPermissionDenied, resp.(*clientPresenceResponse).err)

	_, _ = c.handleCmd(testSubscribeCmd("test"))
	resp, err = c.handleCmd(testPresenceCmd("test"))
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, resp.(*clientPresenceResponse).err)
}

func TestClientUpdatePresence(t *testing.T) {
	app := testApp()
	c, err := newClient(app, &testSession{})
	assert.Equal(t, nil, err)

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	cmds := []clientCommand{testConnectCmd(timestamp), testSubscribeCmd("test1"), testSubscribeCmd("test2")}
	err = c.handleCommands(cmds)
	assert.Equal(t, nil, err)
	assert.Equal(t, 2, len(c.channels()))

	assert.NotEqual(t, nil, c.presenceTimer)
	timer := c.presenceTimer
	c.updatePresence()
	assert.NotEqual(t, timer, c.presenceTimer)
}

func TestClientHistory(t *testing.T) {
	app := testApp()
	c, err := newClient(app, &testSession{})
	assert.Equal(t, nil, err)

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	cmds := []clientCommand{testConnectCmd(timestamp)}
	err = c.handleCommands(cmds)
	assert.Equal(t, nil, err)

	resp, err := c.handleCmd(testHistoryCmd("test"))
	assert.Equal(t, nil, err)
	assert.Equal(t, ErrPermissionDenied, resp.(*clientHistoryResponse).err)

	_, _ = c.handleCmd(testSubscribeCmd("test"))
	resp, err = c.handleCmd(testHistoryCmd("test"))
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, resp.(*clientHistoryResponse).err)
}

func TestClientPing(t *testing.T) {
	app := testApp()
	c, err := newClient(app, &testSession{})
	assert.Equal(t, nil, err)

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	cmds := []clientCommand{testConnectCmd(timestamp)}
	err = c.handleCommands(cmds)
	assert.Equal(t, nil, err)

	resp, err := c.handleCmd(testPingCmd())
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, resp.(*clientPingResponse).err)
}

func testSubscribeRecoverCmd(channel string, last string, rec bool) clientCommand {
	subscribeCmd := subscribeClientCommand{
		Channel: Channel(channel),
		Last:    MessageID(last),
		Recover: rec,
	}
	cmdBytes, _ := json.Marshal(subscribeCmd)
	cmd := clientCommand{
		Method: "subscribe",
		Params: cmdBytes,
	}
	return cmd
}

func TestSubscribeRecover(t *testing.T) {
	app := testMemoryApp()
	app.config.Recover = true
	app.config.HistoryLifetime = 30
	app.config.HistorySize = 5

	c, err := newClient(app, &testSession{})
	assert.Equal(t, nil, err)

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	cmds := []clientCommand{testConnectCmd(timestamp), testSubscribeCmd("test")}
	err = c.handleCommands(cmds)
	assert.Equal(t, nil, err)

	data, _ := json.Marshal(map[string]string{"input": "test"})
	err = app.Publish(Channel("test"), data, ConnID(""), nil)
	assert.Equal(t, nil, err)
	assert.Equal(t, int64(1), app.metrics.NumMsgPublished.LoadRaw())

	messages, _ := app.History(Channel("test"))
	assert.Equal(t, 1, len(messages))
	message := messages[0]
	last := message.UID

	// test setting last message uid when no uid provided
	c, _ = newClient(app, &testSession{})
	cmds = []clientCommand{testConnectCmd(timestamp)}
	err = c.handleCommands(cmds)
	assert.Equal(t, nil, err)
	subscribeCmd := testSubscribeCmd("test")
	resp, err := c.handleCmd(subscribeCmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, last, string(resp.(*clientSubscribeResponse).Body.Last))

	// publish 2 messages since last
	data, _ = json.Marshal(map[string]string{"input": "test1"})
	err = app.Publish(Channel("test"), data, ConnID(""), nil)
	assert.Equal(t, nil, err)
	data, _ = json.Marshal(map[string]string{"input": "test2"})
	err = app.Publish(Channel("test"), data, ConnID(""), nil)
	assert.Equal(t, nil, err)

	assert.Equal(t, int64(3), app.metrics.NumMsgPublished.LoadRaw())

	// test no messages recovered when recover is false in subscribe cmd
	c, _ = newClient(app, &testSession{})
	cmds = []clientCommand{testConnectCmd(timestamp)}
	err = c.handleCommands(cmds)
	assert.Equal(t, nil, err)
	subscribeLastCmd := testSubscribeRecoverCmd("test", last, false)
	resp, err = c.handleCmd(subscribeLastCmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, 0, len(resp.(*clientSubscribeResponse).Body.Messages))
	assert.NotEqual(t, last, resp.(*clientSubscribeResponse).Body.Last)

	// test normal recover
	c, _ = newClient(app, &testSession{})
	cmds = []clientCommand{testConnectCmd(timestamp)}
	err = c.handleCommands(cmds)
	assert.Equal(t, nil, err)
	subscribeLastCmd = testSubscribeRecoverCmd("test", last, true)
	resp, err = c.handleCmd(subscribeLastCmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, 2, len(resp.(*clientSubscribeResponse).Body.Messages))
	assert.Equal(t, true, resp.(*clientSubscribeResponse).Body.Recovered)
	assert.Equal(t, MessageID(""), resp.(*clientSubscribeResponse).Body.Last)
	messages = resp.(*clientSubscribeResponse).Body.Messages
	m0, _ := json.Marshal(messages[0].Data)
	m1, _ := json.Marshal(messages[1].Data)
	// in reversed order in history
	assert.Equal(t, strings.Contains(string(m0), "test2"), true)
	assert.Equal(t, strings.Contains(string(m1), "test1"), true)

	// test part recover - when Centrifugo can not recover all missed messages
	for i := 0; i < 10; i++ {
		data, _ = json.Marshal(map[string]string{"input": "test1"})
		err = app.Publish(Channel("test"), data, ConnID(""), nil)
		assert.Equal(t, nil, err)
	}
	c, _ = newClient(app, &testSession{})
	cmds = []clientCommand{testConnectCmd(timestamp)}
	err = c.handleCommands(cmds)
	assert.Equal(t, nil, err)
	subscribeLastCmd = testSubscribeRecoverCmd("test", last, true)
	resp, err = c.handleCmd(subscribeLastCmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, 5, len(resp.(*clientSubscribeResponse).Body.Messages))
	assert.Equal(t, false, resp.(*clientSubscribeResponse).Body.Recovered)
}
