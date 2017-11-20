package control

import "github.com/centrifugal/centrifugo/libcentrifugo/raw"

// Command describes single request command.
// TODO: protobuf.
type Command struct {
	UID    string  `json:"u"`
	Method int     `json:"m"`
	Params raw.Raw `json:"p"`
}

// NodeInfo contains information and statistics about Centrifugo node.
type NodeInfo struct {
	UID     string           `json:"uid"`
	Name    string           `json:"name"`
	Started int64            `json:"started_at"`
	Metrics map[string]int64 `json:"metrics"`
}

// Ping allows nodes to know about each other - node sends this
// control command periodically.
type Ping struct {
	Info NodeInfo `json:"info"`
}

// Unsubscribe used to unsubscribe user from channel on all nodes.
type Unsubscribe struct {
	Channel string `json:"channel"`
	User    string `json:"user"`
}

// Disconnect used to disconnect user on all nodes.
type Disconnect struct {
	User string `json:"user"`
}
