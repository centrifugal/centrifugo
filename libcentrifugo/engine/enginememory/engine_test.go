package enginememory

import (
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/libcentrifugo/conns"
	"github.com/centrifugal/centrifugo/libcentrifugo/node"
	"github.com/centrifugal/centrifugo/libcentrifugo/proto"
	"github.com/centrifugal/centrifugo/libcentrifugo/raw"
	"github.com/stretchr/testify/assert"
)

func getTestChannelOptions() proto.ChannelOptions {
	return proto.ChannelOptions{
		Watch:           true,
		Publish:         true,
		Presence:        true,
		HistorySize:     1,
		HistoryLifetime: 1,
	}
}

func getTestNamespace(name node.NamespaceKey) node.Namespace {
	return node.Namespace{
		Name:           name,
		ChannelOptions: getTestChannelOptions(),
	}
}

func NewTestConfig() *node.Config {
	c := node.DefaultConfig
	var ns []node.Namespace
	ns = append(ns, getTestNamespace("test"))
	c.Namespaces = ns
	c.Secret = "secret"
	c.ChannelOptions = getTestChannelOptions()
	return c
}

func testMemoryEngine() *MemoryEngine {
	c := NewTestConfig()
	n := node.New("", c)
	e, _ := New(n, nil)
	err := n.Run(&node.RunOptions{Engine: e})
	if err != nil {
		panic(err)
	}
	return e
}

type TestConn struct {
	uid      string
	userID   string
	channels []string
}

func (t *TestConn) UID() string {
	return t.uid
}
func (t *TestConn) User() string {
	return t.userID
}
func (t *TestConn) Channels() []string {
	return t.channels
}
func (t *TestConn) Send(message []byte) error {
	return nil
}
func (t *TestConn) Handle(message []byte) error {
	return nil
}
func (t *TestConn) Unsubscribe(ch string) error {
	return nil
}
func (t *TestConn) Close(adv *conns.DisconnectAdvice) error {
	return nil
}

func newTestMessage() *proto.Message {
	return proto.NewMessage("channel", []byte("{}"), "", nil)
}

func TestMemoryEngine(t *testing.T) {
	e := testMemoryEngine()
	err := e.Run()
	assert.Equal(t, nil, err)
	assert.NotEqual(t, nil, e.historyHub)
	assert.NotEqual(t, nil, e.presenceHub)
	assert.NotEqual(t, e.Name(), "")

	err = <-e.PublishMessage(newTestMessage(), nil)
	assert.Equal(t, nil, err)

	assert.Equal(t, nil, e.Subscribe("channel"))

	// Memory engine is actually tightly coupled to application hubs in implementation
	// so calling subscribe on the engine alone is actually a no-op since Application already
	// knows about the subscription.
	// In order to test publish works after subscription is added, we actually need to inject a
	// fake subscription into the Application hub
	fakeConn := &TestConn{"test", "test", []string{"channel"}}
	e.node.AddClientSub("channel", fakeConn)

	// Now we've subscribed...
	err = <-e.PublishMessage(newTestMessage(), nil)
	assert.Equal(t, nil, err)

	assert.Equal(t, nil, e.Unsubscribe(string("channel")))

	// Same dance to manually remove sub from app hub
	e.node.RemoveClientSub(string("channel"), fakeConn)

	assert.Equal(t, nil, e.AddPresence(string("channel"), "uid", proto.ClientInfo{}, int(e.node.Config().PresenceExpireInterval.Seconds())))
	p, err := e.Presence(string("channel"))
	assert.Equal(t, nil, err)
	assert.Equal(t, 1, len(p))
	err = e.RemovePresence(string("channel"), "uid")
	assert.Equal(t, nil, err)

	msg := proto.Message{UID: "test UID", Channel: "channel"}

	// test adding history
	assert.Equal(t, nil, <-e.PublishMessage(&msg, &proto.ChannelOptions{HistorySize: 4, HistoryLifetime: 1, HistoryDropInactive: false}))
	h, err := e.History("channel", 0)
	assert.Equal(t, nil, err)
	assert.Equal(t, 1, len(h))
	assert.Equal(t, h[0].UID, "test UID")

	// test history limit
	assert.Equal(t, nil, <-e.PublishMessage(&msg, &proto.ChannelOptions{HistorySize: 4, HistoryLifetime: 1, HistoryDropInactive: false}))
	assert.Equal(t, nil, <-e.PublishMessage(&msg, &proto.ChannelOptions{HistorySize: 4, HistoryLifetime: 1, HistoryDropInactive: false}))
	assert.Equal(t, nil, <-e.PublishMessage(&msg, &proto.ChannelOptions{HistorySize: 4, HistoryLifetime: 1, HistoryDropInactive: false}))
	h, err = e.History(string("channel"), 2)
	assert.Equal(t, nil, err)
	assert.Equal(t, 2, len(h))

	// test history limit greater than history size
	assert.Equal(t, nil, <-e.PublishMessage(&msg, &proto.ChannelOptions{HistorySize: 1, HistoryLifetime: 1, HistoryDropInactive: false}))
	assert.Equal(t, nil, <-e.PublishMessage(&msg, &proto.ChannelOptions{HistorySize: 1, HistoryLifetime: 1, HistoryDropInactive: false}))
	assert.Equal(t, nil, <-e.PublishMessage(&msg, &proto.ChannelOptions{HistorySize: 1, HistoryLifetime: 1, HistoryDropInactive: false}))
	h, err = e.History(string("channel"), 2)

	// HistoryDropInactive tests - new channel to avoid conflicts with test above
	// 1. add history with DropInactive = true should be a no-op if history is empty
	msg2 := proto.Message{UID: "test UID", Channel: "channel-2"}

	assert.Equal(t, nil, <-e.PublishMessage(&msg2, &proto.ChannelOptions{HistorySize: 2, HistoryLifetime: 5, HistoryDropInactive: true}))
	h, err = e.History("channel-2", 0)
	assert.Equal(t, nil, err)
	assert.Equal(t, 0, len(h))

	// 2. add history with DropInactive = false should always work
	assert.Equal(t, nil, <-e.PublishMessage(&msg2, &proto.ChannelOptions{HistorySize: 2, HistoryLifetime: 5, HistoryDropInactive: false}))
	h, err = e.History("channel-2", 0)
	assert.Equal(t, nil, err)
	assert.Equal(t, 1, len(h))

	// 3. add with DropInactive = true should work immediately since there should be something in history
	// for 5 seconds from above
	assert.Equal(t, nil, <-e.PublishMessage(&msg2, &proto.ChannelOptions{HistorySize: 2, HistoryLifetime: 5, HistoryDropInactive: true}))
	h, err = e.History("channel-2", 0)
	assert.Equal(t, nil, err)
	assert.Equal(t, 2, len(h))
}

// Emulate history drop inactive edge case: when single client subscribes on channel
// and then goes offline for a short time. At this moment we unsubscribe node from
// channel but we have to save messages into history for history lifetime interval
// so client could recover it.
func TestMemoryEngineDropInactive(t *testing.T) {
	e := testMemoryEngine()
	conf := e.node.Config()
	conf.HistoryDropInactive = true
	conf.HistoryLifetime = 5
	conf.HistorySize = 2
	e.node.SetConfig(&conf)

	err := e.Run()

	msg := proto.Message{UID: "test UID", Channel: "channel-drop-inactive"}
	opts, _ := e.node.ChannelOpts(msg.Channel)

	assert.Nil(t, <-e.PublishMessage(&msg, &opts))
	h, err := e.History(msg.Channel, 0)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(h))

	e.Unsubscribe(msg.Channel)

	assert.Nil(t, <-e.PublishMessage(&msg, &opts))
	h, err = e.History(msg.Channel, 0)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(h))
}

func TestMemoryPresenceHub(t *testing.T) {
	h := newPresenceHub()
	assert.Equal(t, 0, len(h.presence))

	testCh1 := string("channel1")
	testCh2 := string("channel2")

	uid := string("uid")

	info := proto.ClientInfo{
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
	assert.Equal(t, 0, len(h.history))
	ch1 := string("channel1")
	ch2 := string("channel2")
	h.add(ch1, proto.Message{}, &proto.ChannelOptions{HistorySize: 1, HistoryLifetime: 1, HistoryDropInactive: false}, false)
	h.add(ch1, proto.Message{}, &proto.ChannelOptions{HistorySize: 1, HistoryLifetime: 1, HistoryDropInactive: false}, false)
	h.add(ch2, proto.Message{}, &proto.ChannelOptions{HistorySize: 2, HistoryLifetime: 1, HistoryDropInactive: false}, false)
	h.add(ch2, proto.Message{}, &proto.ChannelOptions{HistorySize: 2, HistoryLifetime: 1, HistoryDropInactive: true}, false) // Test that adding only if active works when it's active
	hist, err := h.get(ch1, 0)
	assert.Equal(t, nil, err)
	assert.Equal(t, 1, len(hist))
	hist, err = h.get(ch2, 0)
	assert.Equal(t, nil, err)
	assert.Equal(t, 2, len(hist))
	time.Sleep(2 * time.Second)

	// test that history cleaned up by periodic task
	assert.Equal(t, 0, len(h.history))
	hist, err = h.get(ch1, 0)
	assert.Equal(t, nil, err)
	assert.Equal(t, 0, len(hist))

	// Now test adding history for inactive channel is a no-op if HistoryDropInactive is true
	h.add(ch2, proto.Message{}, &proto.ChannelOptions{HistorySize: 2, HistoryLifetime: 10, HistoryDropInactive: true}, false)
	assert.Equal(t, 0, len(h.history))
	hist, err = h.get(ch2, 0)
	assert.Equal(t, nil, err)
	assert.Equal(t, 0, len(hist))

	// test history messages limit
	h.add(ch1, proto.Message{}, &proto.ChannelOptions{HistorySize: 10, HistoryLifetime: 1, HistoryDropInactive: false}, false)
	h.add(ch1, proto.Message{}, &proto.ChannelOptions{HistorySize: 10, HistoryLifetime: 1, HistoryDropInactive: false}, false)
	h.add(ch1, proto.Message{}, &proto.ChannelOptions{HistorySize: 10, HistoryLifetime: 1, HistoryDropInactive: false}, false)
	h.add(ch1, proto.Message{}, &proto.ChannelOptions{HistorySize: 10, HistoryLifetime: 1, HistoryDropInactive: false}, false)
	hist, err = h.get(ch1, 0)
	assert.Equal(t, nil, err)
	assert.Equal(t, 4, len(hist))
	hist, err = h.get(ch1, 1)
	assert.Equal(t, nil, err)
	assert.Equal(t, 1, len(hist))

	// test history limit greater than history size
	h.add(ch1, proto.Message{}, &proto.ChannelOptions{HistorySize: 1, HistoryLifetime: 1, HistoryDropInactive: false}, false)
	h.add(ch1, proto.Message{}, &proto.ChannelOptions{HistorySize: 1, HistoryLifetime: 1, HistoryDropInactive: false}, false)
	hist, err = h.get(ch1, 2)
	assert.Equal(t, nil, err)
	assert.Equal(t, 1, len(hist))
}

func BenchmarkPublish(b *testing.B) {
	e := testMemoryEngine()
	rawData := raw.Raw([]byte(`{"bench": true}`))
	msg := proto.Message{UID: "test UID", Channel: "channel", Data: rawData}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.PublishMessage(&msg, &proto.ChannelOptions{HistorySize: 0, HistoryLifetime: 0, HistoryDropInactive: false})
	}
}

func BenchmarkPublishWithHistory(b *testing.B) {
	e := testMemoryEngine()
	rawData := raw.Raw([]byte(`{"bench": true}`))
	msg := proto.Message{UID: "test UID", Channel: "channel", Data: rawData}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.PublishMessage(&msg, &proto.ChannelOptions{HistorySize: 10, HistoryLifetime: 300, HistoryDropInactive: false})
	}
}

func BenchmarkOpAddPresence(b *testing.B) {
	e := testMemoryEngine()
	expire := int(e.node.Config().PresenceExpireInterval.Seconds())
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := e.AddPresence("channel", "uid", proto.ClientInfo{}, expire)
		if err != nil {
			panic(err)
		}
	}
}

func BenchmarkOpAddPresenceParallel(b *testing.B) {
	e := testMemoryEngine()
	expire := int(e.node.Config().PresenceExpireInterval.Seconds())
	b.ResetTimer()
	b.SetParallelism(12)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			err := e.AddPresence("channel", "uid", proto.ClientInfo{}, expire)
			if err != nil {
				panic(err)
			}
		}
	})
}

func BenchmarkOpPresence(b *testing.B) {
	e := testMemoryEngine()
	e.AddPresence("channel", "uid", proto.ClientInfo{}, 300)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := e.Presence("channel")
		if err != nil {
			panic(err)
		}
	}
}

func BenchmarkOpPresenceParallel(b *testing.B) {
	e := testMemoryEngine()
	e.AddPresence("channel", "uid", proto.ClientInfo{}, 300)
	b.ResetTimer()
	b.SetParallelism(12)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := e.Presence("channel")
			if err != nil {
				panic(err)
			}
		}
	})
}

func BenchmarkOpHistory(b *testing.B) {
	e := testMemoryEngine()
	rawData := raw.Raw([]byte("{}"))
	msg := proto.Message{UID: "test UID", Channel: "channel", Data: rawData}
	<-e.PublishMessage(&msg, &proto.ChannelOptions{HistorySize: 4, HistoryLifetime: 300, HistoryDropInactive: false})
	<-e.PublishMessage(&msg, &proto.ChannelOptions{HistorySize: 4, HistoryLifetime: 300, HistoryDropInactive: false})
	<-e.PublishMessage(&msg, &proto.ChannelOptions{HistorySize: 4, HistoryLifetime: 300, HistoryDropInactive: false})
	<-e.PublishMessage(&msg, &proto.ChannelOptions{HistorySize: 4, HistoryLifetime: 300, HistoryDropInactive: false})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := e.History("channel", 0)
		if err != nil {
			panic(err)
		}

	}
}

func BenchmarkOpHistoryParallel(b *testing.B) {
	e := testMemoryEngine()
	rawData := raw.Raw([]byte("{}"))
	msg := proto.Message{UID: "test UID", Channel: "channel", Data: rawData}
	<-e.PublishMessage(&msg, &proto.ChannelOptions{HistorySize: 4, HistoryLifetime: 300, HistoryDropInactive: false})
	<-e.PublishMessage(&msg, &proto.ChannelOptions{HistorySize: 4, HistoryLifetime: 300, HistoryDropInactive: false})
	<-e.PublishMessage(&msg, &proto.ChannelOptions{HistorySize: 4, HistoryLifetime: 300, HistoryDropInactive: false})
	<-e.PublishMessage(&msg, &proto.ChannelOptions{HistorySize: 4, HistoryLifetime: 300, HistoryDropInactive: false})
	b.ResetTimer()
	b.SetParallelism(12)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := e.History("channel", 0)
			if err != nil {
				panic(err)
			}
		}
	})
}
