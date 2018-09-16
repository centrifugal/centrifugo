package sockjs

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/websocket"
)

func (h *handler) rawWebsocket(rw http.ResponseWriter, req *http.Request) {
	conn, err := websocket.Upgrade(rw, req, nil, WebSocketReadBufSize, WebSocketWriteBufSize)
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
	if h.handlerFunc != nil {
		go h.handlerFunc(sess)
	}

	receiver := newRawWsReceiver(conn)
	sess.attachReceiver(receiver)
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
	conn    *websocket.Conn
	closeCh chan struct{}
}

func newRawWsReceiver(conn *websocket.Conn) *rawWsReceiver {
	return &rawWsReceiver{
		conn:    conn,
		closeCh: make(chan struct{}),
	}
}

func (w *rawWsReceiver) sendBulk(messages ...string) {
	if len(messages) > 0 {
		for _, m := range messages {
			err := w.conn.WriteMessage(websocket.TextMessage, []byte(m))
			if err != nil {
				w.close()
				break
			}

		}
	}
}

func (w *rawWsReceiver) sendFrame(frame string) {
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
