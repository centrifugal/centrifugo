package conns

import (
	"encoding/json"
	"sync"
)

// DisconnectAdvice sent to client when we want it to gracefully disconnect.
type DisconnectAdvice struct {
	mu        sync.RWMutex
	Reason    string `json:"reason"`
	Reconnect bool   `json:"reconnect"`
	jsonified string
}

// JSONString contains cached representation of DisconnectAdvice as JSON.
func (a *DisconnectAdvice) JSONString() (string, error) {
	a.mu.RLock()
	if a.jsonified != "" {
		a.mu.RUnlock()
		return a.jsonified, nil
	}
	a.mu.RUnlock()

	a.mu.Lock()
	defer a.mu.Unlock()
	b, err := json.Marshal(a)
	if err != nil {
		return "", err
	}
	a.jsonified = string(b)
	return a.jsonified, nil
}

// DefaultDisconnectAdvice is no reason and reconnect.
var DefaultDisconnectAdvice = &DisconnectAdvice{Reason: "", Reconnect: true}

// ClientConn is an interface abstracting all methods used
// by application to interact with client connection.
type ClientConn interface {
	// UID returns unique connection id.
	UID() string
	// User return user ID associated with connection.
	User() string
	// Channels returns a slice of channels connection subscribed to.
	Channels() []string
	// Handle message coming from client.
	Handle(message []byte) error
	// Send allows to send message to connection client.
	Send(message []byte) error
	// Unsubscribe allows to unsubscribe connection from channel.
	Unsubscribe(ch string) error
	// Close closes client's connection.
	Close(*DisconnectAdvice) error
}

// AdminConn is an interface abstracting all methods used
// by application to interact with admin connection.
type AdminConn interface {
	// UID returns unique admin connection id.
	UID() string
	// Handle message coming from admin client.
	Handle(message []byte) error
	// Send allows to send message to admin connection.
	Send(message []byte) error
	// Close closes admin's connection.
	Close(*DisconnectAdvice) error
}

// Session represents a connection transport between server and client.
type Session interface {
	// Send sends one message to session
	Send([]byte) error
	// Close closes the session with provided code and reason.
	Close(*DisconnectAdvice) error
}
