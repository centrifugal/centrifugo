package httpserver

import (
	"sync"
	"time"

	"github.com/centrifugal/centrifugo/libcentrifugo/conns"
	"github.com/gorilla/websocket"
	"gopkg.in/igm/sockjs-go.v2/sockjs"
)

const (
	// CloseStatus is status code set when closing client connections.
	CloseStatus = 3000
)

type sockjsSession struct {
	mu     sync.RWMutex
	closed bool
	sess   sockjs.Session
}

func newSockjsSession(sess sockjs.Session) *sockjsSession {
	return &sockjsSession{
		sess: sess,
	}
}

func (sess *sockjsSession) Send(msg []byte) error {
	return sess.sess.Send(string(msg))
}

func (sess *sockjsSession) Close(advice *conns.DisconnectAdvice) error {
	sess.mu.Lock()
	defer sess.mu.Unlock()
	if sess.closed {
		// Already closed, noop.
		return nil
	}
	sess.closed = true
	// Give polling client a chance to receive and process buffered messages.
	time.Sleep(time.Second)
	return sess.sess.Close(CloseStatus, advice.Reason)
}

// websocketConn is an interface to mimic gorilla/websocket methods we use in Centrifugo.
// Needed only to simplify wsConn struct tests. Description can be found in gorilla websocket docs:
// https://godoc.org/github.com/gorilla/websocket.
type websocketConn interface {
	ReadMessage() (messageType int, p []byte, err error)
	WriteMessage(messageType int, data []byte) error
	WriteControl(messageType int, data []byte, deadline time.Time) error
	Close() error
}

// wsSession is a wrapper struct over websocket connection to fit session interface so
// client will accept it.
type wsSession struct {
	mu           sync.RWMutex
	ws           websocketConn
	closed       bool
	closeCh      chan struct{}
	pingInterval time.Duration
	pingTimer    *time.Timer
}

func newWSSession(ws websocketConn, pingInterval time.Duration) *wsSession {
	sess := &wsSession{
		ws:           ws,
		closeCh:      make(chan struct{}),
		pingInterval: pingInterval,
	}
	sess.addPing()
	return sess
}

func (sess *wsSession) ping() {
	select {
	case <-sess.closeCh:
		return
	default:
		deadline := time.Now().Add(sess.pingInterval / 2)
		err := sess.ws.WriteControl(websocket.PingMessage, []byte("ping"), deadline)
		if err != nil {
			sess.Close(&conns.DisconnectAdvice{"write ping error", true})
			return
		}
		sess.addPing()
	}
}

func (sess *wsSession) addPing() {
	sess.mu.Lock()
	if sess.closed {
		sess.mu.Unlock()
		return
	}
	sess.pingTimer = time.AfterFunc(sess.pingInterval, sess.ping)
	sess.mu.Unlock()
}

func (sess *wsSession) Send(msg []byte) error {
	select {
	case <-sess.closeCh:
		return nil
	default:
		return sess.ws.WriteMessage(websocket.TextMessage, msg)
	}
}

func (sess *wsSession) Close(advice *conns.DisconnectAdvice) error {
	sess.mu.Lock()
	if sess.closed {
		// Already closed, noop.
		sess.mu.Unlock()
		return nil
	}
	close(sess.closeCh)
	sess.closed = true
	sess.pingTimer.Stop()
	sess.mu.Unlock()
	deadline := time.Now().Add(time.Second)
	msg := websocket.FormatCloseMessage(int(CloseStatus), advice.Reason)
	sess.ws.WriteControl(websocket.CloseMessage, msg, deadline)
	return sess.ws.Close()
}
