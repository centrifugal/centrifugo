package libcentrifugo

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type testClientConn struct{}

func (c *testClientConn) uid() ConnID {
	return "test uid"
}

func (c *testClientConn) project() ProjectKey {
	return "test project"
}

func (c *testClientConn) user() UserID {
	return "test user"
}

func (c *testClientConn) channels() []Channel {
	return []Channel{"test"}
}

func (c *testClientConn) send(message string) error {
	return nil
}

func (c *testClientConn) unsubscribe(channel Channel) error {
	return nil
}

func (c *testClientConn) close(reason string) error {
	return nil
}

type testAdminConn struct{}

func (c *testAdminConn) uid() ConnID {
	return "test uid"
}

func (c *testAdminConn) send(message string) error {
	return nil
}

func TestClientHub(t *testing.T) {
	h := newClientHub()
	c := &testClientConn{}
	h.add(c)
	assert.Equal(t, len(h.connections), 1)
	conns := h.userConnections("test project", "test user")
	assert.Equal(t, 1, len(conns))
	assert.Equal(t, 1, h.nClients())
	assert.Equal(t, 1, h.nUniqueClients())
	h.remove(c)
	assert.Equal(t, len(h.connections), 0)
	assert.Equal(t, 1, len(conns))
}

func TestSubHub(t *testing.T) {
	h := newSubHub()
	c := &testClientConn{}
	h.add("test1", c)
	h.add("test2", c)
	assert.Equal(t, 2, h.nChannels())
	channels := []string{}
	for _, ch := range h.channels() {
		channels = append(channels, string(ch))
	}
	assert.Equal(t, stringInSlice("test1", channels), true)
	assert.Equal(t, stringInSlice("test2", channels), true)
	err := h.broadcast("test1", "message")
	assert.Equal(t, err, nil)
	h.remove("test1", c)
	h.remove("test2", c)
	assert.Equal(t, len(h.subs), 0)
}

func TestAdminHub(t *testing.T) {
	h := newAdminHub()
	c := &testAdminConn{}
	err := h.add(c)
	assert.Equal(t, err, nil)
	assert.Equal(t, len(h.connections), 1)
	err = h.broadcast("message")
	assert.Equal(t, err, nil)
	err = h.remove(c)
	assert.Equal(t, err, nil)
	assert.Equal(t, len(h.connections), 0)
}
