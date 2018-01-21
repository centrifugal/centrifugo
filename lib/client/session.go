package client

import (
	"github.com/centrifugal/centrifugo/lib/proto"
)

// Session represents a connection transport between server and client.
type Session interface {
	// Send sends data to session.
	Send(data []byte) error
	// Close closes the session with provided code and reason.
	Close(*proto.Disconnect) error
}
