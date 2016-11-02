package proto

import (
	"encoding/json"
	"sync"

	"github.com/centrifugal/centrifugo/libcentrifugo/raw"
)

// NodeInfo contains information and statistics about Centrifugo node.
type NodeInfo struct {
	mu      sync.RWMutex
	UID     string           `json:"uid"`
	Name    string           `json:"name"`
	Started int64            `json:"started_at"`
	Metrics map[string]int64 `json:"metrics"`
	updated int64            `json:"-"`
}

func (i *NodeInfo) Updated() int64 {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.updated
}

func (i *NodeInfo) SetUpdated(up int64) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.updated = up
}

func (i *NodeInfo) SetMetrics(m map[string]int64) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.Metrics = m
}

// ServerStats contains state and metrics information from running Centrifugo nodes.
type ServerStats struct {
	Nodes           []NodeInfo `json:"nodes"`
	MetricsInterval int64      `json:"metrics_interval"`
}

type ClientCommand struct {
	UID    string  `json:"uid"`
	Method string  `json:"method"`
	Params raw.Raw `json:"params"`
}

type ApiCommand struct {
	UID    string  `json:"uid"`
	Method string  `json:"method"`
	Params raw.Raw `json:"params"`
}

var (
	arrayJSONPrefix  byte = '['
	objectJSONPrefix byte = '{'
)

func APICommandsFromJSON(msg []byte) ([]ApiCommand, error) {
	var cmds []ApiCommand

	if len(msg) == 0 {
		return cmds, nil
	}

	firstByte := msg[0]

	switch firstByte {
	case objectJSONPrefix:
		// single command request
		var command ApiCommand
		err := json.Unmarshal(msg, &command)
		if err != nil {
			return nil, err
		}
		cmds = append(cmds, command)
	case arrayJSONPrefix:
		// array of commands received
		err := json.Unmarshal(msg, &cmds)
		if err != nil {
			return nil, err
		}
	default:
		return nil, ErrInvalidMessage
	}
	return cmds, nil
}

func ClientCommandsFromJSON(msgBytes []byte) ([]ClientCommand, error) {
	var cmds []ClientCommand
	firstByte := msgBytes[0]
	switch firstByte {
	case objectJSONPrefix:
		// single command request
		var cmd ClientCommand
		err := json.Unmarshal(msgBytes, &cmd)
		if err != nil {
			return nil, err
		}
		cmds = append(cmds, cmd)
	case arrayJSONPrefix:
		// array of commands received
		err := json.Unmarshal(msgBytes, &cmds)
		if err != nil {
			return nil, err
		}
	}
	return cmds, nil
}

// ConnectClientCommand is a command to authorize connection - it contains user ID
// in web application, additional connection information as JSON string, timestamp
// with unix seconds on moment when connect parameters generated and HMAC token to
// prove correctness of all those parameters.
type ConnectClientCommand struct {
	User      string `json:"user"`
	Timestamp string `json:"timestamp"`
	Info      string `json:"info"`
	Token     string `json:"token"`
}

// RefreshClientCommand is used to prolong connection lifetime when connection check
// mechanism is enabled. It can only be sent by client after successfull connect.
type RefreshClientCommand struct {
	User      string `json:"user"`
	Timestamp string `json:"timestamp"`
	Info      string `json:"info"`
	Token     string `json:"token"`
}

// SubscribeClientCommand is used to subscribe on channel.
// It can only be sent by client after successfull connect.
// It also can have Client, Info and Sign properties when channel is private.
type SubscribeClientCommand struct {
	Channel string `json:"channel"`
	Client  string `json:"client"`
	Last    string `json:"last"`
	Recover bool   `json:"recover"`
	Info    string `json:"info"`
	Sign    string `json:"sign"`
}

// UnsubscribeClientCommand is used to unsubscribe from channel.
type UnsubscribeClientCommand struct {
	Channel string `json:"channel"`
}

// PublishClientCommand is used to publish messages into channel.
type PublishClientCommand struct {
	Channel string  `json:"channel"`
	Data    raw.Raw `json:"data"`
}

// PresenceClientCommand is used to get presence (actual channel subscriptions).
// information for channel
type PresenceClientCommand struct {
	Channel string `json:"channel"`
}

// HistoryClientCommand is used to get history information for channel.
type HistoryClientCommand struct {
	Channel string `json:"channel"`
}

// PingClientCommand is used to ping server.
type PingClientCommand struct {
	Data string `json:"data"`
}

// PublishApiCommand is used to publish messages into channel.
type PublishAPICommand struct {
	Channel string  `json:"channel"`
	Client  string  `json:"client"`
	Data    raw.Raw `json:"data"`
}

// BroadcastApiCommand is used to publish messages into multiple channels.
type BroadcastAPICommand struct {
	Channels []string `json:"channels"`
	Data     raw.Raw  `json:"data"`
	Client   string   `json:"client"`
}

// UnsubscribeApiCommand is used to unsubscribe user from channel.
type UnsubscribeAPICommand struct {
	Channel string `json:"channel"`
	User    string `json:"user"`
}

// DisconnectApiCommand is used to disconnect user.
type DisconnectAPICommand struct {
	User string `json:"user"`
}

// PresenceApiCommand is used to get presence (actual channel subscriptions)
// information for channel.
type PresenceAPICommand struct {
	Channel string `json:"channel"`
}

// HistoryApiCommand is used to get history information for channel.
type HistoryAPICommand struct {
	Channel string `json:"channel"`
}

// PingControlCommand allows nodes to know about each other - node sends this
// control command periodically.
type PingControlCommand struct {
	Info NodeInfo `json:"info"`
}

// UnsubscribeControlCommand required when node received unsubscribe API command –
// node unsubscribes user from channel and then send this control command so other
// nodes could unsubscribe user too.
type UnsubscribeControlCommand struct {
	Channel string `json:"channel"`
	User    string `json:"user"`
}

// DisconnectControlCommand required to disconnect user from all nodes.
type DisconnectControlCommand struct {
	User string `json:"user"`
}

// ConnectAdminCommand required to authorize admin connection and provide
// connection options.
type ConnectAdminCommand struct {
	Token string `json:"token"`
	Watch bool   `json:"watch"`
}

// PingAdminCommand is used to ping server.
type PingAdminCommand struct {
	Data string `json:"data"`
}
