package engine

import (
	"github.com/centrifugal/centrifugo/libcentrifugo/config"
	"github.com/centrifugal/centrifugo/libcentrifugo/proto"
)

type FlagSetter interface {
	StringFlag(p *string, name, shorthand string, value string, usage string)
	BoolFlag(p *bool, name, shorthand string, value bool, usage string)
	IntFlag(p *int, name, shorthand string, value int, usage string)
}

type ConfigSetter interface {
	SetDefault(key string, value interface{})
	BindEnv(key string)
	BindFlag(key string, flagName string)
}

type ConfigGetter interface {
	Get(string) interface{}
	GetString(string) string
	GetBool(string) bool
	GetInt(string) int
	IsSet(string) bool
}

// type EngineFactory func(Node, ConfigValueGetter) Engine

// var Engines map[string]EngineFactory

// func RegisterEngine(name string, fn EngineFactory) {
// 	Engines[name] = fn
// }

// func init() {
// 	Engines = map[string]Engine{
// 		"memory": NewMemoryEngine,
// 		"redis":  NewRedisEngine,
// 	}
// }

type Node interface {
	// Config allows to get node Config.
	Config() config.Config

	// ClientMsg handles client message received from channel -
	// broadcasts it to all connected interested clients.
	ClientMsg(proto.Channel, *proto.Message) error
	// JoinMsg handles join message in channel.
	JoinMsg(proto.Channel, *proto.JoinMessage) error
	// LeaveMsg handles leave message in channel.
	LeaveMsg(proto.Channel, *proto.LeaveMessage) error
	// AdminMsg handles admin message - broadcasts it to all connected admins.
	AdminMsg(*proto.AdminMessage) error
	// ControlMsg handles control message.
	ControlMsg(*proto.ControlMessage) error

	// NumSubscribers allows to get number of active channel subscribers.
	NumSubscribers(proto.Channel) int
	// Channels allows to get list of all active node channels.
	Channels() []proto.Channel

	// ApiCmd allows to handle API command.
	APICmd(proto.ApiCommand) (proto.Response, error)

	// NotifyShutdown allows to get channel which will be closed on node shutdown.
	NotifyShutdown() chan struct{}
}

// Engine is an interface with all methods that can be used by client or
// application to publish message, handle subscriptions, save or retrieve
// presence and history data.
type Engine interface {
	// Name returns a name of concrete engine implementation.
	Name() string

	// Run called once on Centrifugo start just after engine set to application.
	Run() error

	// PublishMessage allows to send message into channel. This message should be delivered
	// to all clients subscribed on this channel at moment on any Centrifugo node.
	// The returned value is channel in which we will send error as soon as engine finishes
	// publish operation. Also the task of this method is to maintain history for channels
	// if enabled.
	PublishMessage(proto.Channel, *proto.Message, *config.ChannelOptions) <-chan error
	// PublishJoin allows to send join message into channel.
	PublishJoin(proto.Channel, *proto.JoinMessage) <-chan error
	// PublishLeave allows to send leave message into channel.
	PublishLeave(proto.Channel, *proto.LeaveMessage) <-chan error
	// PublishControl allows to send control message to all connected nodes.
	PublishControl(*proto.ControlMessage) <-chan error
	// PublishAdmin allows to send admin message to all connected admins.
	PublishAdmin(*proto.AdminMessage) <-chan error

	// Subscribe on channel.
	Subscribe(proto.Channel) error
	// Unsubscribe from channel.
	Unsubscribe(proto.Channel) error
	// Channels returns slice of currently active channels (with one or more subscribers)
	// on all Centrifugo nodes.
	Channels() ([]proto.Channel, error)

	// AddPresence sets or updates presence info in channel for connection with uid.
	AddPresence(proto.Channel, proto.ConnID, proto.ClientInfo) error
	// RemovePresence removes presence information for connection with uid.
	RemovePresence(proto.Channel, proto.ConnID) error
	// Presence returns actual presence information for channel.
	Presence(proto.Channel) (map[proto.ConnID]proto.ClientInfo, error)

	// History returns a slice of history messages for channel.
	// Integer limit sets the max amount of messages that must be returned. 0 means no limit - i.e.
	// return all history messages (actually limited by configured history_size).
	History(ch proto.Channel, limit int) ([]proto.Message, error)
}
