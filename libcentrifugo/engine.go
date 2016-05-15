package libcentrifugo

type historyOpts struct {
	// Limit sets the max amount of messages that must be returned.
	// 0 means no limit - i.e. return all history messages.
	Limit int
}

// Engine is an interface with all methods that can be used by client or
// application to publish message, handle subscriptions, save or retrieve
// presence and history data.
type Engine interface {
	// name returns a name of concrete engine implementation.
	name() string

	// run called once on Centrifugo start just after engine set to application.
	run() error

	// publishMessage allows to send message into channel. This message should be delivered
	// to all clients subscribed on this channel at moment on any Centrifugo node.
	// The returned value is channel in which we will send error as soon as engine finishes
	// publish operation. Also the task of this method is to maintain history for channels
	// if enabled.
	publishMessage(Channel, *Message, *ChannelOptions) <-chan error
	// publishJoin allows to send join message into channel.
	publishJoin(Channel, *JoinMessage) <-chan error
	// publishLeave allows to send leave message into channel.
	publishLeave(Channel, *LeaveMessage) <-chan error
	// publishControl allows to send control message to all connected nodes.
	publishControl(*controlCommand) <-chan error
	// publishAdmin allows to send admin message to all connected admins.
	publishAdmin(*adminCommand) <-chan error

	// subscribe on channel.
	subscribe(Channel) error
	// unsubscribe from channel.
	unsubscribe(Channel) error
	// channels returns slice of currently active channels (with one or more subscribers)
	// on all Centrifugo nodes.
	channels() ([]Channel, error)

	// addPresence sets or updates presence info in channel for connection with uid.
	addPresence(Channel, ConnID, ClientInfo) error
	// removePresence removes presence information for connection with uid.
	removePresence(Channel, ConnID) error
	// presence returns actual presence information for channel.
	presence(Channel) (map[ConnID]ClientInfo, error)

	// history returns a slice of history messages for channel according to provided
	// historyOpts.
	history(Channel, historyOpts) ([]Message, error)
}
