package libcentrifugo

import (
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
	ws           websocketConn
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
	sess.pingTimer = time.AfterFunc(sess.pingInterval, sess.ping)
	return sess
}

// TODO: data race detector complains here as we call ping from different goroutines –
// no real trouble as we do this periodically but better to fix anyway.
func (sess *wsSession) ping() {
	select {
	case <-sess.closeCh:
		sess.pingTimer.Stop()
		return
	default:
		deadline := time.Now().Add(sess.pingInterval / 2)
		err := sess.ws.WriteControl(websocket.PingMessage, []byte("ping"), deadline)
		if err != nil {
			sess.ws.Close()
			sess.pingTimer.Stop()
			return
		}
		sess.pingTimer = time.AfterFunc(sess.pingInterval, sess.ping)
	}
}

func (sess *wsSession) Send(message []byte) error {
	select {
	case <-sess.closeCh:
		return nil
	default:
		return sess.ws.WriteMessage(websocket.TextMessage, message)
	}
}

func (sess *wsSession) Close(status uint32, reason string) error {
	deadline := time.Now().Add(time.Second)
	msg := websocket.FormatCloseMessage(int(status), reason)
	sess.ws.WriteControl(websocket.CloseMessage, msg, deadline)
	return sess.ws.Close()
}
