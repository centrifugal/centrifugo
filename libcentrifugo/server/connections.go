package server

import (
	"github.com/centrifugal/centrifugo/libcentrifugo/proto"
)

// clientConn is an interface abstracting all methods used
// by application to interact with client connection
type clientConn interface {
	// UID returns unique connection id.
	UID() proto.ConnID
	// User return user ID associated with connection.
	User() proto.UserID
	// Channels returns a slice of channels connection subscribed to.
	Channels() []proto.Channel
	// Send allows to send message to connection client.
	Send(message []byte) error
	// Unsubscribe allows to unsubscribe connection from channel.
	Unsubscribe(ch proto.Channel) error
	// Close closes client's connection.
	Close(reason string) error
}

// adminConn is an interface abstracting all methods used
// by application to interact with admin connection.
type adminConn interface {
	// UID returns unique admin connection id.
	UID() proto.ConnID
	// Send allows to send message to admin connection.
	Send(message []byte) error
	// Close closes admin's connection.
	Close(reason string) error
}
