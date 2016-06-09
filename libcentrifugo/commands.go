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

type apiCommand struct {
	UID    string          `json:"uid"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

// connectClientCommand is a command to authorize connection - it contains user ID
// in web application, additional connection information as JSON string, timestamp
// with unix seconds on moment when connect parameters generated and HMAC token to
// prove correctness of all those parameters.
type connectClientCommand struct {
	User      UserID `json:"user"`
	Timestamp string `json:"timestamp"`
	Info      string `json:"info"`
	Token     string `json:"token"`
}

// refreshClientCommand is used to prolong connection lifetime when connection check
// mechanism is enabled. It can only be sent by client after successfull connect.
type refreshClientCommand struct {
	User      UserID `json:"user"`
	Timestamp string `json:"timestamp"`
	Info      string `json:"info"`
	Token     string `json:"token"`
}

// subscribeClientCommand is used to subscribe on channel.
// It can only be sent by client after successfull connect.
// It also can have Client, Info and Sign properties when channel is private.
type subscribeClientCommand struct {
	Channel Channel   `json:"channel"`
	Client  ConnID    `json:"client"`
	Last    MessageID `json:"last"`
	Recover bool      `json:"recover"`
	Info    string    `json:"info"`
	Sign    string    `json:"sign"`
}

// unsubscribeClientCommand is used to unsubscribe from channel.
type unsubscribeClientCommand struct {
	Channel Channel `json:"channel"`
}

// publishClientCommand is used to publish messages into channel.
type publishClientCommand struct {
	Channel Channel         `json:"channel"`
	Data    json.RawMessage `json:"data"`
}

// presenceClientCommand is used to get presence (actual channel subscriptions).
// information for channel
type presenceClientCommand struct {
	Channel Channel `json:"channel"`
}

// historyClientCommand is used to get history information for channel.
type historyClientCommand struct {
	Channel Channel `json:"channel"`
}

// pingClientCommand is used to ping server.
type pingClientCommand struct {
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
	Info nodeInfo `json:"info"`
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
