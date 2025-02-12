package unisse

import (
	"net/http"
	"sync"

	"github.com/centrifugal/centrifuge"
)

type eventsourceTransport struct {
	mu             sync.Mutex
	req            *http.Request
	messages       chan [][]byte
	disconnectCh   chan *centrifuge.Disconnect
	closedCh       chan struct{}
	closed         bool
	pingPongConfig centrifuge.PingPongConfig
}

func newEventsourceTransport(req *http.Request, pingPongConfig centrifuge.PingPongConfig) *eventsourceTransport {
	return &eventsourceTransport{
		messages:       make(chan [][]byte),
		disconnectCh:   make(chan *centrifuge.Disconnect),
		closedCh:       make(chan struct{}),
		req:            req,
		pingPongConfig: pingPongConfig,
	}
}

const transportName = "uni_sse"

func (t *eventsourceTransport) Name() string {
	return transportName
}

func (t *eventsourceTransport) Protocol() centrifuge.ProtocolType {
	return centrifuge.ProtocolTypeJSON
}

// ProtocolVersion returns transport protocol version.
func (t *eventsourceTransport) ProtocolVersion() centrifuge.ProtocolVersion {
	return centrifuge.ProtocolVersion2
}

// Unidirectional returns whether transport is unidirectional.
func (t *eventsourceTransport) Unidirectional() bool {
	return true
}

// DisabledPushFlags ...
func (t *eventsourceTransport) DisabledPushFlags() uint64 {
	return 0
}

// PingPongConfig ...
func (t *eventsourceTransport) PingPongConfig() centrifuge.PingPongConfig {
	return t.pingPongConfig
}

// Emulation ...
func (t *eventsourceTransport) Emulation() bool {
	return false
}

func (t *eventsourceTransport) Write(message []byte) error {
	return t.WriteMany(message)
}

func (t *eventsourceTransport) WriteMany(messages ...[]byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return nil
	}
	select {
	case t.messages <- messages:
	case <-t.closedCh:
		return nil
	}
	return nil
}

func (t *eventsourceTransport) Close(_ centrifuge.Disconnect) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return nil
	}
	t.closed = true
	close(t.disconnectCh)
	<-t.closedCh
	return nil
}
