package centrifuge

import (
	"context"
)

// Mediator allows to proxy Centrifugo events to Go application code.
type Mediator struct {
	// Connect called every time client connects to node.
	Connect ConnectHandler
	// Disconnect called when client disconnected.
	Disconnect DisconnectHandler
	// Subscribe called when client subscribes on channel.
	Subscribe SubscribeHandler
	// Unsubscribe called when client unsubscribes from channel.
	Unsubscribe UnsubscribeHandler
	// Publish called when client publishes message into channel.
	Publish PublishHandler
	// Presence allows to register action to be executed on every
	// periodic client connection presence update.
	Presence PresenceHandler
	// Refresh called when it's time to refresh connection credentials.
	Refresh RefreshHandler
	// RPC allows to register custom logic on incoming RPC calls.
	RPC RPCHandler
	// Message called when client sent asynchronous message.
	Message MessageHandler
}

// Event added to all specific event contexts.
type Event struct {
	Client Client
}

// ConnectEvent contains fields related to connect event.
type ConnectEvent struct {
	Event
}

// ConnectReply contains fields determining the reaction on connect event.
type ConnectReply struct {
	Error      *Error
	Disconnect *Disconnect
}

// ConnectHandler ...
type ConnectHandler func(context.Context, ConnectEvent) ConnectReply

// DisconnectEvent contains fields related to disconnect event.
type DisconnectEvent struct {
	Event
	Disconnect *Disconnect
}

// DisconnectReply contains fields determining the reaction on disconnect event.
type DisconnectReply struct{}

// DisconnectHandler ...
type DisconnectHandler func(context.Context, DisconnectEvent) DisconnectReply

// SubscribeEvent contains fields related to subscribe event.
type SubscribeEvent struct {
	Event
	Channel string
}

// SubscribeReply contains fields determining the reaction on subscribe event.
type SubscribeReply struct {
	Error       *Error
	Disconnect  *Disconnect
	ChannelInfo Raw
}

// SubscribeHandler ...
type SubscribeHandler func(context.Context, SubscribeEvent) SubscribeReply

// UnsubscribeEvent contains fields related to unsubscribe event.
type UnsubscribeEvent struct {
	Event
	Channel string
}

// UnsubscribeReply contains fields determining the reaction on unsubscribe event.
type UnsubscribeReply struct {
}

// UnsubscribeHandler ...
type UnsubscribeHandler func(context.Context, UnsubscribeEvent) UnsubscribeReply

// PublishEvent contains fields related to publish event.
type PublishEvent struct {
	Event
	Channel string
	Pub     *Pub
}

// PublishReply contains fields determining the reaction on publish event.
type PublishReply struct {
	Error      *Error
	Disconnect *Disconnect
}

// PublishHandler ...
type PublishHandler func(context.Context, PublishEvent) PublishReply

// PresenceEvent contains fields related to presence update event.
type PresenceEvent struct {
	Event
}

// PresenceReply contains fields determining the reaction on presence update event.
type PresenceReply struct {
	Disconnect *Disconnect
}

// PresenceHandler ...
type PresenceHandler func(context.Context, PresenceEvent) PresenceReply

// RefreshEvent contains fields related to refresh event.
type RefreshEvent struct {
	Event
}

// RefreshReply contains fields determining the reaction on refresh event.
type RefreshReply struct {
	Exp  int64
	Info []byte
}

// RefreshHandler ...
type RefreshHandler func(context.Context, RefreshEvent) RefreshReply

// RPCEvent contains fields related to rpc request.
type RPCEvent struct {
	Event
	Data Raw
}

// RPCReply contains fields determining the reaction on rpc request.
type RPCReply struct {
	Error      *Error
	Disconnect *Disconnect
	Data       Raw
}

// RPCHandler must handle incoming command from client.
type RPCHandler func(context.Context, RPCEvent) RPCReply

// MessageEvent contains fields related to message request.
type MessageEvent struct {
	Event
	Data Raw
}

// MessageReply contains fields determining the reaction on message request.
type MessageReply struct {
	Disconnect *Disconnect
}

// MessageHandler must handle incoming async message from client.
type MessageHandler func(context.Context, MessageEvent) MessageReply
