package node

import (
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/lib/proto"

	"github.com/stretchr/testify/assert"
)

func TestNodeRegistry(t *testing.T) {
	registry := newNodeRegistry("node1")
	nodeInfo1 := proto.NodeInfo{UID: "node1"}
	nodeInfo2 := proto.NodeInfo{UID: "node2"}
	registry.add(nodeInfo1)
	registry.add(nodeInfo2)
	assert.Equal(t, 2, len(registry.list()))
	info := registry.get("node1")
	assert.Equal(t, "node1", info.UID)
	registry.clean(10 * time.Second)
	time.Sleep(2 * time.Second)
	registry.clean(time.Second)
	// Current node info should still be in node registry - we never delete it.
	assert.Equal(t, 1, len(registry.list()))
}
