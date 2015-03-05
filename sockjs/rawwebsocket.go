package sockjs

import (
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/nu7hatch/gouuid"
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

	uid, err := uuid.NewV4()
	if err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		return
	}
	sess := newSession(uid.String(), h.options.DisconnectDelay, h.options.HeartbeatDelay)
	sess.setRaw()
	if h.handlerFunc != nil {
		go h.handlerFunc(sess)
	}

	receiver := newRawWsReceiver(conn)
	sess.attachReceiver(receiver)
	readCloseCh := make(chan struct{})
	go func() {
		for {
			_, p, err := conn.ReadMessage()
			if err != nil {
				close(readCloseCh)
				return
			}
			sess.accept(string(p))
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
	*wsReceiver
}

func newRawWsReceiver(conn *websocket.Conn) *rawWsReceiver {
	return &rawWsReceiver{
		&wsReceiver{
			conn:    conn,
			closeCh: make(chan struct{}),
		},
	}
}

func (w *rawWsReceiver) sendBulk(messages ...string) {
	for _, message := range messages {
		w.sendFrame(message)
	}
}
