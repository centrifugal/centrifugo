package node

import (
	"github.com/centrifugal/centrifugo/libcentrifugo/engine"
	"github.com/centrifugal/centrifugo/libcentrifugo/proto"
	"github.com/centrifugal/centrifugo/libcentrifugo/server"
)

// EngineHandler contains all methods required to process messages coming from
// engine and deliver them to connected users if needed.
type EngineHandler interface {
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
}

type NodeRunOptions struct {
	Engine   engine.Engine
	Servers  map[string]server.Server
	Mediator Mediator
}

type Node interface {
	// Run starts a node with provided Engine, Servers and Mediator.
	Run(*NodeRunOptions) error

	// Shutdown shuts down a node.
	Shutdown() error

	// NotifyShutdown allows to get a channel which will be closed on node shutdown.
	NotifyShutdown() chan struct{}

	// Config allows to get copy of current node Config.
	Config() Config
	// SetConfig allows to set node config.
	SetConfig(*Config)

	// Returns EngineHandler which allows to process messages from Engine.
	EngineHandler() EngineHandler

	// Returns node ClientHub.
	ClientHub() ClientHub
	// Returns node AdminHub.
	AdminHub() AdminHub

	// NewClient creates new client connection.
	NewClient(Session, *ClientOptions) (ClientConn, error)
	// NewAdminClient creates new admin connection.
	NewAdminClient(Session, *AdminOptions) (AdminConn, error)

	// ApiCmd allows to handle API command.
	APICmd(proto.ApiCommand, *APIOptions) (proto.Response, error)
}
