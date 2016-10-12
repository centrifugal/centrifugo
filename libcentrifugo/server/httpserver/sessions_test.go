package httpserver

import (
	"errors"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
)

type testWSConnection struct {
	writeErr   bool
	readErr    bool
	controlErr bool
	closeErr   bool
	closed     bool
}

func (c *testWSConnection) ReadMessage() (messageType int, p []byte, err error) {
	if c.readErr {
		return websocket.TextMessage, nil, errors.New("error")
	}
	return websocket.TextMessage, []byte("test"), nil
}

func (c *testWSConnection) WriteMessage(messageType int, data []byte) error {
	if c.writeErr {
		return errors.New("error")
	}
	return nil
}

func (c *testWSConnection) WriteControl(messageType int, data []byte, deadline time.Time) error {
	if c.controlErr {
		return errors.New("error")
	}
	return nil
}

func (c *testWSConnection) Close() error {
	c.closed = true
	if c.closeErr {
		return errors.New("error")
	}
	return nil
}

func TestWSConnPing(t *testing.T) {
	ws := &testWSConnection{}
	c := newWSSession(ws, 1*time.Nanosecond)
	c.ping()
	assert.Equal(t, false, c.ws.(*testWSConnection).closed)
}

func TestWSConnPingFailed(t *testing.T) {
	ws := &testWSConnection{controlErr: true}
	c := newWSSession(ws, 1*time.Nanosecond)
	c.ping()
	assert.Equal(t, true, c.ws.(*testWSConnection).closed)
}

func TestWSConnPingAfterClose(t *testing.T) {
	ws := &testWSConnection{}
	c := newWSSession(ws, 1*time.Nanosecond)
	err := c.Close(1, "test close")
	// TODO: closing channel should be inside Close!
	close(c.closeCh)
	assert.Equal(t, nil, err)
	c.ping()
	assert.Equal(t, true, c.ws.(*testWSConnection).closed)
}

func TestSendAfterClose(t *testing.T) {
	ws := &testWSConnection{}
	c := newWSSession(ws, 1*time.Nanosecond)
	err := c.Close(1, "test close")
	// TODO: closing channel should be inside Close!
	close(c.closeCh)
	assert.Equal(t, nil, err)
	err = c.Send([]byte("test"))
	assert.Equal(t, nil, err)
}
