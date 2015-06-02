package libcentrifugo

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAPICmd(t *testing.T) {
	app := testApp()
	p, _ := app.projectByKey("test1")

	cmd := apiCommand{
		Method: "nonexistent",
		Params: []byte("{}"),
	}
	_, err := app.apiCmd(p, cmd)
	assert.Equal(t, err, ErrMethodNotFound)

	cmd = apiCommand{
		Method: "publish",
		Params: []byte("{}"),
	}
	_, err = app.apiCmd(p, cmd)
	assert.Equal(t, err, ErrInvalidApiMessage)

	cmd = apiCommand{
		Method: "unsubscribe",
		Params: []byte("{}"),
	}
	_, err = app.apiCmd(p, cmd)
	assert.Equal(t, err, ErrInvalidApiMessage)

	cmd = apiCommand{
		Method: "disconnect",
		Params: []byte("{}"),
	}
	_, err = app.apiCmd(p, cmd)
	assert.Equal(t, err, ErrInvalidApiMessage)

	cmd = apiCommand{
		Method: "presence",
		Params: []byte("{}"),
	}
	_, err = app.apiCmd(p, cmd)
	assert.Equal(t, err, ErrInvalidApiMessage)

	cmd = apiCommand{
		Method: "history",
		Params: []byte("{}"),
	}
	_, err = app.apiCmd(p, cmd)
	assert.Equal(t, err, ErrInvalidApiMessage)
}

func TestAPIPublish(t *testing.T) {
	app := testApp()
	p, _ := app.projectByKey("test1")

	cmd := &publishApiCommand{
		Channel: "channel",
		Data:    []byte("null"),
	}
	resp, err := app.publishCmd(p, cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, resp.err)
}

func TestAPIUnsubscribe(t *testing.T) {
	app := testApp()
	p, _ := app.projectByKey("test1")

	cmd := &unsubscribeApiCommand{
		User:    "test user",
		Channel: "channel",
	}
	resp, err := app.unsubcribeCmd(p, cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, resp.err)

	// unsubscribe from all channels
	cmd = &unsubscribeApiCommand{
		User:    "test user",
		Channel: "",
	}
	resp, err = app.unsubcribeCmd(p, cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, resp.err)
}

func TestAPIDisconnect(t *testing.T) {
	app := testApp()
	p, _ := app.projectByKey("test1")

	cmd := &disconnectApiCommand{
		User: "test user",
	}
	resp, err := app.disconnectCmd(p, cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, resp.err)
}

func TestAPIPresence(t *testing.T) {
	app := testApp()
	p, _ := app.projectByKey("test1")

	cmd := &presenceApiCommand{
		Channel: "channel",
	}
	resp, err := app.presenceCmd(p, cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, resp.err)
}

func TestAPIHistory(t *testing.T) {
	app := testApp()
	p, _ := app.projectByKey("test1")

	cmd := &historyApiCommand{
		Channel: "channel",
	}
	resp, err := app.historyCmd(p, cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, resp.err)
}
