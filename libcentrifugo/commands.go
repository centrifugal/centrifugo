package libcentrifugo

import (
	"encoding/json"
)

type clientCommand struct {
	UID    string `json:"uid"`
	Method string
	Params json.RawMessage
}

type apiCommand struct {
	UID    string `json:"uid"`
	Method string
	Params json.RawMessage
}

type adminCommand struct {
	Method string
	Params json.RawMessage
}

type controlCommand struct {
	// unique node ID which sent this control command
	UID string

	Method string
	Params *json.RawMessage
}

// connectClientCommand is a command to authorize connection - it contains project key
// to bind connection to a specific project, user ID in web application, additional
// connection information as JSON string, timestamp with unix seconds on moment
// when connect parameters generated and HMAC token to prove correctness of all those
// parameters
type connectClientCommand struct {
	Project   ProjectKey
	User      UserID
	Timestamp string
	Info      string
	Token     string
}

// refreshClientCommand is used to prolong connection lifetime when connection check
// mechanism is enabled. It can only be sent by client after successfull connect.
type refreshClientCommand struct {
	Project   ProjectKey
	User      UserID
	Timestamp string
	Info      string
	Token     string
}

// subscribeClientCommand is used to subscribe on channel.
// It can only be sent by client after successfull connect.
// It also can have Client, Info and Sign properties when channel is private.
type subscribeClientCommand struct {
	Channel Channel
	Client  ConnID
	Info    string
	Sign    string
}

// unsubscribeClientCommand is used to unsubscribe from channel
type unsubscribeClientCommand struct {
	Channel Channel
}

// publishClientCommand is used to publish messages into channel
type publishClientCommand struct {
	Channel Channel
	Data    json.RawMessage
}

// presenceClientCommand is used to get presence (actual channel subscriptions)
// information for channel
type presenceClientCommand struct {
	Channel Channel
}

// historyClientCommand is used to get history information for channel
type historyClientCommand struct {
	Channel Channel
}

// publishApiCommand is used to publish messages into channel
type publishApiCommand struct {
	Channel Channel
	Data    json.RawMessage
	Client  ConnID
}

// unsubscribeApiCommand is used to unsubscribe user from channel
type unsubscribeApiCommand struct {
	Channel Channel
	User    UserID
}

// disconnectApiCommand is used to disconnect user
type disconnectApiCommand struct {
	User UserID
}

// presenceApiCommand is used to get presence (actual channel subscriptions)
// information for channel
type presenceApiCommand struct {
	Channel Channel
}

// historyApiCommand is used to get history information for channel
type historyApiCommand struct {
	Channel Channel
}

// pingControlCommand allows nodes to know about each other - node sends this
// control command periodically
type pingControlCommand struct {
	Uid      string
	Name     string
	Clients  int
	Unique   int
	Channels int
	Started  int64
}

// unsubscribeControlCommand required when node received unsubscribe API command -
// node unsubscribes user from channel and then send this control command so other
// nodes could unsubscribe user too
type unsubscribeControlCommand struct {
	Project ProjectKey
	User    UserID
	Channel Channel
}

// disconnectControlCommand required to disconnect user from all nodes
type disconnectControlCommand struct {
	Project ProjectKey
	User    UserID
}

// authAdminCommand required to authorize admin connection
type authAdminCommand struct {
	Token string
}
