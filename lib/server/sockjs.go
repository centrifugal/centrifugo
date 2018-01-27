package server

import (
	"encoding/json"
	"sync"

	"github.com/centrifugal/centrifugo/lib/proto"

	"github.com/igm/sockjs-go/sockjs"
)

const (
	// We don't use specific websocket close codes because our client
	// does not know transport specifics.
	sockjsCloseStatus = 3000
)

type sockjsTransport struct {
	mu      sync.RWMutex
	closed  bool
	closeCh chan struct{}
	session sockjs.Session
}

func newSockjsTransport(s sockjs.Session) *sockjsTransport {
	return &sockjsTransport{
		session: s,
		closeCh: make(chan struct{}),
	}
}

func (t *sockjsTransport) Name() string {
	return "sockjs"
}

func (t *sockjsTransport) Send(msg []byte) error {
	select {
	case <-t.closeCh:
		return nil
	default:
		return t.session.Send(string(msg))
	}
}

func (t *sockjsTransport) Close(disconnect *proto.Disconnect) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		// Already closed, noop.
		return nil
	}
	t.closed = true
	close(t.closeCh)
	if disconnect == nil {
		disconnect = proto.DisconnectNormal
	}
	reason, err := json.Marshal(disconnect)
	if err != nil {
		return err
	}
	return t.session.Close(sockjsCloseStatus, string(reason))
}
