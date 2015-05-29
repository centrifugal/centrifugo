package libcentrifugo

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type testClientConnection struct{}

func (c *testClientConnection) uid() ConnID {
	return "test uid"
}

func (c *testClientConnection) project() ProjectKey {
	return "test project"
}

func (c *testClientConnection) user() UserID {
	return "test user"
}

func (c *testClientConnection) channels() []ChannelID {
	return []ChannelID{"test"}
}

func (c *testClientConnection) send(message string) error {
	return nil
}

func (c *testClientConnection) unsubscribe(channel ChannelID) error {
	return nil
}

func (c *testClientConnection) close(reason string) error {
	return nil
}

func TestClientConnectionHub(t *testing.T) {
	h := newClientHub()
	c := &testClientConnection{}
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

func getTestClientSubscriptionHub() *subHub {
	return newSubHub()
}

func TestClientSubscriptionHub(t *testing.T) {
	h := getTestClientSubscriptionHub()
	c := &testClientConnection{}
	h.add("test1", c)
	h.add("test2", c)
	assert.Equal(t, 2, h.nChannels())
	// FIXME(klauspost): Need to test in channel array
	// channels := h.channels()
	//assert.Equal(t, stringInSlice("test1", channels), true)
	//assert.Equal(t, stringInSlice("test2", channels), true)
	err := h.broadcast("test1", "message")
	assert.Equal(t, err, nil)
	h.remove("test1", c)
	h.remove("test2", c)
}
