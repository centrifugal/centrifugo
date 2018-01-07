package node

import (
	"sync"
	"time"

	"github.com/centrifugal/centrifugo/lib/proto/control"
)

type nodeRegistry struct {
	// mu allows to synchronize access to node registry.
	mu sync.RWMutex
	// currentUID keeps uid of current node
	currentUID string
	// nodes is a map with information about known nodes.
	nodes map[string]control.Node
	// updates track time we last received ping from node. Used to clean up nodes map.
	updates map[string]int64
}

func newNodeRegistry(currentUID string) *nodeRegistry {
	return &nodeRegistry{
		currentUID: currentUID,
		nodes:      make(map[string]control.Node),
		updates:    make(map[string]int64),
	}
}

func (r *nodeRegistry) list() []control.Node {
	r.mu.RLock()
	nodes := make([]control.Node, len(r.nodes))
	i := 0
	for _, info := range r.nodes {
		nodes[i] = info
		i++
	}
	r.mu.RUnlock()
	return nodes
}

func (r *nodeRegistry) get(uid string) control.Node {
	r.mu.RLock()
	info, _ := r.nodes[uid]
	r.mu.RUnlock()
	return info
}

func (r *nodeRegistry) add(info *control.Node) {
	r.mu.Lock()
	currentInfo, ok := r.nodes[info.UID]
	if !ok {
		r.nodes[info.UID] = *info
	} else {
		// Only metrics is variable part of running node at moment.
		if len(info.Metrics) > 0 {
			currentInfo.Metrics = info.Metrics
		}
		r.nodes[info.UID] = currentInfo
	}
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
