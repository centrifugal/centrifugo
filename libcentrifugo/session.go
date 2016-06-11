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

// wsConn is a struct to fit SockJS session interface so client will accept
// it as its sess
type wsConn struct {
	ws           *websocket.Conn
	closeCh      chan struct{}
	pingInterval time.Duration
	pingTimer    *time.Timer
}

func newWSConn(ws *websocket.Conn, pingInterval time.Duration) *wsConn {
	conn := &wsConn{
		ws:           ws,
		closeCh:      make(chan struct{}),
		pingInterval: pingInterval,
	}
	conn.pingTimer = time.AfterFunc(conn.pingInterval, conn.ping)
	return conn
}

func (conn *wsConn) ping() {
	select {
	case <-conn.closeCh:
		conn.pingTimer.Stop()
		return
	default:
		err := conn.ws.WriteControl(websocket.PingMessage, []byte("ping"), time.Now().Add(conn.pingInterval/2))
		if err != nil {
			conn.ws.Close()
			conn.pingTimer.Stop()
			return
		}
		conn.pingTimer = time.AfterFunc(conn.pingInterval, conn.ping)
	}
}

func (conn *wsConn) Send(message []byte) error {
	select {
	case <-conn.closeCh:
		return nil
	default:
		return conn.ws.WriteMessage(websocket.TextMessage, message)
	}
}

func (conn *wsConn) Close(status uint32, reason string) error {
	conn.ws.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(int(status), reason), time.Now().Add(time.Second))
	return conn.ws.Close()
}
