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
		Method: "unsubscribe",
		Params: []byte("{}"),
	}
	resp, err = app.apiCmd(cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, ErrInvalidMessage, resp.err)

	cmd = apiCommand{
		Method: "disconnect",
		Params: []byte("{}"),
	}
	resp, err = app.apiCmd(cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, ErrInvalidMessage, resp.err)

	cmd = apiCommand{
		Method: "presence",
		Params: []byte("{}"),
	}
	resp, err = app.apiCmd(cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, ErrInvalidMessage, resp.err)

	cmd = apiCommand{
		Method: "history",
		Params: []byte("{}"),
	}
	_, err = app.apiCmd(cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, ErrInvalidMessage, resp.err)
}

func TestAPIPublish(t *testing.T) {
	app := testApp()
	cmd := &publishApiCommand{
		Channel: "channel",
		Data:    []byte("null"),
	}
	resp, err := app.publishCmd(cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, resp.err)
}

func TestAPIUnsubscribe(t *testing.T) {
	app := testApp()
	cmd := &unsubscribeApiCommand{
		User:    "test user",
		Channel: "channel",
	}
	resp, err := app.unsubcribeCmd(cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, resp.err)

	// unsubscribe from all channels
	cmd = &unsubscribeApiCommand{
		User:    "test user",
		Channel: "",
	}
	resp, err = app.unsubcribeCmd(cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, resp.err)
}

func TestAPIDisconnect(t *testing.T) {
	app := testApp()
	cmd := &disconnectApiCommand{
		User: "test user",
	}
	resp, err := app.disconnectCmd(cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, resp.err)
}

func TestAPIPresence(t *testing.T) {
	app := testApp()
	cmd := &presenceApiCommand{
		Channel: "channel",
	}
	resp, err := app.presenceCmd(cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, resp.err)
}

func TestAPIHistory(t *testing.T) {
	app := testApp()
	cmd := &historyApiCommand{
		Channel: "channel",
	}
	resp, err := app.historyCmd(cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, resp.err)
}
