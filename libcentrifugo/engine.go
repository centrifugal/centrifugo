package libcentrifugo

// engine is an interface with all methods that can be used by client or
// application to publish message, handle subscriptions, save or retrieve
// presence and history data
type engine interface {
	// getName returns a name of concrete engine implementation
	name() string

	// initialize provides a way to make additional engine startup work
	initialize() error

	// publish allows to send message into channel
	publish(ch Channel, message []byte) error

	// subscribe on channel
	subscribe(ch Channel) error
	// unsubscribe from channel
	unsubscribe(ch Channel) error

	// addPresence sets or updates presence info for connection with uid
	addPresence(ch Channel, uid ConnID, info ClientInfo) error
	// removePresence removes presence information for connection with uid
	removePresence(ch Channel, uid ConnID) error
	// getPresence returns actual presence information for channel
	presence(ch Channel) (map[ConnID]ClientInfo, error)

	// addHistoryMessage adds message into channel history and takes care about history size
	addHistoryMessage(ch Channel, message Message, size, lifetime int64) error
	// getHistory returns history messages for channel
	history(ch Channel) ([]Message, error)
}
