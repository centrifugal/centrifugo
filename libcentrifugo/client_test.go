package libcentrifugo

import (
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/Godeps/_workspace/src/github.com/stretchr/testify/assert"
	"github.com/centrifugal/centrifugo/libcentrifugo/auth"
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
	connectCmd := ConnectClientCommand{
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
	refreshCmd := RefreshClientCommand{
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
	subscribeCmd := SubscribeClientCommand{
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
	subscribeCmd := SubscribeClientCommand{
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
	unsubscribeCmd := UnsubscribeClientCommand{
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
	presenceCmd := PresenceClientCommand{
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
	historyCmd := HistoryClientCommand{
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
	publishCmd := PublishClientCommand{
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
	assert.Equal(t, UserID("user1"), clientInfo.User)

	assert.Equal(t, 1, len(app.clients.conns))

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
	assert.Equal(t, ErrPermissionDenied, resp.err)

	cmd = testPublishCmd("test")
	resp, err = c.handleCmd(cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, resp.err)
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
	assert.Equal(t, ErrPermissionDenied, resp.err)

	resp, err = c.handleCmd(testSubscribePrivateCmd("$test", c.UID))
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, resp.err)

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
	assert.Equal(t, ErrPermissionDenied, resp.err)

	_, _ = c.handleCmd(testSubscribeCmd("test"))
	resp, err = c.handleCmd(testPresenceCmd("test"))
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, resp.err)
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
	assert.Equal(t, ErrPermissionDenied, resp.err)

	_, _ = c.handleCmd(testSubscribeCmd("test"))
	resp, err = c.handleCmd(testHistoryCmd("test"))
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, resp.err)
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
	assert.Equal(t, nil, resp.err)
}
