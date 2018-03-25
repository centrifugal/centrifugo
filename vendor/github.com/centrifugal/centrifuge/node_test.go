package centrifuge

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/centrifugal/centrifuge/internal/proto"
	"github.com/centrifugal/centrifuge/internal/proto/controlproto"

	"github.com/stretchr/testify/assert"
)

type TestEngine struct {
	publishCount        int32
	publishJoinCount    int32
	publishLeaveCount   int32
	publishControlCount int32
}

func NewTestEngine() *TestEngine {
	return &TestEngine{}
}

func (e *TestEngine) name() string {
	return "test engine"
}

func (e *TestEngine) run() error {
	return nil
}

func (e *TestEngine) shutdown() error {
	return nil
}

func (e *TestEngine) publish(ch string, publication *proto.Publication, opts *ChannelOptions) <-chan error {
	atomic.AddInt32(&e.publishCount, 1)
	eChan := make(chan error, 1)
	eChan <- nil
	return eChan
}

func (e *TestEngine) publishJoin(ch string, join *proto.Join, opts *ChannelOptions) <-chan error {
	atomic.AddInt32(&e.publishJoinCount, 1)
	eChan := make(chan error, 1)
	eChan <- nil
	return eChan
}

func (e *TestEngine) publishLeave(ch string, leave *proto.Leave, opts *ChannelOptions) <-chan error {
	atomic.AddInt32(&e.publishLeaveCount, 1)
	eChan := make(chan error, 1)
	eChan <- nil
	return eChan
}

func (e *TestEngine) publishControl(msg []byte) <-chan error {
	atomic.AddInt32(&e.publishControlCount, 1)
	eChan := make(chan error, 1)
	eChan <- nil
	return eChan
}

func (e *TestEngine) subscribe(ch string) error {
	return nil
}

func (e *TestEngine) unsubscribe(ch string) error {
	return nil
}

func (e *TestEngine) addPresence(ch string, uid string, info *proto.ClientInfo, expire time.Duration) error {
	return nil
}

func (e *TestEngine) removePresence(ch string, uid string) error {
	return nil
}

func (e *TestEngine) presence(ch string) (map[string]*proto.ClientInfo, error) {
	return map[string]*proto.ClientInfo{}, nil
}

func (e *TestEngine) history(ch string, filter historyFilter) ([]*proto.Publication, error) {
	return []*proto.Publication{}, nil
}

func (e *TestEngine) removeHistory(ch string) error {
	return nil
}

func (e *TestEngine) channels() ([]string, error) {
	return []string{}, nil
}

func testNode() *Node {
	c := DefaultConfig
	n := New(c)
	n.SetEngine(NewTestEngine())
	err := n.Run()
	if err != nil {
		panic(err)
	}
	return n
}

func TestUserAllowed(t *testing.T) {
	node := testNode()
	assert.True(t, node.userAllowed("channel#1", "1"))
	assert.True(t, node.userAllowed("channel", "1"))
	assert.False(t, node.userAllowed("channel#1", "2"))
	assert.True(t, node.userAllowed("channel#1,2", "1"))
	assert.True(t, node.userAllowed("channel#1,2", "2"))
	assert.False(t, node.userAllowed("channel#1,2", "3"))
}

func TestSetConfig(t *testing.T) {
	node := testNode()
	err := node.Reload(DefaultConfig)
	assert.NoError(t, err)
}

func TestClientAllowed(t *testing.T) {
	node := testNode()
	assert.True(t, node.clientAllowed("channel&67330d48-f668-4916-758b-f4eb1dd5b41d", string("67330d48-f668-4916-758b-f4eb1dd5b41d")))
	assert.True(t, node.clientAllowed("channel", string("67330d48-f668-4916-758b-f4eb1dd5b41d")))
	assert.False(t, node.clientAllowed("channel&long-client-id", string("wrong-client-id")))
}

func TestNodeRegistry(t *testing.T) {
	registry := newNodeRegistry("node1")
	nodeInfo1 := controlproto.Node{UID: "node1"}
	nodeInfo2 := controlproto.Node{UID: "node2"}
	registry.add(&nodeInfo1)
	registry.add(&nodeInfo2)
	assert.Equal(t, 2, len(registry.list()))
	info := registry.get("node1")
	assert.Equal(t, "node1", info.UID)
	registry.clean(10 * time.Second)
	time.Sleep(2 * time.Second)
	registry.clean(time.Second)
	// Current node info should still be in node registry - we never delete it.
	assert.Equal(t, 1, len(registry.list()))
}
