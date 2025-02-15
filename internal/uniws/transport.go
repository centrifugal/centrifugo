package uniws

import (
	"sync"
	"time"

	"github.com/centrifugal/centrifuge"
	"github.com/centrifugal/protocol"
	"github.com/gorilla/websocket"
)

// websocketTransport is a wrapper struct over websocket connection to fit session
// interface so client will accept it.
type websocketTransport struct {
	mu        sync.RWMutex
	writeMu   sync.Mutex // sync general write with unidirectional ping write.
	conn      *websocket.Conn
	closeCh   chan struct{}
	graceCh   chan struct{}
	opts      websocketTransportOptions
	pingTimer *time.Timer
	closed    bool
}

type websocketTransportOptions struct {
	pingInterval       time.Duration
	writeTimeout       time.Duration
	compressionMinSize int
	pingPongConfig     centrifuge.PingPongConfig
	joinMessages       bool
}

func newWebsocketTransport(conn *websocket.Conn, opts websocketTransportOptions, graceCh chan struct{}) *websocketTransport {
	transport := &websocketTransport{
		conn:    conn,
		closeCh: make(chan struct{}),
		graceCh: graceCh,
		opts:    opts,
	}
	if opts.pingInterval > 0 {
		transport.addPing()
	}
	return transport
}

func (t *websocketTransport) ping() {
	select {
	case <-t.closeCh:
		return
	default:
		deadline := time.Now().Add(t.opts.pingInterval / 2)
		err := t.conn.WriteControl(websocket.PingMessage, nil, deadline)
		if err != nil {
			_ = t.Close(centrifuge.DisconnectWriteError)
			return
		}
		t.addPing()
	}
}

func (t *websocketTransport) addPing() {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return
	}
	t.pingTimer = time.AfterFunc(t.opts.pingInterval, t.ping)
	t.mu.Unlock()
}

const transportName = "uni_websocket"

// Name returns name of transport.
func (t *websocketTransport) Name() string {
	return transportName
}

// Protocol returns transport protocol.
func (t *websocketTransport) Protocol() centrifuge.ProtocolType {
	return centrifuge.ProtocolTypeJSON
}

// ProtocolVersion returns transport protocol version.
func (t *websocketTransport) ProtocolVersion() centrifuge.ProtocolVersion {
	return centrifuge.ProtocolVersion2
}

// Unidirectional returns whether transport is unidirectional.
func (t *websocketTransport) Unidirectional() bool {
	return true
}

// DisabledPushFlags ...
func (t *websocketTransport) DisabledPushFlags() uint64 {
	return 0
}

// PingPongConfig ...
func (t *websocketTransport) PingPongConfig() centrifuge.PingPongConfig {
	return t.opts.pingPongConfig
}

// Emulation ...
func (t *websocketTransport) Emulation() bool {
	return false
}

func (t *websocketTransport) writeData(data []byte) error {
	if t.opts.compressionMinSize > 0 {
		t.conn.EnableWriteCompression(len(data) > t.opts.compressionMinSize)
	}
	var messageType = websocket.TextMessage
	if t.Protocol() == centrifuge.ProtocolTypeProtobuf {
		messageType = websocket.BinaryMessage
	}

	t.writeMu.Lock()
	if t.opts.writeTimeout > 0 {
		_ = t.conn.SetWriteDeadline(time.Now().Add(t.opts.writeTimeout))
	}
	err := t.conn.WriteMessage(messageType, data)
	if err != nil {
		t.writeMu.Unlock()
		return err
	}
	if t.opts.writeTimeout > 0 {
		_ = t.conn.SetWriteDeadline(time.Time{})
	}
	t.writeMu.Unlock()

	return nil
}

func (t *websocketTransport) writeMany(messages ...[]byte) error {
	protoType := protocol.TypeJSON
	if t.Protocol() == centrifuge.ProtocolTypeProtobuf {
		protoType = protocol.TypeProtobuf
	}
	encoder := protocol.GetDataEncoder(protoType)
	defer protocol.PutDataEncoder(protoType, encoder)
	for i := range messages {
		_ = encoder.Encode(messages[i])
	}
	return t.writeData(encoder.Finish())
}

// Write data to transport.
func (t *websocketTransport) Write(message []byte) error {
	return t.WriteMany(message)
}

// WriteMany data to transport.
func (t *websocketTransport) WriteMany(messages ...[]byte) error {
	select {
	case <-t.closeCh:
		return nil
	default:
		var err error
		if t.opts.joinMessages {
			if len(messages) == 1 && t.Protocol() == centrifuge.ProtocolTypeJSON {
				// Fast path for one JSON message.
				return t.writeData(messages[0])
			}
			err = t.writeMany(messages...)
			if err != nil {
				return err
			}
		} else {
			for i := 0; i < len(messages); i++ {
				err = t.writeData(messages[i])
				if err != nil {
					return err
				}
			}
		}
		return nil
	}
}

const closeFrameWait = 5 * time.Second

// Close closes transport.
func (t *websocketTransport) Close(_ centrifuge.Disconnect) error {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return nil
	}
	t.closed = true
	if t.pingTimer != nil {
		t.pingTimer.Stop()
	}
	close(t.closeCh)
	t.mu.Unlock()
	return t.conn.Close()
}
