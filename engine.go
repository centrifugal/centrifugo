package main

// engine is an interface with all methods that can be used by client or
// application to publish message, handle subscriptions, save or retreive
// presence and history data
type engine interface {

	// getName returns a name of concrete engine implementation
	getName() string

	// publish allows to send message into channel
	publish(channel, message string) error

	//publishControlMessage(message string) error
	//publishAdminMessage(message string) error
	//handleMessage(channel, message string) error
	//handleControlMessage(message string) error
	//handleAdminMessage(message string) error

	// subscribe on channel
	subscribe(channel string) error
	// unsubscribe from channel
	unsubscribe(channel string) error

	// addPresence sets or updates presence information for connection
	addPresence(channel string, c connection) error
	// removePresence removes presence information for connection
	removePresence(channel string, c connection) error
	// getPresence returns actual presence information for channel
	getPresence(channel string) (interface{}, error)

	// addHistoryMessage adds message into channel history and takes care about history size
	addHistoryMessage(channel string, message string) error
	// getHistory returns history messages for channel
	getHistory(channel string) (interface{}, error)
}
