package conns

import (
	"github.com/centrifugal/centrifugo/lib/proto"
)

// Transport abstracts a connection transport between server and client.
type Transport interface {
	// Name returns a name of transport used for client connection.
	Name() string
	// Encoding returns transport encoding used.
	Encoding() proto.Encoding
	// Send sends data to session.
	Send(*proto.PreparedReply) error
	// Close closes the session with provided code and reason.
	Close(*proto.Disconnect) error
}
