package libcentrifugo

import (
	"encoding/json"
	"strconv"
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

	// project and user not set before connect command success
	assert.Equal(t, ProjectKey(""), c.project())
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

func testConnectCmd(timestamp string) clientCommand {
	token := auth.GenerateClientToken("secret", "test1", "user1", timestamp, "")
	connectCmd := connectClientCommand{
		Project:   ProjectKey("test1"),
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

func TestClientConnect(t *testing.T) {
	app := testApp()
	c, err := newClient(app, &testSession{})
	assert.Equal(t, nil, err)

	var cmd clientCommand
	var cmds []clientCommand

	cmd = clientCommand{
		Method: "connect",
		Params: []byte("{}"),
	}
	cmds = []clientCommand{cmd}
	err = c.handleCommands(cmds)
	assert.Equal(t, ErrProjectNotFound, err)

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
}
