package websocket

import (
	"github.com/centrifugal/centrifugo/v6/internal/messagefilter"
	"github.com/centrifugal/centrifuge"
)

// FilteredWebSocketTransport wraps a WebSocket transport to add message filtering
type FilteredWebSocketTransport struct {
	transport         centrifuge.Transport
	messageInterceptor *messagefilter.MessageInterceptor
	clientID          string
}

// NewFilteredWebSocketTransport creates a new filtered WebSocket transport
func NewFilteredWebSocketTransport(transport centrifuge.Transport, interceptor *messagefilter.MessageInterceptor, clientID string) *FilteredWebSocketTransport {
	return &FilteredWebSocketTransport{
		transport:         transport,
		messageInterceptor: interceptor,
		clientID:          clientID,
	}
}

// Name returns the transport name
func (t *FilteredWebSocketTransport) Name() string {
	return t.transport.Name()
}

// Protocol returns the transport protocol
func (t *FilteredWebSocketTransport) Protocol() centrifuge.ProtocolType {
	return t.transport.Protocol()
}

// ProtocolVersion returns the transport protocol version
func (t *FilteredWebSocketTransport) ProtocolVersion() centrifuge.ProtocolVersion {
	return t.transport.ProtocolVersion()
}

// Unidirectional returns whether the transport is unidirectional
func (t *FilteredWebSocketTransport) Unidirectional() bool {
	return t.transport.Unidirectional()
}

// DisabledPushFlags returns disabled push flags
func (t *FilteredWebSocketTransport) DisabledPushFlags() uint64 {
	return t.transport.DisabledPushFlags()
}

// PingPongConfig returns ping pong configuration
func (t *FilteredWebSocketTransport) PingPongConfig() centrifuge.PingPongConfig {
	return t.transport.PingPongConfig()
}

// Emulation returns whether emulation is enabled
func (t *FilteredWebSocketTransport) Emulation() bool {
	return t.transport.Emulation()
}

// Write writes a message to the transport
func (t *FilteredWebSocketTransport) Write(message []byte) error {
	// Check if this is a publication message that should be filtered
	if t.messageInterceptor != nil {
		// For now, we'll pass all messages through
		// The actual filtering will be done at the subscription level
		return t.transport.Write(message)
	}
	return t.transport.Write(message)
}

// WriteMany writes multiple messages to the transport
func (t *FilteredWebSocketTransport) WriteMany(messages ...[]byte) error {
	// For now, we'll pass all messages through
	// The actual filtering will be done at the subscription level
	return t.transport.WriteMany(messages...)
}

// Close closes the transport
func (t *FilteredWebSocketTransport) Close(disconnect centrifuge.Disconnect) error {
	return t.transport.Close(disconnect)
} 