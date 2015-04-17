package main

type Params map[string]interface{}

type clientCommand struct {
	Method string
	Params Params
}

type apiCommand struct {
	Method string
	Params Params
}

type controlCommand struct {
	// unique node ID which sent this control command
	Uid string

	Method string
	Params Params
}

// connectClientCommand is a command to authorize connection - it contains project key
// to bind connection to a specific project, user ID in web application, additional
// connection information as JSON string, timestamp with unix seconds on moment
// when connect parameters generated and HMAC token to prove correctness of all those
// parameters
type connectClientCommand struct {
	Project   string
	User      string
	Timestamp string
	Info      string
	Token     string
}

// refreshClientCommand is used to prolong connection lifetime when connection check
// mechanism is enabled. It can only be sent by client after successfull connect.
type refreshClientCommand struct {
	Project   string
	User      string
	Timestamp string
	Info      string
	Token     string
}

// subscribeClientCommand is used to subscribe on channel.
// It can only be sent by client after successfull connect.
// It also can have Client, Info and Sign properties when channel is private.
type subscribeClientCommand struct {
	Channel string
	Client  string
	Info    string
	Sign    string
}

// unsubscribeClientCommand is used to unsubscribe from channel
type unsubscribeClientCommand struct {
	Channel string
}

// publishClientCommand is used to publish messages into channel
type publishClientCommand struct {
	Channel string
	Data    interface{}
}

// presenceClientCommand is used to get presence (actual channel subscriptions)
// information for channel
type presenceClientCommand struct {
	Channel string
}

// historyClientCommand is used to get history information for channel
type historyClientCommand struct {
	Channel string
}

// publishApiCommand is used to publish messages into channel
type publishApiCommand struct {
	Channel string
	Data    interface{}
}

// unsubscribeApiCommand is used to unsubscribe user from channel
type unsubscribeApiCommand struct {
	Channel string
	User    string
}

// disconnectApiCommand is used to disconnect user
type disconnectApiCommand struct {
	User string
}

// presenceApiCommand is used to get presence (actual channel subscriptions)
// information for channel
type presenceApiCommand struct {
	Channel string
}

// historyApiCommand is used to get history information for channel
type historyApiCommand struct {
	Channel string
}
