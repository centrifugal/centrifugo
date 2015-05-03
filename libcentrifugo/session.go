package libcentrifugo

// Session represents a connection between server and client.
type session interface {
	// Send sends one text frame to session
	Send(string) error
	// Close closes the session with provided code and reason.
	Close(status uint32, reason string) error
}
