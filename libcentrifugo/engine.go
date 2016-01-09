package libcentrifugo

type historyOpts struct {
	// Limit sets the max amount of messages that must be returned.
	// 0 means no limit - i.e. return all history messages.
	Limit int
}

type publishOpts struct {
	// Message is Message being published
	Message Message
	// HistorySize is maximum size of channel history that engine must maintain.
	HistorySize int
	// HistoryLifetime is maximum amount of seconds history messages should exist
	// before expiring and most probably being deleted (to prevent memory leaks).
	HistoryLifetime int
	// HistoryDropInactive hints to the engine that there were no actual subscribers
	// connected when message was published, and that it can skip saving if there is
	// no unexpired history for the channel (i.e. no subscribers active within history_lifetime)
	HistoryDropInactive bool
}

// Engine is an interface with all methods that can be used by client or
// application to publish message, handle subscriptions, save or retrieve
// presence and history data.
type Engine interface {
	// name returns a name of concrete engine implementation.
	name() string

	// run called once just after engine set to application.
	run() error

	// publish allows to send message into channel.
	// If publishOpts is nil message must be just sent into provided channel
	// and no additional work must be done.
	publish(chID ChannelID, message []byte, opts *publishOpts) error

	// subscribe on channel.
	subscribe(chID ChannelID) error
	// unsubscribe from channel.
	unsubscribe(chID ChannelID) error
	// channels returns slice of currently active channels IDs (with one or more subscribers).
	channels() ([]ChannelID, error)

	// addPresence sets or updates presence info for connection with uid.
	addPresence(chID ChannelID, uid ConnID, info ClientInfo) error
	// removePresence removes presence information for connection with uid.
	removePresence(chID ChannelID, uid ConnID) error
	// presence returns actual presence information for channel.
	presence(chID ChannelID) (map[ConnID]ClientInfo, error)

	// history returns a slice of history messages for channel, limit sets maximum amount
	// of history messages to return.
	history(chID ChannelID, opts historyOpts) ([]Message, error)
}
