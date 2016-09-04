package app

import (
	"github.com/centrifugal/centrifugo/libcentrifugo/commands"
	"github.com/centrifugal/centrifugo/libcentrifugo/config"
	"github.com/centrifugal/centrifugo/libcentrifugo/message"
	"github.com/centrifugal/centrifugo/libcentrifugo/response"
)

// TODO: maybe rename to node.
type App interface {
	Config() config.Config
	ClientMsg(message.Channel, *message.Message) error
	JoinMsg(message.Channel, *message.JoinMessage) error
	LeaveMsg(message.Channel, *message.LeaveMessage) error
	AdminMsg(*message.AdminMessage) error
	ControlMsg(*message.ControlMessage) error
	NumSubscribers(message.Channel) int
	Channels() []message.Channel
	ApiCmd(commands.ApiCommand) (response.Response, error)
	NotifyShutdown() chan struct{}
}
