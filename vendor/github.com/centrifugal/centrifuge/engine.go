package centrifuge

import (
	"time"
)

// PresenceStats represents a short presence information for channel.
type PresenceStats struct {
	NumClients int
	NumUsers   int
}

// EngineEventHandler can handle messages received from PUB/SUB system.
type EngineEventHandler interface {
	// Publication must register callback func to handle Publications received.
	HandlePublication(ch string, pub *Publication) error
	// Join must register callback func to handle Join messages received.
	HandleJoin(ch string, join *Join) error
	// Leave must register callback func to handle Leave messages received.
	HandleLeave(ch string, leave *Leave) error
	// Control must register callback func to handle Control data received.
	HandleControl([]byte) error
}

// Engine is responsible for PUB/SUB mechanics, channel history and
// presence information.
type Engine interface {
	// Run called once on start when engine already set to node.
	run(EngineEventHandler) error

	// Publish allows to send Publication into channel. This message should
	// be delivered to all clients subscribed on this channel at moment on
	// any Centrifugo node. The returned value is channel in which we will
	// send error as soon as engine finishes publish operation. Also this
	// method must maintain history for channels if enabled in channel options.
	publish(ch string, pub *Publication, opts *ChannelOptions) <-chan error
	// PublishJoin publishes Join message into channel.
	publishJoin(ch string, join *Join, opts *ChannelOptions) <-chan error
	// PublishLeave publishes Leave message into channel.
	publishLeave(ch string, leave *Leave, opts *ChannelOptions) <-chan error
	// PublishControl allows to send control command data to all running nodes.
	publishControl(data []byte) <-chan error

	// Subscribe node on channel to listen all messages coming from channel.
	subscribe(ch string) error
	// Unsubscribe node from channel to stop listening messages from it.
	unsubscribe(ch string) error
	// Channels returns slice of currently active channels (with
	// one or more subscribers) on all running nodes.
	channels() ([]string, error)

	// History returns a slice of history messages for channel.
	// limit argument sets the max amount of messages that must
	// be returned. 0 means no limit - i.e. return all history
	// messages (though limited by configured history_size). 1 means
	// last (most recent) message only, 2 - two last messages etc.
	history(ch string, limit int) ([]*Publication, error)
	// recoverHistory allows to recover missed messages starting
	// from last seen Publication UID provided by client. This method
	// should return as many Publications as possible and boolean value
	// indicating whether lastUID was found in publications or not
	// The case when publications can not be fully restored
	// can happen if old Publications already removed from history
	// due to size or lifetime limits.
	recoverHistory(ch string, lastUID string) ([]*Publication, bool, error)
	// RemoveHistory removes history from channel. This is in general not
	// needed as history expires automatically (based on history_lifetime)
	// but sometimes can be useful for application logic.
	removeHistory(ch string) error

	// Presence returns actual presence information for channel.
	presence(ch string) (map[string]*ClientInfo, error)
	// PresenseStats returns short stats of current presence data
	// suitable for scenarios when caller does not need full client
	// info returned by presence method.
	presenceStats(ch string) (PresenceStats, error)
	// AddPresence sets or updates presence information in channel
	// for connection with specified identifier. Engine should have a
	// property to expire client information that was not updated
	// (touched) after some configured time interval.
	addPresence(ch string, clientID string, info *ClientInfo, expire time.Duration) error
	// RemovePresence removes presence information for connection
	// with specified identifier.
	removePresence(ch string, clientID string) error
}
