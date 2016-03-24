package libcentrifugo

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/Godeps/_workspace/src/github.com/stretchr/testify/assert"
)

func testMemoryEngine() *MemoryEngine {
	c := newTestConfig()
	app, _ := NewApplication(&c)
	e := NewMemoryEngine(app)
	app.SetEngine(e)
	return e
}

type TestConn struct {
	Uid      ConnID
	UserID   UserID
	Channels []Channel
}

func (t *TestConn) uid() ConnID {
	return t.Uid
}
func (t *TestConn) user() UserID {
	return t.UserID
}
func (t *TestConn) channels() []Channel {
	return t.Channels
}
func (t *TestConn) send(message []byte) error {
	return nil
}
func (t *TestConn) unsubscribe(ch Channel) error {
	return nil
}
func (t *TestConn) close(reason string) error {
	return nil
}

func TestMemoryEngine(t *testing.T) {
	e := testMemoryEngine()
	err := e.run()
	assert.Equal(t, nil, err)
	assert.NotEqual(t, nil, e.historyHub)
	assert.NotEqual(t, nil, e.presenceHub)
	assert.NotEqual(t, e.name(), "")

	err = <-e.publish(ChannelID("channel"), []byte("{}"), nil)
	assert.Equal(t, nil, err)

	assert.Equal(t, nil, e.subscribe(ChannelID("channel")))

	// Memory engine is actually tightly coupled to application hubs in implementation
	// so calling subscribe on the engine alone is actually a no-op since Application already
	// knows about the subscription.
	// In order to test publish works after subscription is added, we actually need to inject a
	// fake subscription into the Application hub
	fakeConn := &TestConn{"test", "test", []Channel{"channel"}}
	e.app.clients.addSub(ChannelID("channel"), fakeConn)

	// Now we've subscribed...
	err = <-e.publish(ChannelID("channel"), []byte("{}"), nil)
	assert.Equal(t, nil, err)

	assert.Equal(t, nil, e.unsubscribe(ChannelID("channel")))

	// Same dance to manually remove sub from app hub
	e.app.clients.removeSub(ChannelID("channel"), fakeConn)

	assert.Equal(t, nil, e.addPresence(ChannelID("channel"), "uid", ClientInfo{}))
	p, err := e.presence(ChannelID("channel"))
	assert.Equal(t, nil, err)
	assert.Equal(t, 1, len(p))
	err = e.removePresence(ChannelID("channel"), "uid")
	assert.Equal(t, nil, err)

	msg := Message{UID: MessageID("test UID")}
	msgJSON, _ := json.Marshal(msg)

	// test adding history
	assert.Equal(t, nil, <-e.publish(ChannelID("channel"), msgJSON, &publishOpts{msg, 4, 1, false}))
	h, err := e.history(ChannelID("channel"), historyOpts{})
	assert.Equal(t, nil, err)
	assert.Equal(t, 1, len(h))
	assert.Equal(t, h[0].UID, MessageID("test UID"))

	// test history limit
	assert.Equal(t, nil, <-e.publish(ChannelID("channel"), msgJSON, &publishOpts{msg, 4, 1, false}))
	assert.Equal(t, nil, <-e.publish(ChannelID("channel"), msgJSON, &publishOpts{msg, 4, 1, false}))
	assert.Equal(t, nil, <-e.publish(ChannelID("channel"), msgJSON, &publishOpts{msg, 4, 1, false}))
	h, err = e.history(ChannelID("channel"), historyOpts{Limit: 2})
	assert.Equal(t, nil, err)
	assert.Equal(t, 2, len(h))

	// test history limit greater than history size
	assert.Equal(t, nil, <-e.publish(ChannelID("channel"), msgJSON, &publishOpts{msg, 1, 1, false}))
	assert.Equal(t, nil, <-e.publish(ChannelID("channel"), msgJSON, &publishOpts{msg, 1, 1, false}))
	assert.Equal(t, nil, <-e.publish(ChannelID("channel"), msgJSON, &publishOpts{msg, 1, 1, false}))
	h, err = e.history(ChannelID("channel"), historyOpts{Limit: 2})

	// HistoryDropInactive tests - new channel to avoid conflicts with test above
	// 1. add history with DropInactive = true should be a no-op if history is empty
	assert.Equal(t, nil, <-e.publish(ChannelID("channel-2"), msgJSON, &publishOpts{msg, 2, 5, true}))
	h, err = e.history(ChannelID("channel-2"), historyOpts{})
	assert.Equal(t, nil, err)
	assert.Equal(t, 0, len(h))

	// 2. add history with DropInactive = false should always work
	assert.Equal(t, nil, <-e.publish(ChannelID("channel-2"), msgJSON, &publishOpts{msg, 2, 5, false}))
	h, err = e.history(ChannelID("channel-2"), historyOpts{})
	assert.Equal(t, nil, err)
	assert.Equal(t, 1, len(h))

	// 3. add with DropInactive = true should work immediately since there should be something in history
	// for 5 seconds from above
	assert.Equal(t, nil, <-e.publish(ChannelID("channel-2"), msgJSON, &publishOpts{msg, 2, 5, true}))
	h, err = e.history(ChannelID("channel-2"), historyOpts{})
	assert.Equal(t, nil, err)
	assert.Equal(t, 2, len(h))
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
	h.add(ch1, Message{}, addHistoryOpts{1, 1, false})
	h.add(ch1, Message{}, addHistoryOpts{1, 1, false})
	h.add(ch2, Message{}, addHistoryOpts{2, 1, false})
	h.add(ch2, Message{}, addHistoryOpts{2, 1, true}) // Test that adding only if active works when it's active
	hist, err := h.get(ch1, historyOpts{})
	assert.Equal(t, nil, err)
	assert.Equal(t, 1, len(hist))
	hist, err = h.get(ch2, historyOpts{})
	assert.Equal(t, nil, err)
	assert.Equal(t, 2, len(hist))
	time.Sleep(2 * time.Second)

	// test that history cleaned up by periodic task
	assert.Equal(t, 0, len(h.history))
	hist, err = h.get(ch1, historyOpts{})
	assert.Equal(t, 0, len(hist))

	// Now test adding history for inactive channel is a no-op if OnlySaveIfActvie is true
	h.add(ch2, Message{}, addHistoryOpts{2, 10, true})
	assert.Equal(t, 0, len(h.history))
	hist, err = h.get(ch2, historyOpts{})
	assert.Equal(t, 0, len(hist))

	// test history messages limit
	h.add(ch1, Message{}, addHistoryOpts{10, 1, false})
	h.add(ch1, Message{}, addHistoryOpts{10, 1, false})
	h.add(ch1, Message{}, addHistoryOpts{10, 1, false})
	h.add(ch1, Message{}, addHistoryOpts{10, 1, false})
	hist, err = h.get(ch1, historyOpts{})
	assert.Equal(t, 4, len(hist))
	hist, err = h.get(ch1, historyOpts{Limit: 1})
	assert.Equal(t, 1, len(hist))

	// test history limit greater than history size
	h.add(ch1, Message{}, addHistoryOpts{1, 1, false})
	h.add(ch1, Message{}, addHistoryOpts{1, 1, false})
	hist, err = h.get(ch1, historyOpts{Limit: 2})
	assert.Equal(t, 1, len(hist))
}

func TestMemoryChannels(t *testing.T) {
	app := testMemoryApp()
	channels, err := app.engine.channels()
	assert.Equal(t, nil, err)
	assert.Equal(t, 0, len(channels))
	createTestClients(app, 10, 1, nil)
	channels, err = app.engine.channels()
	assert.Equal(t, nil, err)
	assert.Equal(t, 10, len(channels))
}
