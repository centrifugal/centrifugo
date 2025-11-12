package uniws

import (
	"sync"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/timers"
	"github.com/centrifugal/centrifugo/v6/internal/tools"
	"github.com/centrifugal/centrifugo/v6/internal/websocket"

	"github.com/centrifugal/centrifuge"
	"github.com/centrifugal/protocol"
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
	framePingInterval       time.Duration
	framePongTimeout        time.Duration
	writeTimeout            time.Duration
	compressionMinSize      int
	pingPongConfig          centrifuge.PingPongConfig
	protoMajor              int
	joinMessages            bool
	disableClosingHandshake bool // TODO: consider keeping/removing for Centrifugo v7.
	disableDisconnectPush   bool
}

func newWebsocketTransport(conn *websocket.Conn, opts websocketTransportOptions, graceCh chan struct{}) *websocketTransport {
	transport := &websocketTransport{
		conn:    conn,
		closeCh: make(chan struct{}),
		graceCh: graceCh,
		opts:    opts,
	}
	if opts.framePingInterval > 0 {
		transport.addPing()
	}
	return transport
}

func (t *websocketTransport) ping() {
	select {
	case <-t.closeCh:
		return
	default:
		if t.opts.framePongTimeout > 0 {
			// It's safe to call SetReadDeadline concurrently with reader in separate goroutine.
			// According to Go docs:
			// SetReadDeadline sets the deadline for future Read calls and any currently-blocked Read call.
			_ = t.conn.SetReadDeadline(time.Now().Add(t.opts.framePongTimeout))
		}
		err := t.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(t.opts.writeTimeout))
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
	t.pingTimer = time.AfterFunc(t.opts.framePingInterval, t.ping)
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
	if t.opts.disableDisconnectPush {
		return centrifuge.PushFlagDisconnect
	}
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

// AcceptProtocol ...
func (t *websocketTransport) AcceptProtocol() string {
	return tools.GetAcceptProtocolLabel(t.opts.protoMajor)
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
		if t.opts.protoMajor > 1 {
			// For HTTP/2 connections, we need to actually clear the deadline on the underlying
			// connection. The websocket Conn.SetWriteDeadline only sets a field, but doesn't
			// clear the deadline that was already set on the underlying net.Conn during write.
			// This is critical for HTTP/2 ResponseController where expired deadlines cannot be
			// extended and will cause the stream to fail permanently.
			err = t.conn.NetConn().SetWriteDeadline(time.Time{})
			if err != nil {
				t.writeMu.Unlock()
				return err
			}
		}
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
func (t *websocketTransport) Close(disconnect centrifuge.Disconnect) error {
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

	if t.opts.disableClosingHandshake {
		return t.conn.Close()
	}

	if disconnect.Code != centrifuge.DisconnectConnectionClosed.Code {
		msg := websocket.FormatCloseMessage(int(disconnect.Code), disconnect.Reason)
		err := t.conn.WriteControl(websocket.CloseMessage, msg, time.Now().Add(t.opts.writeTimeout))
		if err != nil {
			return t.conn.Close()
		}
		select {
		case <-t.graceCh:
		default:
			// Wait for closing handshake completion.
			tm := timers.AcquireTimer(closeFrameWait)
			select {
			case <-t.graceCh:
			case <-tm.C:
			}
			timers.ReleaseTimer(tm)
		}
		return t.conn.Close()
	}
	return t.conn.Close()
}
