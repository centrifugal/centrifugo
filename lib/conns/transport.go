package conns

import (
	"github.com/centrifugal/centrifugo/lib/proto"
)

/*
Client should know nothing about encoding

Client should look sth like:

type Client {
	// ID returns unique connection id.
	ID() string
	// User return user ID associated with connection.
	UserID() string
	// Channels returns a slice of channels connection subscribed to at moment.
	Channels() []string
	// TransportName returns name of transport used.
	TransportName() string
	// TransportEncoding returns transport protocol encoding.
	TransportEncoding() proto.Encoding

	Connect(ConnectRequest) ConnectResponse
	Subscribe(SubscribeRequest) SubscribeResponse
	Unsubscribe(UnsubscribeRequest) UnsubscribeResponse
	Refresh(RefreshRequest) RefreshResponse
	RPC (RPCRequest) RPCResponse
}

Транспорт как-то так:

type Transport interface {
	Name() string

	Encoding() proto.Encoding

	// Handle data coming from connection.
	Handle(data []byte) error

	// Send data to connection transport.
	Send(data []byte) error

	Close(*proto.Disconnect) error
}

*/

// Transport abstracts a connection transport between server and client.
type Transport interface {
	// Name returns a name of transport used for client connection.
	Name() string
	// Send sends data to session.
	Send(*proto.PreparedReply) error
	// Close closes the session with provided code and reason.
	Close(*proto.Disconnect) error
}
