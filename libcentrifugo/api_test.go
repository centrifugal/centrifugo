package libcentrifugo

import (
	"testing"

	"github.com/centrifugal/centrifugo/Godeps/_workspace/src/github.com/stretchr/testify/assert"
)

func TestAPICmd(t *testing.T) {
	app := testApp()

	cmd := apiCommand{
		Method: "nonexistent",
		Params: []byte("{}"),
	}
	_, err := app.apiCmd(cmd)
	assert.Equal(t, err, ErrMethodNotFound)

	cmd = apiCommand{
		Method: "publish",
		Params: []byte("{}"),
	}
	resp, err := app.apiCmd(cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, ErrInvalidMessage, resp.err)

	cmd = apiCommand{
		Method: "publish",
		Params: []byte("test"),
	}
	_, err = app.apiCmd(cmd)
	assert.Equal(t, ErrInvalidMessage, err)

	cmd = apiCommand{
		Method: "broadcast",
		Params: []byte("{}"),
	}
	resp, err = app.apiCmd(cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, ErrInvalidMessage, resp.err)

	cmd = apiCommand{
		Method: "broadcast",
		Params: []byte("test"),
	}
	_, err = app.apiCmd(cmd)
	assert.Equal(t, ErrInvalidMessage, err)

	cmd = apiCommand{
		Method: "unsubscribe",
		Params: []byte("{}"),
	}
	resp, err = app.apiCmd(cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, ErrInvalidMessage, resp.err)

	cmd = apiCommand{
		Method: "unsubscribe",
		Params: []byte("test"),
	}
	_, err = app.apiCmd(cmd)
	assert.Equal(t, ErrInvalidMessage, err)

	cmd = apiCommand{
		Method: "disconnect",
		Params: []byte("{}"),
	}
	resp, err = app.apiCmd(cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, ErrInvalidMessage, resp.err)

	cmd = apiCommand{
		Method: "disconnect",
		Params: []byte("test"),
	}
	_, err = app.apiCmd(cmd)
	assert.Equal(t, ErrInvalidMessage, err)

	cmd = apiCommand{
		Method: "presence",
		Params: []byte("{}"),
	}
	resp, err = app.apiCmd(cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, ErrInvalidMessage, resp.err)

	cmd = apiCommand{
		Method: "presence",
		Params: []byte("test"),
	}
	_, err = app.apiCmd(cmd)
	assert.Equal(t, ErrInvalidMessage, err)

	cmd = apiCommand{
		Method: "history",
		Params: []byte("{}"),
	}
	resp, err = app.apiCmd(cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, ErrInvalidMessage, resp.err)

	cmd = apiCommand{
		Method: "history",
		Params: []byte("test"),
	}
	_, err = app.apiCmd(cmd)
	assert.Equal(t, ErrInvalidMessage, err)

	cmd = apiCommand{
		Method: "channels",
		Params: []byte("{}"),
	}
	resp, err = app.apiCmd(cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, resp.err)

	cmd = apiCommand{
		Method: "stats",
		Params: []byte("{}"),
	}
	resp, err = app.apiCmd(cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, resp.err)
}

func TestAPIPublish(t *testing.T) {
	app := testApp()
	cmd := &publishAPICommand{
		Channel: "channel",
		Data:    []byte("null"),
	}
	resp, err := app.publishCmd(cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, resp.err)
	cmd = &publishAPICommand{
		Channel: "nonexistentnamespace:channel-2",
		Data:    []byte("null"),
	}
	resp, err = app.publishCmd(cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, ErrNamespaceNotFound, resp.err)
}

func TestAPIBroadcast(t *testing.T) {
	app := testApp()
	cmd := &broadcastAPICommand{
		Channels: []Channel{"channel-1", "channel-2"},
		Data:     []byte("null"),
	}
	resp, err := app.broadcastCmd(cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, resp.err)
	cmd = &broadcastAPICommand{
		Channels: []Channel{"channel-1", "nonexistentnamespace:channel-2"},
		Data:     []byte("null"),
	}
	resp, err = app.broadcastCmd(cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, ErrNamespaceNotFound, resp.err)
	cmd = &broadcastAPICommand{
		Channels: []Channel{},
		Data:     []byte("null"),
	}
	resp, err = app.broadcastCmd(cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, ErrInvalidMessage, resp.err)
}

func TestAPIUnsubscribe(t *testing.T) {
	app := testApp()
	cmd := &unsubscribeAPICommand{
		User:    "test user",
		Channel: "channel",
	}
	resp, err := app.unsubcribeCmd(cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, resp.err)

	// unsubscribe from all channels
	cmd = &unsubscribeAPICommand{
		User:    "test user",
		Channel: "",
	}
	resp, err = app.unsubcribeCmd(cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, resp.err)
}

func TestAPIDisconnect(t *testing.T) {
	app := testApp()
	cmd := &disconnectAPICommand{
		User: "test user",
	}
	resp, err := app.disconnectCmd(cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, resp.err)
}

func TestAPIPresence(t *testing.T) {
	app := testApp()
	cmd := &presenceAPICommand{
		Channel: "channel",
	}
	resp, err := app.presenceCmd(cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, resp.err)
}

func TestAPIHistory(t *testing.T) {
	app := testApp()
	cmd := &historyAPICommand{
		Channel: "channel",
	}
	resp, err := app.historyCmd(cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, resp.err)
}

func TestAPIChannels(t *testing.T) {
	app := testApp()
	resp, err := app.channelsCmd()
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, resp.err)
	app = testMemoryApp()
	createTestClients(app, 10, 1)
	resp, err = app.channelsCmd()
	assert.Equal(t, nil, err)
	body := resp.Body.(*ChannelsBody)
	assert.Equal(t, 10, len(body.Data))
}

func TestAPIStats(t *testing.T) {
	app := testApp()
	resp, err := app.statsCmd()
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, resp.err)
}
