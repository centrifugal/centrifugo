package libcentrifugo

// engine is an interface with all methods that can be used by client or
// application to publish message, handle subscriptions, save or retrieve
// presence and history data
type Engine interface {
	// getName returns a name of concrete engine implementation
	name() string

	// publish allows to send message into channel
	publish(chID ChannelID, message []byte) error

	// subscribe on channel
	subscribe(chID ChannelID) error
	// unsubscribe from channel
	unsubscribe(chID ChannelID) error

	// addPresence sets or updates presence info for connection with uid
	addPresence(chID ChannelID, uid ConnID, info ClientInfo) error
	// removePresence removes presence information for connection with uid
	removePresence(chID ChannelID, uid ConnID) error
	// getPresence returns actual presence information for channel
	presence(chID ChannelID) (map[ConnID]ClientInfo, error)

	// addHistoryMessage adds message into channel history and takes care about history size
	addHistoryMessage(chID ChannelID, message Message, size, lifetime int64) error
	// getHistory returns history messages for channel
	history(chID ChannelID) ([]Message, error)
}
