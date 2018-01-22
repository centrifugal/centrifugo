package client

import (
	"github.com/centrifugal/centrifugo/lib/proto"
)

// StateReader used to get various connection specific data.
type StateReader interface {
	// Encoding returns connection protocol encoding.
	Encoding() proto.Encoding
	// ID returns unique connection id.
	ID() string
	// User return user ID associated with connection.
	UserID() string
	// Channels returns a slice of channels connection subscribed to at moment.
	Channels() []string
	// Transport returns name of transport used.
	Transport() string
}

// ActionPerformer used to perform actions over connection from outside.
type ActionPerformer interface {
	// Handle data coming from connection transport.
	Handle(data []byte) error
	// Send data to connection transport.
	Send(data []byte) error
	// Unsubscribe allows to unsubscribe connection from channel.
	Unsubscribe(channel string) error
	// Close closes client's connection.
	Close(*proto.Disconnect) error
}

// Conn is an interface abstracting all methods used
// by application to interact with client connection.
type Conn interface {
	StateReader
	ActionPerformer
}
