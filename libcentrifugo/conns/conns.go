package conns

import (
	"encoding/json"
	"sync"

	"github.com/gorilla/websocket"
)

// QueuedMessage is a wrapper structure over raw data payload.
// It can optionally contain prepared websocket message (to drastically
// reduce overhead of framing in case of using websocket compression).
type QueuedMessage struct {
	// Payload is raw message payload (encoded in JSON).
	Payload []byte
	// UsePrepared can be used in session Send method to determine which
	// underlying transport write method to use. At moment used for websocket
	// session only - so we can call WriteMessage or WritePreparedMessage
	// connection methods.
	UsePrepared bool
	prepared    *websocket.PreparedMessage
	once        sync.Once
}

// NewQueuedMessage initializes QueuedMessage.
func NewQueuedMessage(payload []byte, usePrepared bool) *QueuedMessage {
	m := &QueuedMessage{
		Payload:     payload,
		UsePrepared: usePrepared,
	}
	return m
}

// Prepared allows to get PreparedMessage for raw websocket connections. It
// constructs PreparedMessage lazily after first call.
func (m *QueuedMessage) Prepared() *websocket.PreparedMessage {
	m.once.Do(func() {
		pm, _ := websocket.NewPreparedMessage(websocket.TextMessage, m.Payload)
		m.prepared = pm
	})
	return m.prepared
}

// Len returns length of QueuedMessage payload so QueuedMessage implements
// item that can be queued into our unbounded queue.
func (m *QueuedMessage) Len() int {
	return len(m.Payload)
}

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
	Send(*QueuedMessage) error
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
	Send(*QueuedMessage) error
	// Close closes admin's connection.
	Close(*DisconnectAdvice) error
}

// Session represents a connection transport between server and client.
type Session interface {
	// Send sends one message to session
	Send(*QueuedMessage) error
	// Close closes the session with provided code and reason.
	Close(*DisconnectAdvice) error
}
