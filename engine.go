package main

// engine is an interface with all methods that can be used by client or
// application to publish message, handle subscriptions, save or retreive
// presence and history data
type engine interface {

	// getName returns a name of concrete engine implementation
	getName() string

	// publishMessage allows to send message into channel
	publishMessage(channel, message string) error

	//publishControlMessage(message string) error
	//publishAdminMessage(message string) error
	//handleMessage(channel, message string) error
	//handleControlMessage(message string) error
	//handleAdminMessage(message string) error

	// addSubscription registers subscription on channel from client's connection
	addSubscription(channel string, c *connection) error
	// removeSubscription unregisters subscription
	removeSubscription(channel string, c *connection) error

	// addPresence sets or updates presence information for connection
	addPresence(channel string, c *connection) error
	// removePresence removes presence information for connection
	removePresence(channel string, c *connection) error
	// getPresence returns actual presence information for channel
	getPresence(channel string) (interface{}, error)

	// addHistoryMessage adds message into channel history and takes care about history size
	addHistoryMessage(channel string, message string) error
	// getHistory returns history messages for channel
	getHistory(channel string) (interface{}, error)
}
