package conns

import (
	"github.com/centrifugal/centrifugo/lib/proto"
)

// Client interface contains functions to inspect and control client
// connection.
type Client interface {
	// ID returns unique connection id.
	ID() string
	// User return user ID associated with connection.
	UserID() string
	// Channels returns a slice of channels connection subscribed to at moment.
	Channels() []string
	// TransportName returns name of transport used.
	Transport() Transport
	// Handle data coming from connection transport.
	Handle(*proto.Command) (*proto.Reply, *proto.Disconnect)
	// Unsubscribe allows to unsubscribe connection from channel.
	Unsubscribe(channel string) error
	// Close closes client's connection.
	Close(*proto.Disconnect) error
}
