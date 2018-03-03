package centrifuge

import (
	"time"

	"github.com/centrifugal/centrifuge/internal/proto"
	"github.com/centrifugal/centrifuge/internal/proto/controlproto"
)

// historyFilter allows to provide several parameters for history
// extraction.
type historyFilter struct {
	// Limit sets the max amount of messages that must
	// be returned. 0 means no limit - i.e. return all history
	// messages (limited by configured history_size). 1 means
	// last message only.
	Limit int
}

// Engine is an interface abstracting PUB/SUB mechanics and
// history/presence data manipulations.
type Engine interface {
	// Name returns a name of concrete engine implementation.
	name() string
	// Run called once on start just after engine set to node.
	run() error
	// Shutdown stops an engine. This is currently not used so implementations
	// can safely return nil without doing any work.
	shutdown() error

	// Publish allows to send Publication into channel. This message should
	// be delivered to all clients subscribed on this channel at moment on
	// any Centrifugo node. The returned value is channel in which we will
	// send error as soon as engine finishes publish operation. Also this
	// method must maintain history for channels if enabled in channel options.
	publish(ch string, publication *proto.Publication, opts *ChannelOptions) <-chan error
	// PublishJoin publishes Join message into channel.
	publishJoin(ch string, join *proto.Join, opts *ChannelOptions) <-chan error
	// PublishLeave publishes Leave message into channel.
	publishLeave(ch string, leave *proto.Leave, opts *ChannelOptions) <-chan error
	// PublishControl allows to send control command to all running nodes.
	publishControl(*controlproto.Command) <-chan error

	// Subscribe node on channel.
	subscribe(ch string) error
	// Unsubscribe node from channel.
	unsubscribe(ch string) error
	// Channels returns slice of currently active channels (with
	// one or more subscribers) on all Centrifuge nodes.
	channels() ([]string, error)

	// History returns a slice of history messages for channel.
	history(ch string, filter historyFilter) ([]*proto.Publication, error)
	// RemoveHistory removes history from channel. This is in general not
	// needed as history expires automatically (based on history_lifetime)
	// but sometimes can be useful for application logic.
	removeHistory(ch string) error

	// AddPresence sets or updates presence information in channel
	// for connection with specified identifier.
	addPresence(ch string, connID string, info *proto.ClientInfo, expire time.Duration) error
	// RemovePresence removes presence information for connection
	// with specified identifier.
	removePresence(ch string, connID string) error
	// Presence returns actual presence information for channel.
	presence(ch string) (map[string]*proto.ClientInfo, error)
}
