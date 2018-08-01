package centrifuge

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func testMemoryEngine() *MemoryEngine {
	n, _ := New(Config{})
	e, _ := NewMemoryEngine(n, MemoryEngineConfig{})
	n.SetEngine(e)
	err := n.Run()
	if err != nil {
		panic(err)
	}
	return e
}

func newTestPublication() *Publication {
	return &Publication{Data: []byte("{}")}
}

func newTestClient(n *Node) *Client {
	transport := newTestTransport()
	ctx := context.Background()
	newCtx := SetCredentials(ctx, &Credentials{UserID: "42"})
	client, _ := newClient(newCtx, n, transport)
	return client
}

func TestMemoryEnginePublishHistory(t *testing.T) {
	e := testMemoryEngine()

	assert.NotEqual(t, nil, e.historyHub)
	assert.NotEqual(t, nil, e.presenceHub)

	err := <-e.publish("channel", newTestPublication(), nil)
	assert.NoError(t, err)

	assert.NoError(t, e.addPresence("channel", "uid", &ClientInfo{}, time.Second))
	p, err := e.presence("channel")
	assert.NoError(t, err)
	assert.Equal(t, 1, len(p))
	assert.NoError(t, e.removePresence("channel", "uid"))

	pub := newTestPublication()
	pub.UID = "test UID"

	// test adding history.
	assert.NoError(t, <-e.publish("channel", pub, &ChannelOptions{HistorySize: 4, HistoryLifetime: 1, HistoryDropInactive: false}))
	h, err := e.history("channel", 0)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(h))
	assert.Equal(t, h[0].UID, "test UID")

	// test history limit.
	assert.NoError(t, <-e.publish("channel", pub, &ChannelOptions{HistorySize: 4, HistoryLifetime: 1, HistoryDropInactive: false}))
	assert.NoError(t, <-e.publish("channel", pub, &ChannelOptions{HistorySize: 4, HistoryLifetime: 1, HistoryDropInactive: false}))
	assert.NoError(t, <-e.publish("channel", pub, &ChannelOptions{HistorySize: 4, HistoryLifetime: 1, HistoryDropInactive: false}))
	h, err = e.history("channel", 2)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(h))

	// test history limit greater than history size
	assert.NoError(t, <-e.publish("channel", pub, &ChannelOptions{HistorySize: 1, HistoryLifetime: 1, HistoryDropInactive: false}))
	assert.NoError(t, <-e.publish("channel", pub, &ChannelOptions{HistorySize: 1, HistoryLifetime: 1, HistoryDropInactive: false}))
	assert.NoError(t, <-e.publish("channel", pub, &ChannelOptions{HistorySize: 1, HistoryLifetime: 1, HistoryDropInactive: false}))
	h, err = e.history("channel", 2)
}

func TestMemoryEnginePublishHistoryDropInactive(t *testing.T) {
	e := testMemoryEngine()

	pub := newTestPublication()
	pub.UID = "test UID"

	// HistoryDropInactive tests - new channel to avoid conflicts with test above
	// 1. add history with DropInactive = true should be a no-op if history is empty
	assert.NoError(t, <-e.publish("channel", pub, &ChannelOptions{HistorySize: 2, HistoryLifetime: 5, HistoryDropInactive: true}))
	h, err := e.history("channel", 2)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(h))

	// 2. add history with DropInactive = false should always work
	assert.NoError(t, <-e.publish("channel", pub, &ChannelOptions{HistorySize: 2, HistoryLifetime: 5, HistoryDropInactive: false}))
	h, err = e.history("channel", 0)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(h))

	// 3. add with DropInactive = true should work immediately since there should be something in history
	// for 5 seconds from above
	assert.NoError(t, <-e.publish("channel", pub, &ChannelOptions{HistorySize: 2, HistoryLifetime: 5, HistoryDropInactive: true}))
	h, err = e.history("channel", 0)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(h))
}

func TestMemoryEngineSubscribeUnsubscribe(t *testing.T) {
	e := testMemoryEngine()
	assert.NoError(t, e.subscribe("channel"))
	assert.NoError(t, e.unsubscribe("channel"))
}

// Emulate history drop inactive edge case: when single client subscribes on channel
// and then goes offline for a short time. At this moment we unsubscribe node from
// channel but we have to save messages into history for history lifetime interval
// so client could recover it.
func TestMemoryEngineDropInactive(t *testing.T) {
	e := testMemoryEngine()

	config := e.node.Config()
	config.HistoryDropInactive = true
	config.HistoryLifetime = 5
	config.HistorySize = 2
	e.node.Reload(config)

	pub := newTestPublication()

	opts, _ := e.node.ChannelOpts("channel")

	assert.NoError(t, <-e.publish("channel", pub, &opts))
	h, err := e.history("channel", 0)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(h))

	e.unsubscribe("channel")

	assert.NoError(t, <-e.publish("channel", pub, &opts))
	h, err = e.history("channel", 0)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(h))
}

func TestMemoryPresenceHub(t *testing.T) {
	h := newPresenceHub()
	assert.Equal(t, 0, len(h.presence))

	testCh1 := "channel1"
	testCh2 := "channel2"
	uid := "uid"

	info := &ClientInfo{
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
	h := newHistoryHub()
	h.initialize()
	h.RLock()
	assert.Equal(t, 0, len(h.history))
	h.RUnlock()
	ch1 := "channel1"
	ch2 := "channel2"
	pub := newTestPublication()
	h.add(ch1, pub, &ChannelOptions{HistorySize: 1, HistoryLifetime: 1, HistoryDropInactive: false}, false)
	h.add(ch1, pub, &ChannelOptions{HistorySize: 1, HistoryLifetime: 1, HistoryDropInactive: false}, false)
	h.add(ch2, pub, &ChannelOptions{HistorySize: 2, HistoryLifetime: 1, HistoryDropInactive: false}, false)

	// Test that adding only if active works when it's active
	h.add(ch2, pub, &ChannelOptions{HistorySize: 2, HistoryLifetime: 1, HistoryDropInactive: true}, false)

	hist, err := h.get(ch1, 0)
	assert.Equal(t, nil, err)
	assert.Equal(t, 1, len(hist))
	hist, err = h.get(ch2, 0)
	assert.Equal(t, nil, err)
	assert.Equal(t, 2, len(hist))
	time.Sleep(2 * time.Second)

	// test that history cleaned up by periodic task
	h.RLock()
	assert.Equal(t, 0, len(h.history))
	h.RUnlock()
	hist, err = h.get(ch1, 0)
	assert.Equal(t, nil, err)
	assert.Equal(t, 0, len(hist))

	// Now test adding history for inactive channel is a no-op if HistoryDropInactive is true
	h.add(ch2, pub, &ChannelOptions{HistorySize: 2, HistoryLifetime: 10, HistoryDropInactive: true}, false)
	h.RLock()
	assert.Equal(t, 0, len(h.history))
	h.RUnlock()
	hist, err = h.get(ch2, 0)
	assert.Equal(t, nil, err)
	assert.Equal(t, 0, len(hist))

	// test history messages limit
	h.add(ch1, pub, &ChannelOptions{HistorySize: 10, HistoryLifetime: 1, HistoryDropInactive: false}, false)
	h.add(ch1, pub, &ChannelOptions{HistorySize: 10, HistoryLifetime: 1, HistoryDropInactive: false}, false)
	h.add(ch1, pub, &ChannelOptions{HistorySize: 10, HistoryLifetime: 1, HistoryDropInactive: false}, false)
	h.add(ch1, pub, &ChannelOptions{HistorySize: 10, HistoryLifetime: 1, HistoryDropInactive: false}, false)
	hist, err = h.get(ch1, 0)
	assert.Equal(t, nil, err)
	assert.Equal(t, 4, len(hist))
	hist, err = h.get(ch1, 1)
	assert.Equal(t, nil, err)
	assert.Equal(t, 1, len(hist))

	// test history limit greater than history size
	h.add(ch1, pub, &ChannelOptions{HistorySize: 1, HistoryLifetime: 1, HistoryDropInactive: false}, false)
	h.add(ch1, pub, &ChannelOptions{HistorySize: 1, HistoryLifetime: 1, HistoryDropInactive: false}, false)
	hist, err = h.get(ch1, 2)
	assert.Equal(t, nil, err)
	assert.Equal(t, 1, len(hist))
}

func BenchmarkMemoryEnginePublish(b *testing.B) {
	e := testMemoryEngine()
	rawData := Raw([]byte(`{"bench": true}`))
	pub := &Publication{UID: "test UID", Data: rawData}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		<-e.publish("channel", pub, &ChannelOptions{HistorySize: 0, HistoryLifetime: 0, HistoryDropInactive: false})
	}
}

func BenchmarkMemoryEnginePublishParallel(b *testing.B) {
	e := testMemoryEngine()
	rawData := Raw([]byte(`{"bench": true}`))
	pub := &Publication{UID: "test UID", Data: rawData}
	b.SetParallelism(128)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			<-e.publish("channel", pub, &ChannelOptions{HistorySize: 0, HistoryLifetime: 0, HistoryDropInactive: false})
		}
	})
}

func BenchmarkMemoryEnginePublishWithHistory(b *testing.B) {
	e := testMemoryEngine()
	rawData := Raw([]byte(`{"bench": true}`))
	pub := &Publication{UID: "test-uid", Data: rawData}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		<-e.publish("channel", pub, &ChannelOptions{HistorySize: 100, HistoryLifetime: 100, HistoryDropInactive: false})
	}
}

func BenchmarkMemoryEnginePublishWithHistoryParallel(b *testing.B) {
	e := testMemoryEngine()
	rawData := Raw([]byte(`{"bench": true}`))
	pub := &Publication{UID: "test-uid", Data: rawData}
	b.SetParallelism(128)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			<-e.publish("channel", pub, &ChannelOptions{HistorySize: 100, HistoryLifetime: 100, HistoryDropInactive: false})
		}
	})
}

func BenchmarkMemoryEngineAddPresence(b *testing.B) {
	e := testMemoryEngine()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := e.addPresence("channel", "uid", &ClientInfo{}, 300*time.Second)
		if err != nil {
			panic(err)
		}
	}
}

func BenchmarkMemoryEngineAddPresenceParallel(b *testing.B) {
	e := testMemoryEngine()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			err := e.addPresence("channel", "uid", &ClientInfo{}, 300*time.Second)
			if err != nil {
				panic(err)
			}
		}
	})
}

func BenchmarkMemoryEnginePresence(b *testing.B) {
	e := testMemoryEngine()
	e.addPresence("channel", "uid", &ClientInfo{}, 300*time.Second)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := e.presence("channel")
		if err != nil {
			panic(err)
		}
	}
}

func BenchmarkMemoryEnginePresenceParallel(b *testing.B) {
	e := testMemoryEngine()
	e.addPresence("channel", "uid", &ClientInfo{}, 300*time.Second)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := e.presence("channel")
			if err != nil {
				panic(err)
			}
		}
	})
}

func BenchmarkMemoryEngineHistory(b *testing.B) {
	e := testMemoryEngine()
	rawData := Raw([]byte("{}"))
	pub := &Publication{UID: "test UID", Data: rawData}
	for i := 0; i < 4; i++ {
		<-e.publish("channel", pub, &ChannelOptions{HistorySize: 4, HistoryLifetime: 300, HistoryDropInactive: false})
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := e.history("channel", 0)
		if err != nil {
			panic(err)
		}

	}
}

func BenchmarkMemoryEngineHistoryParallel(b *testing.B) {
	e := testMemoryEngine()
	rawData := Raw([]byte("{}"))
	pub := &Publication{UID: "test-uid", Data: rawData}
	for i := 0; i < 4; i++ {
		<-e.publish("channel", pub, &ChannelOptions{HistorySize: 4, HistoryLifetime: 300, HistoryDropInactive: false})
	}
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := e.history("channel", 0)
			if err != nil {
				panic(err)
			}
		}
	})
}

func BenchmarkMemoryEngineHistoryRecoverParallel(b *testing.B) {
	e := testMemoryEngine()
	rawData := Raw([]byte("{}"))
	numMessages := 100
	for i := 0; i < numMessages; i++ {
		pub := &Publication{UID: "uid" + strconv.Itoa(i), Data: rawData}
		<-e.publish("channel", pub, &ChannelOptions{HistorySize: numMessages, HistoryLifetime: 300, HistoryDropInactive: false})
	}
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _, err := e.recoverHistory("channel", "uid"+strconv.Itoa(numMessages-5))
			if err != nil {
				panic(err)
			}
		}
	})
}
