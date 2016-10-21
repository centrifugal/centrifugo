package enginememory

import (
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/libcentrifugo/node"
	"github.com/centrifugal/centrifugo/libcentrifugo/proto"
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
	e, _ := NewMemoryEngine(n, nil)
	err := n.Run(&node.RunOptions{Engine: e})
	if err != nil {
		panic(err)
	}
	return e.(*MemoryEngine)
}

type TestConn struct {
	uid      proto.ConnID
	userID   proto.UserID
	channels []proto.Channel
}

func (t *TestConn) UID() proto.ConnID {
	return t.uid
}
func (t *TestConn) User() proto.UserID {
	return t.userID
}
func (t *TestConn) Channels() []proto.Channel {
	return t.channels
}
func (t *TestConn) Send(message []byte) error {
	return nil
}
func (t *TestConn) Handle(message []byte) error {
	return nil
}
func (t *TestConn) Unsubscribe(ch proto.Channel) error {
	return nil
}
func (t *TestConn) Close(reason string) error {
	return nil
}

func newTestMessage() *proto.Message {
	return proto.NewMessage(proto.Channel("test"), []byte("{}"), "", nil)
}

func TestMemoryEngine(t *testing.T) {
	e := testMemoryEngine()
	err := e.Run()
	assert.Equal(t, nil, err)
	assert.NotEqual(t, nil, e.historyHub)
	assert.NotEqual(t, nil, e.presenceHub)
	assert.NotEqual(t, e.Name(), "")

	err = <-e.PublishMessage(proto.Channel("channel"), newTestMessage(), nil)
	assert.Equal(t, nil, err)

	assert.Equal(t, nil, e.Subscribe(proto.Channel("channel")))

	// Memory engine is actually tightly coupled to application hubs in implementation
	// so calling subscribe on the engine alone is actually a no-op since Application already
	// knows about the subscription.
	// In order to test publish works after subscription is added, we actually need to inject a
	// fake subscription into the Application hub
	fakeConn := &TestConn{"test", "test", []proto.Channel{"channel"}}
	e.node.AddClientSub(proto.Channel("channel"), fakeConn)

	// Now we've subscribed...
	err = <-e.PublishMessage(proto.Channel("channel"), newTestMessage(), nil)
	assert.Equal(t, nil, err)

	assert.Equal(t, nil, e.Unsubscribe(proto.Channel("channel")))

	// Same dance to manually remove sub from app hub
	e.node.RemoveClientSub(proto.Channel("channel"), fakeConn)

	assert.Equal(t, nil, e.AddPresence(proto.Channel("channel"), "uid", proto.ClientInfo{}, int(e.node.Config().PresenceExpireInterval.Seconds())))
	p, err := e.Presence(proto.Channel("channel"))
	assert.Equal(t, nil, err)
	assert.Equal(t, 1, len(p))
	err = e.RemovePresence(proto.Channel("channel"), "uid")
	assert.Equal(t, nil, err)

	msg := proto.Message{UID: "test UID"}

	// test adding history
	assert.Equal(t, nil, <-e.PublishMessage(proto.Channel("channel"), &msg, &proto.ChannelOptions{HistorySize: 4, HistoryLifetime: 1, HistoryDropInactive: false}))
	h, err := e.History(proto.Channel("channel"), 0)
	assert.Equal(t, nil, err)
	assert.Equal(t, 1, len(h))
	assert.Equal(t, h[0].UID, "test UID")

	// test history limit
	assert.Equal(t, nil, <-e.PublishMessage(proto.Channel("channel"), &msg, &proto.ChannelOptions{HistorySize: 4, HistoryLifetime: 1, HistoryDropInactive: false}))
	assert.Equal(t, nil, <-e.PublishMessage(proto.Channel("channel"), &msg, &proto.ChannelOptions{HistorySize: 4, HistoryLifetime: 1, HistoryDropInactive: false}))
	assert.Equal(t, nil, <-e.PublishMessage(proto.Channel("channel"), &msg, &proto.ChannelOptions{HistorySize: 4, HistoryLifetime: 1, HistoryDropInactive: false}))
	h, err = e.History(proto.Channel("channel"), 2)
	assert.Equal(t, nil, err)
	assert.Equal(t, 2, len(h))

	// test history limit greater than history size
	assert.Equal(t, nil, <-e.PublishMessage(proto.Channel("channel"), &msg, &proto.ChannelOptions{HistorySize: 1, HistoryLifetime: 1, HistoryDropInactive: false}))
	assert.Equal(t, nil, <-e.PublishMessage(proto.Channel("channel"), &msg, &proto.ChannelOptions{HistorySize: 1, HistoryLifetime: 1, HistoryDropInactive: false}))
	assert.Equal(t, nil, <-e.PublishMessage(proto.Channel("channel"), &msg, &proto.ChannelOptions{HistorySize: 1, HistoryLifetime: 1, HistoryDropInactive: false}))
	h, err = e.History(proto.Channel("channel"), 2)

	// HistoryDropInactive tests - new channel to avoid conflicts with test above
	// 1. add history with DropInactive = true should be a no-op if history is empty
	assert.Equal(t, nil, <-e.PublishMessage(proto.Channel("channel-2"), &msg, &proto.ChannelOptions{HistorySize: 2, HistoryLifetime: 5, HistoryDropInactive: true}))
	h, err = e.History(proto.Channel("channel-2"), 0)
	assert.Equal(t, nil, err)
	assert.Equal(t, 0, len(h))

	// 2. add history with DropInactive = false should always work
	assert.Equal(t, nil, <-e.PublishMessage(proto.Channel("channel-2"), &msg, &proto.ChannelOptions{HistorySize: 2, HistoryLifetime: 5, HistoryDropInactive: false}))
	h, err = e.History(proto.Channel("channel-2"), 0)
	assert.Equal(t, nil, err)
	assert.Equal(t, 1, len(h))

	// 3. add with DropInactive = true should work immediately since there should be something in history
	// for 5 seconds from above
	assert.Equal(t, nil, <-e.PublishMessage(proto.Channel("channel-2"), &msg, &proto.ChannelOptions{HistorySize: 2, HistoryLifetime: 5, HistoryDropInactive: true}))
	h, err = e.History(proto.Channel("channel-2"), 0)
	assert.Equal(t, nil, err)
	assert.Equal(t, 2, len(h))
}

func TestMemoryPresenceHub(t *testing.T) {
	h := newMemoryPresenceHub()
	assert.Equal(t, 0, len(h.presence))

	testCh1 := proto.Channel("channel1")
	testCh2 := proto.Channel("channel2")

	uid := proto.ConnID("uid")

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
	h := newMemoryHistoryHub()
	h.initialize()
	assert.Equal(t, 0, len(h.history))
	ch1 := proto.Channel("channel1")
	ch2 := proto.Channel("channel2")
	h.add(ch1, proto.Message{}, addHistoryOpts{1, 1, false})
	h.add(ch1, proto.Message{}, addHistoryOpts{1, 1, false})
	h.add(ch2, proto.Message{}, addHistoryOpts{2, 1, false})
	h.add(ch2, proto.Message{}, addHistoryOpts{2, 1, true}) // Test that adding only if active works when it's active
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
	assert.Equal(t, 0, len(hist))

	// Now test adding history for inactive channel is a no-op if OnlySaveIfActvie is true
	h.add(ch2, proto.Message{}, addHistoryOpts{2, 10, true})
	assert.Equal(t, 0, len(h.history))
	hist, err = h.get(ch2, 0)
	assert.Equal(t, 0, len(hist))

	// test history messages limit
	h.add(ch1, proto.Message{}, addHistoryOpts{10, 1, false})
	h.add(ch1, proto.Message{}, addHistoryOpts{10, 1, false})
	h.add(ch1, proto.Message{}, addHistoryOpts{10, 1, false})
	h.add(ch1, proto.Message{}, addHistoryOpts{10, 1, false})
	hist, err = h.get(ch1, 0)
	assert.Equal(t, 4, len(hist))
	hist, err = h.get(ch1, 1)
	assert.Equal(t, 1, len(hist))

	// test history limit greater than history size
	h.add(ch1, proto.Message{}, addHistoryOpts{1, 1, false})
	h.add(ch1, proto.Message{}, addHistoryOpts{1, 1, false})
	hist, err = h.get(ch1, 2)
	assert.Equal(t, 1, len(hist))
}

/*
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
*/
