package server

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/FZambia/websocket"
	"github.com/centrifugal/centrifugo/lib/logger"
	"github.com/centrifugal/centrifugo/lib/proto"
)

const (
	// We don't use specific websocket close codes because our client
	// does not know transport specifics.
	websocketCloseStatus = 3000
)

// websocketConn is an interface to mimic gorilla/websocket methods we use
// in Centrifugo. Needed only to simplify websocketConn struct tests.
// Description can be found in gorilla websocket docs:
// https://godoc.org/github.com/gorilla/websocket.
type websocketConn interface {
	ReadMessage() (messageType int, p []byte, err error)
	WriteMessage(messageType int, data []byte) error
	WriteControl(messageType int, data []byte, deadline time.Time) error
	SetWriteDeadline(t time.Time) error
	EnableWriteCompression(enable bool)
	Close() error
}

// websocketTransport is a wrapper struct over websocket connection to fit session
// interface so client will accept it.
type websocketTransport struct {
	mu        sync.RWMutex
	conn      websocketConn
	closed    bool
	closeCh   chan struct{}
	opts      *websocketTransportOptions
	pingTimer *time.Timer
}

type websocketTransportOptions struct {
	pingInterval       time.Duration
	writeTimeout       time.Duration
	compressionMinSize int
}

func newWebsocketTransport(conn websocketConn, opts *websocketTransportOptions) *websocketTransport {
	sess := &websocketTransport{
		conn:    conn,
		closeCh: make(chan struct{}),
		opts:    opts,
	}
	if opts.pingInterval > 0 {
		sess.addPing()
	}
	return sess
}

func (t *websocketTransport) ping() {
	select {
	case <-t.closeCh:
		return
	default:
		deadline := time.Now().Add(t.opts.pingInterval / 2)
		err := t.conn.WriteControl(websocket.PingMessage, []byte("ping"), deadline)
		if err != nil {
			logger.ERROR.Printf("Error write ping: %v", err)
			t.Close(proto.DisconnectServerError)
			return
		}
		t.addPing()
	}
}

func (t *websocketTransport) addPing() {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return
	}
	t.pingTimer = time.AfterFunc(t.opts.pingInterval, t.ping)
	t.mu.Unlock()
}

func (t *websocketTransport) Name() string {
	return "websocket"
}

func (t *websocketTransport) Send(msg []byte) error {
	select {
	case <-t.closeCh:
		return nil
	default:
		if t.opts.compressionMinSize > 0 {
			t.conn.EnableWriteCompression(len(msg) > t.opts.compressionMinSize)
		}
		if t.opts.writeTimeout > 0 {
			t.conn.SetWriteDeadline(time.Now().Add(t.opts.writeTimeout))
		}
		var err error
		err = t.conn.WriteMessage(websocket.TextMessage, msg)

		if t.opts.writeTimeout > 0 {
			t.conn.SetWriteDeadline(time.Time{})
		}
		return err
	}
}

func (t *websocketTransport) Close(disconnect *proto.Disconnect) error {
	t.mu.Lock()
	if t.closed {
		// Already closed, noop.
		t.mu.Unlock()
		return nil
	}
	close(t.closeCh)
	t.closed = true
	if t.pingTimer != nil {
		t.pingTimer.Stop()
	}
	t.mu.Unlock()
	if disconnect != nil {
		deadline := time.Now().Add(time.Second)
		reason, err := json.Marshal(disconnect)
		if err != nil {
			return err
		}
		msg := websocket.FormatCloseMessage(websocketCloseStatus, string(reason))
		t.conn.WriteControl(websocket.CloseMessage, msg, deadline)
		return t.conn.Close()
	}
	return t.conn.Close()
}
