package sockjs

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

func (h *handler) rawWebsocket(rw http.ResponseWriter, req *http.Request) {
	var conn *websocket.Conn
	var err error
	if h.options.WebsocketUpgrader != nil {
		conn, err = h.options.WebsocketUpgrader.Upgrade(rw, req, nil)
	} else {
		// use default as before, so that those 2 buffer size variables are used as before
		conn, err = websocket.Upgrade(rw, req, nil, WebSocketReadBufSize, WebSocketWriteBufSize)
	}

	if _, ok := err.(websocket.HandshakeError); ok {
		http.Error(rw, `Can "Upgrade" only to "WebSocket".`, http.StatusBadRequest)
		return
	} else if err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		return
	}

	sessID := ""
	sess := newSession(req, sessID, h.options.DisconnectDelay, h.options.HeartbeatDelay)
	sess.raw = true

	receiver := newRawWsReceiver(conn, h.options.WebsocketWriteTimeout)
	sess.attachReceiver(receiver)
	if h.handlerFunc != nil {
		go h.handlerFunc(sess)
	}
	readCloseCh := make(chan struct{})
	go func() {
		for {
			frameType, p, err := conn.ReadMessage()
			if err != nil {
				close(readCloseCh)
				return
			}
			if frameType == websocket.TextMessage || frameType == websocket.BinaryMessage {
				sess.accept(string(p))
			}
		}
	}()

	select {
	case <-readCloseCh:
	case <-receiver.doneNotify():
	}
	sess.close()
	conn.Close()
}

type rawWsReceiver struct {
	conn         *websocket.Conn
	closeCh      chan struct{}
	writeTimeout time.Duration
}

func newRawWsReceiver(conn *websocket.Conn, writeTimeout time.Duration) *rawWsReceiver {
	return &rawWsReceiver{
		conn:         conn,
		closeCh:      make(chan struct{}),
		writeTimeout: writeTimeout,
	}
}

func (w *rawWsReceiver) sendBulk(messages ...string) {
	if len(messages) > 0 {
		for _, m := range messages {
			if w.writeTimeout != 0 {
				w.conn.SetWriteDeadline(time.Now().Add(w.writeTimeout))
			}
			err := w.conn.WriteMessage(websocket.TextMessage, []byte(m))
			if err != nil {
				w.close()
				break
			}

		}
	}
}

func (w *rawWsReceiver) sendFrame(frame string) {
	if w.writeTimeout != 0 {
		w.conn.SetWriteDeadline(time.Now().Add(w.writeTimeout))
	}
	var err error
	if frame == "h" {
		err = w.conn.WriteMessage(websocket.PingMessage, []byte{})
	} else if len(frame) > 0 && frame[0] == 'c' {
		status, reason := parseCloseFrame(frame)
		msg := websocket.FormatCloseMessage(int(status), reason)
		err = w.conn.WriteMessage(websocket.CloseMessage, msg)
	} else {
		err = w.conn.WriteMessage(websocket.TextMessage, []byte(frame))
	}
	if err != nil {
		w.close()
	}
}

func parseCloseFrame(frame string) (status uint32, reason string) {
	var items [2]interface{}
	json.Unmarshal([]byte(frame)[1:], &items)
	statusF, _ := items[0].(float64)
	status = uint32(statusF)
	reason, _ = items[1].(string)
	return
}

func (w *rawWsReceiver) close() {
	select {
	case <-w.closeCh: // already closed
	default:
		close(w.closeCh)
	}
}
func (w *rawWsReceiver) canSend() bool {
	select {
	case <-w.closeCh: // already closed
		return false
	default:
		return true
	}
}
func (w *rawWsReceiver) doneNotify() <-chan struct{}        { return w.closeCh }
func (w *rawWsReceiver) interruptedNotify() <-chan struct{} { return nil }
