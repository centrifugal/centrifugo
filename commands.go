package main

// connectCommand is a command to authorize connection - it contains project key
// to bind connection to a specific project, user ID in web application, additional
// connection information as JSON string, timestamp with unix seconds on moment
// when connect parameters generated and HMAC token to prove correctness of all those
// parameters
type connectCommand struct {
	Project   string
	User      string
	Timestamp string
	Info      string
	Token     string
}

// refreshCommand is used to prolong connection lifetime when connection check
// mechanism is enabled. It can only be sent by client after successfull connect.
type refreshCommand struct {
	Project   string
	User      string
	Timestamp string
	Info      string
	Token     string
}

// subscribeCommand is used to subscribe on channel.
// It can only be sent by client after successfull connect.
// It also can have Client, Info and Sign properties when channel is private.
type subscribeCommand struct {
	Channel string
	Client  string
	Info    string
	Sign    string
}

// unsubscribeCommand is used to unsubscribe from channel
type unsubscribeCommand struct {
	Channel string
}

// publishCommand is used to publish messages into channel
type publishCommand struct {
	Channel string
	Data    interface{}
}

// presenceCommand is used to get presence (actual channel subscriptions)
// information for channel
type presenceCommand struct {
	Channel string
}

// historyCommand is used to get history information for channel
type historyCommand struct {
	Channel string
}
