package centrifuge

import (
	"context"
)

// ConnectEvent contains fields related to connect event.
type ConnectEvent struct{}

// ConnectReply contains fields determining the reaction on connect event.
type ConnectReply struct {
	Error      *Error
	Disconnect *Disconnect
}

// ConnectHandler ...
type ConnectHandler func(context.Context, Client, ConnectEvent) ConnectReply

// DisconnectEvent contains fields related to disconnect event.
type DisconnectEvent struct {
	Disconnect *Disconnect
}

// DisconnectReply contains fields determining the reaction on disconnect event.
type DisconnectReply struct{}

// DisconnectHandler ...
type DisconnectHandler func(DisconnectEvent) DisconnectReply

// SubscribeEvent contains fields related to subscribe event.
type SubscribeEvent struct {
	Channel string
}

// SubscribeReply contains fields determining the reaction on subscribe event.
type SubscribeReply struct {
	Error       *Error
	Disconnect  *Disconnect
	ChannelInfo Raw
}

// SubscribeHandler ...
type SubscribeHandler func(SubscribeEvent) SubscribeReply

// UnsubscribeEvent contains fields related to unsubscribe event.
type UnsubscribeEvent struct {
	Channel string
}

// UnsubscribeReply contains fields determining the reaction on unsubscribe event.
type UnsubscribeReply struct {
}

// UnsubscribeHandler ...
type UnsubscribeHandler func(UnsubscribeEvent) UnsubscribeReply

// PublishEvent contains fields related to publish event.
type PublishEvent struct {
	Channel string
	Pub     *Pub
}

// PublishReply contains fields determining the reaction on publish event.
type PublishReply struct {
	Error      *Error
	Disconnect *Disconnect
}

// PublishHandler ...
type PublishHandler func(PublishEvent) PublishReply

// PresenceEvent contains fields related to presence update event.
type PresenceEvent struct{}

// PresenceReply contains fields determining the reaction on presence update event.
type PresenceReply struct {
	Disconnect *Disconnect
}

// PresenceHandler ...
type PresenceHandler func(PresenceEvent) PresenceReply

// RefreshEvent contains fields related to refresh event.
type RefreshEvent struct{}

// RefreshReply contains fields determining the reaction on refresh event.
type RefreshReply struct {
	Exp  int64
	Info []byte
}

// RefreshHandler ...
type RefreshHandler func(RefreshEvent) RefreshReply

// RPCEvent contains fields related to rpc request.
type RPCEvent struct {
	Data Raw
}

// RPCReply contains fields determining the reaction on rpc request.
type RPCReply struct {
	Error      *Error
	Disconnect *Disconnect
	Data       Raw
}

// RPCHandler must handle incoming command from client.
type RPCHandler func(RPCEvent) RPCReply

// MessageEvent contains fields related to message request.
type MessageEvent struct {
	Data Raw
}

// MessageReply contains fields determining the reaction on message request.
type MessageReply struct {
	Disconnect *Disconnect
}

// MessageHandler must handle incoming async message from client.
type MessageHandler func(MessageEvent) MessageReply
