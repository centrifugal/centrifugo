package httpserver

import (
	"sync"
	"time"

	"github.com/centrifugal/centrifugo/libcentrifugo/conns"
	"github.com/gorilla/websocket"
	"github.com/igm/sockjs-go/sockjs"
)

const (
	// CloseStatus is status code set when closing client connections.
	CloseStatus = 3000
)

type sockjsSession struct {
	mu      sync.RWMutex
	closed  bool
	closeCh chan struct{}
	sess    sockjs.Session
}

func newSockjsSession(sess sockjs.Session) *sockjsSession {
	return &sockjsSession{
		sess:    sess,
		closeCh: make(chan struct{}),
	}
}

func (sess *sockjsSession) Send(msg []byte) error {
	select {
	case <-sess.closeCh:
		return nil
	default:
		return sess.sess.Send(string(msg))
	}
}

func (sess *sockjsSession) Close(advice *conns.DisconnectAdvice) error {
	sess.mu.Lock()
	defer sess.mu.Unlock()
	if sess.closed {
		// Already closed, noop.
		return nil
	}
	sess.closed = true
	close(sess.closeCh)
	if advice == nil {
		advice = conns.DefaultDisconnectAdvice
	}
	reason, err := advice.JSONString()
	if err != nil {
		return err
	}
	return sess.sess.Close(CloseStatus, reason)
}

// websocketConn is an interface to mimic gorilla/websocket methods we use in Centrifugo.
// Needed only to simplify wsConn struct tests. Description can be found in gorilla websocket docs:
// https://godoc.org/github.com/gorilla/websocket.
type websocketConn interface {
	ReadMessage() (messageType int, p []byte, err error)
	WriteMessage(messageType int, data []byte) error
	WriteControl(messageType int, data []byte, deadline time.Time) error
	SetWriteDeadline(t time.Time) error
	EnableWriteCompression(enable bool)
	Close() error
}

// wsSession is a wrapper struct over websocket connection to fit session interface so
// client will accept it.
type wsSession struct {
	mu                 sync.RWMutex
	ws                 websocketConn
	closed             bool
	closeCh            chan struct{}
	pingInterval       time.Duration
	writeTimeout       time.Duration
	compressionMinSize int
	pingTimer          *time.Timer
}

func newWSSession(ws websocketConn, pingInterval time.Duration, writeTimeout time.Duration, compressionMinSize int) *wsSession {
	sess := &wsSession{
		ws:                 ws,
		closeCh:            make(chan struct{}),
		pingInterval:       pingInterval,
		writeTimeout:       writeTimeout,
		compressionMinSize: compressionMinSize,
	}
	if pingInterval > 0 {
		sess.addPing()
	}
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
			sess.Close(&conns.DisconnectAdvice{Reason: "write ping error", Reconnect: true})
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
		if sess.compressionMinSize > 0 {
			sess.ws.EnableWriteCompression(len(msg) > sess.compressionMinSize)
		}
		if sess.writeTimeout > 0 {
			sess.ws.SetWriteDeadline(time.Now().Add(sess.writeTimeout))
		}
		err := sess.ws.WriteMessage(websocket.TextMessage, msg)
		if sess.writeTimeout > 0 {
			sess.ws.SetWriteDeadline(time.Time{})
		}
		return err
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
	if sess.pingTimer != nil {
		sess.pingTimer.Stop()
	}
	sess.mu.Unlock()
	if advice != nil {
		deadline := time.Now().Add(time.Second)
		reason, err := advice.JSONString()
		if err != nil {
			return err
		}
		msg := websocket.FormatCloseMessage(int(CloseStatus), reason)
		sess.ws.WriteControl(websocket.CloseMessage, msg, deadline)
	}
	return sess.ws.Close()
}
