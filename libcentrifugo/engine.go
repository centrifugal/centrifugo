package libcentrifugo

type historyOptions struct {
	Size     int64
	Lifetime int64
	Recover  bool
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
	publish(chID ChannelID, message []byte) error

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
	addHistory(chID ChannelID, message Message, opts historyOptions) error
	// history returns a slice of history messages for channel, limit sets maximum amount
	// of history messages to return. If limit is 0 then all history messages must be returned.
	history(chID ChannelID, limit int64) ([]Message, error)
}
