package conns

import (
	"github.com/centrifugal/centrifugo/lib/proto"
)

// options contains various connection specific options.
// TODO: ?
type options struct {
	// Hidden allows to hide connection from public visibility - i.e.
	// connection  will be hidden from other clients - no join/leave events
	// will be sent for it, connection will not be part of presence information.
	Hidden bool `json:"hidden,omitempty"`
}

// Client represents functions to inspect and control client connection.
type Client interface {
	// Encoding returns connection protocol encoding.
	Encoding() proto.Encoding
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
