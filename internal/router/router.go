package router

import (
	"github.com/centrifugal/centrifuge"

	"github.com/dghubble/trie"
)

type Router struct {
	node         *centrifuge.Node
	exactRoutes  map[string]any
	prefixRoutes *trie.RuneTrie
}

func New(n *centrifuge.Node) *Router {
	return &Router{
		node:         n,
		exactRoutes:  make(map[string]any),
		prefixRoutes: trie.NewRuneTrie(),
	}
}

func (r *Router) AddExact(channel string, value any) {
	r.exactRoutes[channel] = value
}

func (r *Router) AddPrefix(channelPrefix string, value any) {
	_ = r.prefixRoutes.Put(channelPrefix, value)
}

func (r *Router) Find(channel string) any {
	if value, ok := r.exactRoutes[channel]; ok {
		if r.node.LogEnabled(centrifuge.LogLevelTrace) {
			r.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelTrace, "exact route for channel", map[string]any{"channel": channel}))
		}
		return value
	}

	var value any
	_ = r.prefixRoutes.WalkPath(channel, func(key string, val any) error {
		if val != nil {
			if r.node.LogEnabled(centrifuge.LogLevelTrace) {
				r.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelTrace, "prefix route for channel", map[string]any{"channel": channel}))
			}
			value = val
		}
		return nil
	})

	if value == nil {
		if value, ok := r.exactRoutes["__default"]; ok {
			if r.node.LogEnabled(centrifuge.LogLevelTrace) {
				r.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelTrace, "default route for channel", map[string]any{"channel": channel}))
			}
			return value
		}
	}

	return value
}
