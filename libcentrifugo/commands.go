package libcentrifugo

import (
	"encoding/json"
)

type (
	// Channel is a string channel name.
	Channel string
	// ChannelID is unique channel identificator in Centrifugo.
	ChannelID string
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

type apiCommand struct {
	UID    string          `json:"uid"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

type adminCommand struct {
	UID    string          `json:"uid"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

type controlCommand struct {
	// UID is a unique node ID which sent this control command.
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
	Data string
}

// publishApiCommand is used to publish messages into channel.
type publishAPICommand struct {
	Channel Channel
	Client  ConnID
	Data    json.RawMessage
}

// broadcastApiCommand is used to publish messages into multiple channels.
type broadcastAPICommand struct {
	Channels []Channel
	Data     json.RawMessage
	Client   ConnID
}

// unsubscribeApiCommand is used to unsubscribe user from channel.
type unsubscribeAPICommand struct {
	Channel Channel
	User    UserID
}

// disconnectApiCommand is used to disconnect user.
type disconnectAPICommand struct {
	User UserID
}

// presenceApiCommand is used to get presence (actual channel subscriptions)
// information for channel.
type presenceAPICommand struct {
	Channel Channel
}

// historyApiCommand is used to get history information for channel.
type historyAPICommand struct {
	Channel Channel
}

// pingControlCommand allows nodes to know about each other - node sends this
// control command periodically.
type pingControlCommand struct {
	Info NodeInfo
}

// unsubscribeControlCommand required when node received unsubscribe API command –
// node unsubscribes user from channel and then send this control command so other
// nodes could unsubscribe user too.
type unsubscribeControlCommand struct {
	User    UserID
	Channel Channel
}

// disconnectControlCommand required to disconnect user from all nodes.
type disconnectControlCommand struct {
	User UserID
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
