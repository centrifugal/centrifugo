package unihttpstream

import (
	"net/http"
	"sync"

	"github.com/centrifugal/centrifuge"
)

type streamTransport struct {
	mu             sync.Mutex
	req            *http.Request
	messages       chan [][]byte
	disconnectCh   chan *centrifuge.Disconnect
	closedCh       chan struct{}
	closed         bool
	pingPongConfig centrifuge.PingPongConfig
}

func newStreamTransport(req *http.Request, pingPongConfig centrifuge.PingPongConfig) *streamTransport {
	return &streamTransport{
		messages:       make(chan [][]byte),
		disconnectCh:   make(chan *centrifuge.Disconnect),
		closedCh:       make(chan struct{}),
		req:            req,
		pingPongConfig: pingPongConfig,
	}
}

const transportName = "uni_http_stream"

func (t *streamTransport) Name() string {
	return transportName
}

func (t *streamTransport) Protocol() centrifuge.ProtocolType {
	return centrifuge.ProtocolTypeJSON
}

// ProtocolVersion returns transport protocol version.
func (t *streamTransport) ProtocolVersion() centrifuge.ProtocolVersion {
	return centrifuge.ProtocolVersion2
}

// Unidirectional returns whether transport is unidirectional.
func (t *streamTransport) Unidirectional() bool {
	return true
}

// DisabledPushFlags ...
func (t *streamTransport) DisabledPushFlags() uint64 {
	return 0
}

// PingPongConfig ...
func (t *streamTransport) PingPongConfig() centrifuge.PingPongConfig {
	return t.pingPongConfig
}

// Emulation ...
func (t *streamTransport) Emulation() bool {
	return false
}

func (t *streamTransport) Write(message []byte) error {
	return t.WriteMany(message)
}

func (t *streamTransport) WriteMany(messages ...[]byte) error {
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

func (t *streamTransport) Close(_ centrifuge.Disconnect) error {
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
