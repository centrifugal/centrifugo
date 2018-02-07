package server

import (
	"encoding/json"
	"sync"

	"github.com/centrifugal/centrifugo/lib/proto"

	"github.com/igm/sockjs-go/sockjs"
)

const (
	// We don't use specific websocket close codes because our client
	// have no notion about transport specifics.
	sockjsCloseStatus = 3000
)

type sockjsTransport struct {
	mu      sync.RWMutex
	closed  bool
	closeCh chan struct{}
	session sockjs.Session
	writer  *writer
}

func newSockjsTransport(s sockjs.Session, w *writer) *sockjsTransport {
	t := &sockjsTransport{
		session: s,
		writer:  w,
		closeCh: make(chan struct{}),
	}
	w.onWrite(t.write)
	return t
}

func (t *sockjsTransport) Name() string {
	return "sockjs"
}

func (t *sockjsTransport) Send(reply *proto.PreparedReply) error {
	data := reply.Data()
	disconnect := t.writer.write(data)
	if disconnect != nil {
		// Close in goroutine to not block message broadcast.
		go t.Close(disconnect)
	}
	return nil
}

func (t *sockjsTransport) write(data []byte) error {
	select {
	case <-t.closeCh:
		return nil
	default:
		transportMessagesSent.WithLabelValues("sockjs").Inc()
		transportBytesOut.WithLabelValues("sockjs").Add(float64(len(data)))
		err := t.session.Send(string(data))
		if err != nil {
			t.Close(&proto.Disconnect{Reason: "error sending message", Reconnect: true})
		}
		return err
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
