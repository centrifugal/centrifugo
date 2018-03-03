package centrifuge

import (
	"context"
)

// EventContext ...
type EventContext struct {
	Client Client
}

// ConnectContext ...
type ConnectContext struct {
	EventContext
}

// ConnectReply ...
type ConnectReply struct {
	Error      *Error
	Disconnect *Disconnect
}

// ConnectHandler ...
type ConnectHandler func(context.Context, *ConnectContext) (*ConnectReply, error)

// DisconnectContext ...
type DisconnectContext struct {
	EventContext
	Disconnect *Disconnect
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
	Error      *Error
	Disconnect *Disconnect
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
	Error      *Error
	Disconnect *Disconnect
}

// UnsubscribeHandler ...
type UnsubscribeHandler func(context.Context, *UnsubscribeContext) (*UnsubscribeReply, error)

// PublishContext ...
type PublishContext struct {
	EventContext
	Channel     string
	Publication *Publication
}

// PublishReply ...
type PublishReply struct {
	Error      *Error
	Disconnect *Disconnect
}

// PublishHandler ...
type PublishHandler func(context.Context, *PublishContext) (*PublishReply, error)

// PresenceContext ...
type PresenceContext struct {
	EventContext
}

// PresenceReply ...
type PresenceReply struct {
	Disconnect *Disconnect
}

// PresenceHandler ...
type PresenceHandler func(context.Context, *PresenceContext) (*PresenceReply, error)

// RefreshContext ...
type RefreshContext struct {
	EventContext
}

// RefreshReply ...
type RefreshReply struct {
	Exp  int64
	Info []byte
}

// RefreshHandler ...
type RefreshHandler func(context.Context, *RefreshContext) (*RefreshReply, error)

// RPCContext ...
type RPCContext struct {
	EventContext
	Data Raw
}

// RPCReply ...
type RPCReply struct {
	Error      *Error
	Disconnect *Disconnect
	Data       Raw
}

// RPCHandler must handle incoming command from client.
type RPCHandler func(context.Context, *RPCContext) (*RPCReply, error)

// MessageContext ...
type MessageContext struct {
	EventContext
	Data Raw
}

// MessageReply ...
type MessageReply struct {
	Disconnect *Disconnect
}

// MessageHandler must handle incoming async message from client.
type MessageHandler func(context.Context, *MessageContext) (*MessageReply, error)
