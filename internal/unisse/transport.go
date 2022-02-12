package unisse

import (
	"net/http"
	"sync"
	"time"

	"github.com/centrifugal/centrifuge"
)

type eventsourceTransport struct {
	mu           sync.Mutex
	req          *http.Request
	messages     chan []byte
	disconnectCh chan *centrifuge.Disconnect
	closedCh     chan struct{}
	closed       bool
	protoVersion centrifuge.ProtocolVersion
}

func newEventsourceTransport(req *http.Request, protoVersion centrifuge.ProtocolVersion) *eventsourceTransport {
	return &eventsourceTransport{
		messages:     make(chan []byte),
		disconnectCh: make(chan *centrifuge.Disconnect),
		closedCh:     make(chan struct{}),
		req:          req,
		protoVersion: protoVersion,
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
	return t.protoVersion
}

// Unidirectional returns whether transport is unidirectional.
func (t *eventsourceTransport) Unidirectional() bool {
	return true
}

// DisabledPushFlags ...
func (t *eventsourceTransport) DisabledPushFlags() uint64 {
	return 0
}

// AppLevelPing ...
func (t *eventsourceTransport) AppLevelPing() centrifuge.AppLevelPing {
	return centrifuge.AppLevelPing{
		PingInterval: 25 * time.Second,
	}
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
	for i := 0; i < len(messages); i++ {
		select {
		case t.messages <- messages[i]:
		case <-t.closedCh:
			return nil
		}
	}
	return nil
}

func (t *eventsourceTransport) Close(_ *centrifuge.Disconnect) error {
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
