package app

import (
	"github.com/centrifugal/centrifugo/libcentrifugo/config"
	"github.com/centrifugal/centrifugo/libcentrifugo/proto"
)

// TODO: maybe rename to node.
type App interface {
	Config() config.Config
	SetConfig(*config.Config)
	ClientMsg(proto.Channel, *proto.Message) error
	JoinMsg(proto.Channel, *proto.JoinMessage) error
	LeaveMsg(proto.Channel, *proto.LeaveMessage) error
	AdminMsg(*proto.AdminMessage) error
	ControlMsg(*proto.ControlMessage) error
	NumSubscribers(proto.Channel) int
	Channels() []proto.Channel
	ApiCmd(proto.ApiCommand) (proto.Response, error)
	NotifyShutdown() chan struct{}
	Shutdown() error
}
