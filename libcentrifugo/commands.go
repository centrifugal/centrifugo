package libcentrifugo

import (
	"encoding/json"
)

type (
	// Channel is a string channel name.
	Channel string
	// UserID is web application user ID as string.
	UserID string
	// ConnID is a unique connection ID.
	ConnID string
	// MessageID is a unique message ID
	MessageID string
)

type clientCommand struct {
	UID    string          `json:"uid"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

type APICommand struct {
	UID    string          `json:"uid"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

type AdminCommand struct {
	UID    string           `json:"uid"`
	Method string           `json:"method"`
	Params *json.RawMessage `json:"params"`
}

type ControlCommand struct {
	// UID in case of controlCommand is a unique node ID which originally published
	// this control command.
	UID    string           `json:"uid"`
	Method string           `json:"method"`
	Params *json.RawMessage `json:"params"`
}

// ConnectClientCommand is a command to authorize connection - it contains user ID
// in web application, additional connection information as JSON string, timestamp
// with unix seconds on moment when connect parameters generated and HMAC token to
// prove correctness of all those parameters.
type ConnectClientCommand struct {
	User      UserID `json:"user"`
	Timestamp string `json:"timestamp"`
	Info      string `json:"info"`
	Token     string `json:"token"`
}

// RefreshClientCommand is used to prolong connection lifetime when connection check
// mechanism is enabled. It can only be sent by client after successfull connect.
type RefreshClientCommand struct {
	User      UserID `json:"user"`
	Timestamp string `json:"timestamp"`
	Info      string `json:"info"`
	Token     string `json:"token"`
}

// SubscribeClientCommand is used to subscribe on channel.
// It can only be sent by client after successfull connect.
// It also can have Client, Info and Sign properties when channel is private.
type SubscribeClientCommand struct {
	Channel Channel   `json:"channel"`
	Client  ConnID    `json:"client"`
	Last    MessageID `json:"last"`
	Recover bool      `json:"recover"`
	Info    string    `json:"info"`
	Sign    string    `json:"sign"`
}

// UnsubscribeClientCommand is used to unsubscribe from channel.
type UnsubscribeClientCommand struct {
	Channel Channel `json:"channel"`
}

// PublishClientCommand is used to publish messages into channel.
type PublishClientCommand struct {
	Channel Channel         `json:"channel"`
	Data    json.RawMessage `json:"data"`
}

// PresenceClientCommand is used to get presence (actual channel subscriptions).
// information for channel
type PresenceClientCommand struct {
	Channel Channel `json:"channel"`
}

// HistoryClientCommand is used to get history information for channel.
type HistoryClientCommand struct {
	Channel Channel `json:"channel"`
}

// PingClientCommand is used to ping server.
type PingClientCommand struct {
	Data string `json:"data"`
}

// publishApiCommand is used to publish messages into channel.
type publishAPICommand struct {
	Channel Channel         `json:"channel"`
	Client  ConnID          `json:"client"`
	Data    json.RawMessage `json:"data"`
}

// broadcastApiCommand is used to publish messages into multiple channels.
type broadcastAPICommand struct {
	Channels []Channel       `json:"channels"`
	Data     json.RawMessage `json:"data"`
	Client   ConnID          `json:"client"`
}

// unsubscribeApiCommand is used to unsubscribe user from channel.
type unsubscribeAPICommand struct {
	Channel Channel `json:"channel"`
	User    UserID  `json:"user"`
}

// disconnectApiCommand is used to disconnect user.
type disconnectAPICommand struct {
	User UserID `json:"user"`
}

// presenceApiCommand is used to get presence (actual channel subscriptions)
// information for channel.
type presenceAPICommand struct {
	Channel Channel `json:"channel"`
}

// historyApiCommand is used to get history information for channel.
type historyAPICommand struct {
	Channel Channel `json:"channel"`
}

// pingControlCommand allows nodes to know about each other - node sends this
// control command periodically.
type pingControlCommand struct {
	Info NodeInfo `json:"info"`
}

// unsubscribeControlCommand required when node received unsubscribe API command –
// node unsubscribes user from channel and then send this control command so other
// nodes could unsubscribe user too.
type unsubscribeControlCommand struct {
	Channel Channel `json:"channel"`
	User    UserID  `json:"user"`
}

// disconnectControlCommand required to disconnect user from all nodes.
type disconnectControlCommand struct {
	User UserID `json:"user"`
}

// connectAdminCommand required to authorize admin connection and provide
// connection options.
type connectAdminCommand struct {
	Token string `json:"token"`
	Watch bool   `json:"watch"`
}

// pingAdminCommand is used to ping server.
type pingAdminCommand struct {
	Data string `json:"data"`
}
