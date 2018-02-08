package events

import (
	"context"

	"github.com/centrifugal/centrifugo/lib/conns"
	"github.com/centrifugal/centrifugo/lib/proto"
)

// EventContext ...
type EventContext struct {
	Client conns.Client
}

// EventReply ...
type EventReply struct {
	Error      *proto.Error
	Disconnect *proto.Disconnect
}

// ConnectContext ...
type ConnectContext struct {
	EventContext
}

// ConnectReply ...
type ConnectReply struct {
	EventReply
}

// ConnectHandler ...
type ConnectHandler func(context.Context, *ConnectContext) (*ConnectReply, error)

// DisconnectContext ...
type DisconnectContext struct {
	EventContext
	Disconnect *proto.Disconnect
}

// DisconnectReply ...
type DisconnectReply struct{}

// DisconnectHandler ...
type DisconnectHandler func(context.Context, *DisconnectContext) (*DisconnectReply, error)

// SubscribeContext ...
type SubscribeContext struct {
	EventContext
	Channel string
}

// SubscribeReply ...
type SubscribeReply struct {
	EventReply
}

// SubscribeHandler ...
type SubscribeHandler func(context.Context, *SubscribeContext) (*SubscribeReply, error)

// UnsubscribeContext ...
type UnsubscribeContext struct {
	EventContext
	Channel string
}

// UnsubscribeReply ...
type UnsubscribeReply struct {
	EventReply
}

// UnsubscribeHandler ...
type UnsubscribeHandler func(context.Context, *UnsubscribeContext) (*UnsubscribeReply, error)

// PublishContext ...
type PublishContext struct {
	EventContext
	Channel     string
	Publication *proto.Publication
}

// PublishReply ...
type PublishReply struct {
	EventReply
}

// PublishHandler ...
type PublishHandler func(context.Context, *PublishContext) (*PublishReply, error)

// PresenceContext ...
type PresenceContext struct {
	EventContext
}

// PresenceReply ...
type PresenceReply struct {
	Disconnect *proto.Disconnect
}

// PresenceHandler ...
type PresenceHandler func(context.Context, *PresenceContext) (*PresenceReply, error)

// RefreshContext ...
type RefreshContext struct {
	EventContext
}

// RefreshReply ...
type RefreshReply struct {
	EventReply
	Exp  int64
	Info []byte
}

// RefreshHandler ...
type RefreshHandler func(context.Context, *RefreshContext) (*RefreshReply, error)

// RPCContext ...
type RPCContext struct {
	EventContext
	Data proto.Raw
}

// RPCReply ...
type RPCReply struct {
	EventReply
	Result proto.Raw
}

// RPCHandler must handle incoming command from client.
type RPCHandler func(context.Context, *RPCContext) (*RPCReply, error)
