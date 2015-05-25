package libcentrifugo

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type testClientConnection struct{}

func (c *testClientConnection) getUid() string {
	return "test uid"
}

func (c *testClientConnection) getProject() string {
	return "test project"
}

func (c *testClientConnection) getUser() string {
	return "test user"
}

func (c *testClientConnection) getChannels() []string {
	return []string{"test"}
}

func (c *testClientConnection) send(message string) error {
	return nil
}

func (c *testClientConnection) unsubscribe(channel string) error {
	return nil
}

func (c *testClientConnection) close(reason string) error {
	return nil
}

func TestClientConnectionHub(t *testing.T) {
	h := newClientConnectionHub()
	c := &testClientConnection{}
	h.add(c)
	assert.Equal(t, len(h.connections), 1)
	conns := h.getUserConnections("test project", "test user")
	assert.Equal(t, 1, len(conns))
	h.remove(c)
	assert.Equal(t, len(h.connections), 0)
	assert.Equal(t, 1, len(conns))
}
