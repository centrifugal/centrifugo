package engine

import (
	"github.com/centrifugal/centrifugo/lib/channel"
	"github.com/centrifugal/centrifugo/lib/proto"
	"github.com/centrifugal/centrifugo/lib/proto/control"
)

// Engine is an interface abstracting PUB/SUB mechanics and
// history/presence data manipulations.
type Engine interface {
	// Name returns a name of concrete engine implementation.
	Name() string
	// Run called once on Centrifugo start just after engine set
	// to application.
	Run() error
	// Shutdown stops an engine. This is currently not used so
	// implementations can safely return nil without doing any work.
	Shutdown() error

	// PublishClient allows to send asynchronous message into channel.
	// This message should be delivered to all clients subscribed on this
	// channel at moment on any Centrifugo node. The returned value is
	// channel in which we will send error as soon as engine finishes
	// publish operation. Also this method should maintain history for
	// channels if enabled in channel options.
	PublishClient(msg *proto.Message, opts *channel.Options) <-chan error
	// PublishControl allows to send control command to all running nodes.
	PublishControl(*control.Command) <-chan error

	// Subscribe on channel.
	Subscribe(ch string) error
	// Unsubscribe from channel.
	Unsubscribe(ch string) error
	// Channels returns slice of currently active channels (with
	// one or more subscribers) on all Centrifugo nodes.
	Channels() ([]string, error)

	// AddPresence sets or updates presence information in channel
	// for connection with specified identifier.
	AddPresence(ch string, connID string, info *proto.ClientInfo, expire int) error
	// RemovePresence removes presence information for connection
	// with specified identifier.
	RemovePresence(ch string, connID string) error
	// Presence returns actual presence information for channel.
	Presence(ch string) (map[string]*proto.ClientInfo, error)

	// History returns a slice of history messages for channel.
	// Integer limit sets the max amount of messages that must
	// be returned. 0 means no limit - i.e. return all history
	// messages (actually limited by configured history_size).
	History(ch string, limit int) ([]*proto.Message, error)
	// RemoveHistory removes history from channel. This is in
	// general not needed as history expires automatically (based
	// on history_lifetime) but sometimes can be useful for
	// application logic.
	RemoveHistory(ch string) error
}
