package node

import (
	"sync"
	"time"

	"github.com/centrifugal/centrifugo/lib/proto/controlproto"
)

type nodeRegistry struct {
	// mu allows to synchronize access to node registry.
	mu sync.RWMutex
	// currentUID keeps uid of current node
	currentUID string
	// nodes is a map with information about known nodes.
	nodes map[string]controlproto.Node
	// updates track time we last received ping from node. Used to clean up nodes map.
	updates map[string]int64
}

func newNodeRegistry(currentUID string) *nodeRegistry {
	return &nodeRegistry{
		currentUID: currentUID,
		nodes:      make(map[string]controlproto.Node),
		updates:    make(map[string]int64),
	}
}

func (r *nodeRegistry) list() []controlproto.Node {
	r.mu.RLock()
	nodes := make([]controlproto.Node, len(r.nodes))
	i := 0
	for _, info := range r.nodes {
		nodes[i] = info
		i++
	}
	r.mu.RUnlock()
	return nodes
}

func (r *nodeRegistry) get(uid string) controlproto.Node {
	r.mu.RLock()
	info, _ := r.nodes[uid]
	r.mu.RUnlock()
	return info
}

func (r *nodeRegistry) add(info *controlproto.Node) {
	r.mu.Lock()
	r.nodes[info.UID] = *info
	r.updates[info.UID] = time.Now().Unix()
	r.mu.Unlock()
}

func (r *nodeRegistry) clean(delay time.Duration) {
	r.mu.Lock()
	for uid := range r.nodes {
		if uid == r.currentUID {
			// No need to clean info for current node.
			continue
		}
		updated, ok := r.updates[uid]
		if !ok {
			// As we do all operations with nodes under lock this should never happen.
			delete(r.nodes, uid)
			continue
		}
		if time.Now().Unix()-updated > int64(delay.Seconds()) {
			// Too many seconds since this node have been last seen - remove it from map.
			delete(r.nodes, uid)
			delete(r.updates, uid)
		}
	}
	r.mu.Unlock()
}
