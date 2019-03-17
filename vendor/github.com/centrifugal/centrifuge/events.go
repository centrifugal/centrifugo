package centrifuge

import (
	"context"
)

// ConnectEvent contains fields related to connect event.
type ConnectEvent struct {
	Data Raw
}

// ConnectReply contains fields determining the reaction on connect event.
type ConnectReply struct {
	Error      *Error
	Disconnect *Disconnect
	Data       Raw
}

// ConnectHandler called when new client connects to server.
type ConnectHandler func(context.Context, *Client, ConnectEvent) ConnectReply

// DisconnectEvent contains fields related to disconnect event.
type DisconnectEvent struct {
	Disconnect *Disconnect
}

// DisconnectReply contains fields determining the reaction on disconnect event.
type DisconnectReply struct{}

// DisconnectHandler called when client disconnects from server.
type DisconnectHandler func(DisconnectEvent) DisconnectReply

// SubscribeEvent contains fields related to subscribe event.
type SubscribeEvent struct {
	Channel string
}

// SubscribeReply contains fields determining the reaction on subscribe event.
type SubscribeReply struct {
	Error       *Error
	Disconnect  *Disconnect
	ExpireAt    int64
	ChannelInfo Raw
}

// SubscribeHandler called when client wants to subscribe on channel.
type SubscribeHandler func(SubscribeEvent) SubscribeReply

// UnsubscribeEvent contains fields related to unsubscribe event.
type UnsubscribeEvent struct {
	Channel string
}

// UnsubscribeReply contains fields determining the reaction on unsubscribe event.
type UnsubscribeReply struct {
}

// UnsubscribeHandler called when client unsubscribed from channel.
type UnsubscribeHandler func(UnsubscribeEvent) UnsubscribeReply

// PublishEvent contains fields related to publish event.
type PublishEvent struct {
	Channel string
	Data    Raw
	Info    *ClientInfo
}

// PublishReply contains fields determining the reaction on publish event.
type PublishReply struct {
	Error      *Error
	Disconnect *Disconnect
}

// PublishHandler called when client publishes into channel.
type PublishHandler func(PublishEvent) PublishReply

// RefreshEvent contains fields related to refresh event.
type RefreshEvent struct{}

// RefreshReply contains fields determining the reaction on refresh event.
type RefreshReply struct {
	ExpireAt int64
	Info     Raw
}

// RefreshHandler called when it's time to validate client connection and
// update it's expiration time.
type RefreshHandler func(RefreshEvent) RefreshReply

// SubRefreshEvent contains fields related to subscription refresh event.
type SubRefreshEvent struct {
	Channel string
}

// SubRefreshReply contains fields determining the reaction on
// subscription refresh event.
type SubRefreshReply struct {
	Expired  bool
	ExpireAt int64
	Info     Raw
}

// SubRefreshHandler called when it's time to validate client subscription to channel and
// update it's state if needed.
type SubRefreshHandler func(SubRefreshEvent) SubRefreshReply

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
