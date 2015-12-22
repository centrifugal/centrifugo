package libcentrifugo

type historyOpts struct {
	// Limit sets the max amount of messages that must be returned.
	// 0 means no limit - i.e. return all history messages.
	Limit int
}

type addHistoryOpts struct {
	// Size is maximum size of channel history that engine must maintain.
	Size int
	// Lifetime is maximum amount of seconds history messages should exist
	// before expiring and most probably being deleted (to prevent memory leaks).
	Lifetime int
	// DropInactive hints to the engine that there were no actual subscribers
	// connected when message was published, and that it can skip saving if there is
	// no unexpired history for the channel (i.e. no subscribers active within history_lifetime)
	DropInactive bool
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
	// Returns whether or not it sent to any active subscribers with `true` indicating
	// at least one subscriber was published to.
	// If engine has no efficient way to know that, it should always return true.
	publish(chID ChannelID, message []byte) (bool, error)

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

	// addHistory adds message into channel history and takes care about history size.
	addHistory(chID ChannelID, message Message, opts addHistoryOpts) error
	// history returns a slice of history messages for channel, limit sets maximum amount
	// of history messages to return.
	history(chID ChannelID, opts historyOpts) ([]Message, error)
}
