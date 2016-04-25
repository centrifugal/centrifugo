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

	// run called once just after engine set to application.
	run() error

	// publishMessage allows to send message into channel. The returned value is channel
	// in which we will send error as soon as engine finishes publish operation. Also
	// the task of this method is to maintain history for channels if enabled.
	publishMessage(ch Channel, message *Message, opts *ChannelOptions) <-chan error
	// publishJoin allows to send join message into channel.
	publishJoin(ch Channel, message *JoinLeaveMessage) <-chan error
	// publishLeave allows to send leave message into channel.
	publishLeave(ch Channel, message *JoinLeaveMessage) <-chan error
	// publishControl allows to send control message to all connected nodes.
	publishControl(message *ControlCommand) <-chan error
	//
	publishAdmin(message *AdminCommand) <-chan error

	// subscribe on channel.
	subscribe(ch Channel) error
	// unsubscribe from channel.
	unsubscribe(ch Channel) error
	// channels returns slice of currently active channels IDs (with one or more subscribers).
	channels() ([]Channel, error)

	// addPresence sets or updates presence info for connection with uid.
	addPresence(ch Channel, uid ConnID, info ClientInfo) error
	// removePresence removes presence information for connection with uid.
	removePresence(ch Channel, uid ConnID) error
	// presence returns actual presence information for channel.
	presence(ch Channel) (map[ConnID]ClientInfo, error)

	// history returns a slice of history messages for channel, limit sets maximum amount
	// of history messages to return.
	history(ch Channel, opts historyOpts) ([]Message, error)
}
