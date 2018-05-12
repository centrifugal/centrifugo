package centrifuge

import (
	"context"
	"io"
	"sync"
	"testing"

	"github.com/centrifugal/centrifuge/internal/proto"
	"github.com/stretchr/testify/assert"
)

type testTransport struct {
	mu     sync.Mutex
	sink   chan *preparedReply
	closed bool
}

func newTestTransport() *testTransport {
	return &testTransport{}
}

func (t *testTransport) Send(rep *preparedReply) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return io.EOF
	}
	if t.sink != nil {
		t.sink <- rep
	}
	return nil
}

func (t *testTransport) Name() string {
	return "test_transport"
}

func (t *testTransport) Encoding() Encoding {
	return proto.EncodingJSON
}

func (t *testTransport) Close(disconnect *Disconnect) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.closed = true
	return nil
}

func TestHub(t *testing.T) {
	h := newHub()
	c, err := newClient(context.Background(), nodeWithMemoryEngine(), newTestTransport())
	assert.NoError(t, err)
	c.user = "test"
	h.add(c)
	assert.Equal(t, len(h.users), 1)
	conns := h.userConnections("test")
	assert.Equal(t, 1, len(conns))
	assert.Equal(t, 1, h.NumClients())
	assert.Equal(t, 1, h.NumUsers())
	h.remove(c)
	assert.Equal(t, len(h.users), 0)
	assert.Equal(t, 1, len(conns))
}

func TestHubSubscriptions(t *testing.T) {
	h := newHub()
	c, err := newClient(context.Background(), nodeWithMemoryEngine(), newTestTransport())
	assert.NoError(t, err)
	h.addSub("test1", c)
	h.addSub("test2", c)
	assert.Equal(t, 2, h.NumChannels())
	channels := []string{}
	for _, ch := range h.Channels() {
		channels = append(channels, string(ch))
	}
	assert.True(t, stringInSlice("test1", channels))
	assert.True(t, stringInSlice("test2", channels))
	assert.True(t, h.NumSubscribers("test1") > 0)
	assert.True(t, h.NumSubscribers("test2") > 0)
	h.removeSub("test1", c)
	h.removeSub("test2", c)
	assert.Equal(t, h.NumChannels(), 0)
	assert.False(t, h.NumSubscribers("test1") > 0)
	assert.False(t, h.NumSubscribers("test2") > 0)
}
