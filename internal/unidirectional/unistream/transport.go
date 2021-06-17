package unistream

import (
	"net/http"
	"sync"

	"github.com/centrifugal/centrifuge"
)

type streamTransport struct {
	mu           sync.Mutex
	req          *http.Request
	messages     chan []byte
	disconnectCh chan *centrifuge.Disconnect
	closedCh     chan struct{}
	closed       bool
}

func newStreamTransport(req *http.Request) *streamTransport {
	return &streamTransport{
		messages:     make(chan []byte),
		disconnectCh: make(chan *centrifuge.Disconnect),
		closedCh:     make(chan struct{}),
		req:          req,
	}
}

func (t *streamTransport) Name() string {
	return "uni_stream"
}

func (t *streamTransport) Protocol() centrifuge.ProtocolType {
	return centrifuge.ProtocolTypeJSON
}

// Unidirectional returns whether transport is unidirectional.
func (t *streamTransport) Unidirectional() bool {
	return true
}

// DisabledPushFlags ...
func (t *streamTransport) DisabledPushFlags() uint64 {
	return 0
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
	for i := 0; i < len(messages); i++ {
		select {
		case t.messages <- messages[i]:
		case <-t.closedCh:
			return nil
		}
	}
	return nil
}

func (t *streamTransport) Close(_ *centrifuge.Disconnect) error {
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
