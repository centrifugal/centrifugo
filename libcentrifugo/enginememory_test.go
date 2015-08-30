package libcentrifugo

import (
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/Godeps/_workspace/src/github.com/stretchr/testify/assert"
)

func testMemoryEngine() *MemoryEngine {
	app, _ := NewApplication(newTestConfig())
	e := NewMemoryEngine(app)
	app.SetEngine(e)
	return e
}

func TestMemoryEngine(t *testing.T) {
	e := testMemoryEngine()
	assert.NotEqual(t, nil, e.historyHub)
	assert.NotEqual(t, nil, e.presenceHub)
	assert.NotEqual(t, e.name(), "")
	assert.Equal(t, nil, e.publish(ChannelID("channel"), []byte("{}")))
	assert.Equal(t, nil, e.subscribe(ChannelID("channel")))
	assert.Equal(t, nil, e.unsubscribe(ChannelID("channel")))
	assert.Equal(t, nil, e.addPresence(ChannelID("channel"), "uid", ClientInfo{}))
	p, err := e.presence(ChannelID("channel"))
	assert.Equal(t, nil, err)
	assert.Equal(t, 1, len(p))
	assert.Equal(t, nil, e.addHistory(ChannelID("channel"), Message{}, 1, 1))
	h, err := e.history(ChannelID("channel"))
	assert.Equal(t, nil, err)
	assert.Equal(t, 1, len(h))
}

func TestMemoryPresenceHub(t *testing.T) {
	h := newMemoryPresenceHub()
	assert.Equal(t, 0, len(h.presence))

	testCh1 := ChannelID("channel1")
	testCh2 := ChannelID("channel2")

	uid := ConnID("uid")

	info := ClientInfo{
		User:   "user",
		Client: "client",
	}

	h.add(testCh1, uid, info)
	assert.Equal(t, 1, len(h.presence))
	h.add(testCh2, uid, info)
	assert.Equal(t, 2, len(h.presence))
	h.remove(testCh1, uid)
	// remove non existing must not fail
	err := h.remove(testCh1, uid)
	assert.Equal(t, nil, err)
	assert.Equal(t, 1, len(h.presence))
	p, err := h.get(testCh1)
	assert.Equal(t, nil, err)
	assert.Equal(t, 0, len(p))
	p, err = h.get(testCh2)
	assert.Equal(t, nil, err)
	assert.Equal(t, 1, len(p))
}

func TestMemoryHistoryHub(t *testing.T) {
	h := newMemoryHistoryHub()
	h.initialize()
	assert.Equal(t, 0, len(h.history))
	ch1 := ChannelID("channel1")
	ch2 := ChannelID("channel2")
	h.add(ch1, Message{}, 1, 1)
	h.add(ch1, Message{}, 1, 1)
	h.add(ch2, Message{}, 2, 1)
	h.add(ch2, Message{}, 2, 1)
	hist, err := h.get(ch1)
	assert.Equal(t, nil, err)
	assert.Equal(t, 1, len(hist))
	hist, err = h.get(ch2)
	assert.Equal(t, nil, err)
	assert.Equal(t, 2, len(hist))
	time.Sleep(2 * time.Second)
	// test that history cleaned up by periodic task
	assert.Equal(t, 0, len(h.history))
	hist, err = h.get(ch1)
	assert.Equal(t, 0, len(hist))
}
