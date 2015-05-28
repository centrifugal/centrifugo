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
	publish(channel string, message []byte) error

	// subscribe on channel
	subscribe(channel string) error
	// unsubscribe from channel
	unsubscribe(channel string) error

	// addPresence sets or updates presence info for connection with uid
	addPresence(channel, uid string, info ClientInfo) error
	// removePresence removes presence information for connection with uid
	removePresence(channel, uid string) error
	// getPresence returns actual presence information for channel
	presence(channel string) (map[string]ClientInfo, error)

	// addHistoryMessage adds message into channel history and takes care about history size
	addHistoryMessage(channel string, message Message, size, lifetime int64) error
	// getHistory returns history messages for channel
	history(channel string) ([]Message, error)
}
