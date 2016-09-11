package server

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Session represents a connection between server and client.
type session interface {
	// Send sends one message to session
	Send([]byte) error
	// Close closes the session with provided code and reason.
	Close(status uint32, reason string) error
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
			sess.Close(CloseStatus, "write ping error")
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

func (sess *wsSession) Close(status uint32, reason string) error {
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
	msg := websocket.FormatCloseMessage(int(status), reason)
	sess.ws.WriteControl(websocket.CloseMessage, msg, deadline)
	return sess.ws.Close()
}
