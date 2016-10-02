package node

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Session represents a connection between server and client.
type session interface {
	// Send sends one message to session
	Send([]byte) error
	// Close closes the session with provided code and reason.
	Close(status uint32, reason string) error
}
