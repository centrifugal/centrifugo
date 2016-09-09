package node

import (
	"github.com/centrifugal/centrifugo/libcentrifugo/config"
	"github.com/centrifugal/centrifugo/libcentrifugo/proto"
)

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
