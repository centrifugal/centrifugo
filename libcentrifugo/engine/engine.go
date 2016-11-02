package engine

import (
	"github.com/centrifugal/centrifugo/libcentrifugo/proto"
)

// Engine is an interface with all methods that can be used by client or
// application to publish message, handle subscriptions, save or retrieve
// presence and history data.
type Engine interface {
	// Name returns a name of concrete engine implementation.
	Name() string

	// Run called once on Centrifugo start just after engine set to application.
	Run() error

	// Shutdown stops an engine. This is currently not used not implemented in
	// Memory and Redis builtin engines.
	Shutdown() error

	// PublishMessage allows to send message into channel. This message should be delivered
	// to all clients subscribed on this channel at moment on any Centrifugo node.
	// The returned value is channel in which we will send error as soon as engine finishes
	// publish operation. Also the task of this method is to maintain history for channels
	// if enabled.
	PublishMessage(*proto.Message, *proto.ChannelOptions) <-chan error
	// PublishJoin allows to send join message into channel.
	PublishJoin(*proto.JoinMessage, *proto.ChannelOptions) <-chan error
	// PublishLeave allows to send leave message into channel.
	PublishLeave(*proto.LeaveMessage, *proto.ChannelOptions) <-chan error
	// PublishControl allows to send control message to all connected nodes.
	PublishControl(*proto.ControlMessage) <-chan error
	// PublishAdmin allows to send admin message to all connected admins.
	PublishAdmin(*proto.AdminMessage) <-chan error

	// Subscribe on channel.
	Subscribe(string) error
	// Unsubscribe from channel.
	Unsubscribe(string) error
	// Channels returns slice of currently active channels (with one or more subscribers)
	// on all Centrifugo nodes.
	Channels() ([]string, error)

	// AddPresence sets or updates presence info in channel for connection with uid.
	AddPresence(ch string, connID string, info proto.ClientInfo, expire int) error
	// RemovePresence removes presence information for connection with uid.
	RemovePresence(ch string, connID string) error
	// Presence returns actual presence information for channel.
	Presence(ch string) (map[string]proto.ClientInfo, error)

	// History returns a slice of history messages for channel.
	// Integer limit sets the max amount of messages that must be returned. 0 means no limit - i.e.
	// return all history messages (actually limited by configured history_size).
	History(ch string, limit int) ([]proto.Message, error)
}
