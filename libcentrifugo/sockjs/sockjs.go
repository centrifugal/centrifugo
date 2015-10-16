package sockjs

import "net/http"

// Session represents a connection between server and client.
type Session interface {
	// Id returns a session id
	ID() string
	// Request returns the first http request
	Request() *http.Request
	// Recv reads one frame from session
	Recv() ([]byte, error)
	// Send sends one frame to session
	Send([]byte) error
	// Close closes the session with provided code and reason.
	Close(status uint32, reason string) error
	//Gets the state of the session. SessionOpening/SessionActive/SessionClosing/SessionClosed;
	GetSessionState() sessionState
}
