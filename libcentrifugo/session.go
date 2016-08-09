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

// wsSession is a wrapper struct over websocket connection to fit session interface so client will accept it.
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

func (sess *wsSession) ping() {
	select {
	case <-sess.closeCh:
		sess.pingTimer.Stop()
		return
	default:
		err := sess.ws.WriteControl(websocket.PingMessage, []byte("ping"), time.Now().Add(sess.pingInterval/2))
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
	sess.ws.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(int(status), reason), time.Now().Add(time.Second))
	return sess.ws.Close()
}
