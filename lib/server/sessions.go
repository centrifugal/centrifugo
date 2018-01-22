package server

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/centrifugal/centrifugo/lib/logger"
	"github.com/centrifugal/centrifugo/lib/proto"

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
	session sockjs.Session
}

func newSockjsSession(s sockjs.Session) *sockjsSession {
	return &sockjsSession{
		session: s,
		closeCh: make(chan struct{}),
	}
}

func (s *sockjsSession) Name() string {
	return "sockjs"
}

func (s *sockjsSession) Send(msg []byte) error {
	select {
	case <-s.closeCh:
		return nil
	default:
		return s.session.Send(string(msg))
	}
}

func (s *sockjsSession) Close(advice *proto.Disconnect) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		// Already closed, noop.
		return nil
	}
	s.closed = true
	close(s.closeCh)
	if advice == nil {
		advice = proto.DisconnectNormal
	}
	reason, err := json.Marshal(advice)
	if err != nil {
		return err
	}
	return s.session.Close(CloseStatus, string(reason))
}

// wsConn is an interface to mimic gorilla/websocket methods we use
// in Centrifugo. Needed only to simplify wsConn struct tests.
// Description can be found in gorilla websocket docs:
// https://godoc.org/github.com/gorilla/websocket.
type wsConn interface {
	ReadMessage() (messageType int, p []byte, err error)
	WriteMessage(messageType int, data []byte) error
	WriteControl(messageType int, data []byte, deadline time.Time) error
	SetWriteDeadline(t time.Time) error
	EnableWriteCompression(enable bool)
	Close() error
}

// wsSession is a wrapper struct over websocket connection to fit session
// interface so client will accept it.
type wsSession struct {
	mu        sync.RWMutex
	conn      wsConn
	closed    bool
	closeCh   chan struct{}
	opts      *wsSessionOptions
	pingTimer *time.Timer
}

type wsSessionOptions struct {
	pingInterval       time.Duration
	writeTimeout       time.Duration
	compressionMinSize int
}

func newWSSession(conn wsConn, opts *wsSessionOptions) *wsSession {
	sess := &wsSession{
		conn:    conn,
		closeCh: make(chan struct{}),
		opts:    opts,
	}
	if opts.pingInterval > 0 {
		sess.addPing()
	}
	return sess
}

func (s *wsSession) ping() {
	select {
	case <-s.closeCh:
		return
	default:
		deadline := time.Now().Add(s.opts.pingInterval / 2)
		err := s.conn.WriteControl(websocket.PingMessage, []byte("ping"), deadline)
		if err != nil {
			logger.ERROR.Printf("Error write ping: %v", err)
			s.Close(proto.DisconnectServerError)
			return
		}
		s.addPing()
	}
}

func (s *wsSession) addPing() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.pingTimer = time.AfterFunc(s.opts.pingInterval, s.ping)
	s.mu.Unlock()
}

func (s *wsSession) Name() string {
	return "websocket"
}

func (s *wsSession) Send(msg []byte) error {
	select {
	case <-s.closeCh:
		return nil
	default:
		if s.opts.compressionMinSize > 0 {
			s.conn.EnableWriteCompression(len(msg) > s.opts.compressionMinSize)
		}
		if s.opts.writeTimeout > 0 {
			s.conn.SetWriteDeadline(time.Now().Add(s.opts.writeTimeout))
		}
		var err error
		err = s.conn.WriteMessage(websocket.TextMessage, msg)

		if s.opts.writeTimeout > 0 {
			s.conn.SetWriteDeadline(time.Time{})
		}
		return err
	}
}

func (s *wsSession) Close(advice *proto.Disconnect) error {
	s.mu.Lock()
	if s.closed {
		// Already closed, noop.
		s.mu.Unlock()
		return nil
	}
	close(s.closeCh)
	s.closed = true
	if s.pingTimer != nil {
		s.pingTimer.Stop()
	}
	s.mu.Unlock()
	if advice != nil {
		deadline := time.Now().Add(time.Second)
		reason, err := json.Marshal(advice)
		if err != nil {
			return err
		}
		msg := websocket.FormatCloseMessage(int(CloseStatus), string(reason))
		s.conn.WriteControl(websocket.CloseMessage, msg, deadline)
		return s.conn.Close()
	}
	return s.conn.Close()
}
