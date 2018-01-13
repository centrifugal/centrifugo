package node

import (
	"github.com/centrifugal/centrifugo/lib/proto"
	clientproto "github.com/centrifugal/centrifugo/lib/proto/client"
)

// Client is an interface abstracting all methods used
// by application to interact with client connection.
type Client interface {
	// Encoding returns connection protocol encoding.
	Encoding() clientproto.Encoding
	// UID returns unique connection id.
	UID() string
	// User return user ID associated with connection.
	User() string
	// Channels returns a slice of channels connection subscribed to.
	Channels() []string
	// Handle message coming from client.
	Handle(data []byte) error
	// Send allows to send prepared message payload to connection client.
	Send(data []byte) error
	// Unsubscribe allows to unsubscribe connection from channel.
	Unsubscribe(channel string) error
	// Close closes client's connection.
	Close(*proto.Disconnect) error
}
