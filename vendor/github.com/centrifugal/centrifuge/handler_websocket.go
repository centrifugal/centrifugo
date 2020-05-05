package centrifuge

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/centrifugal/centrifuge/internal/timers"
)

const (
	transportWebsocket = "websocket"
)

// websocketTransport is a wrapper struct over websocket connection to fit session
// interface so client will accept it.
type websocketTransport struct {
	mu        sync.RWMutex
	conn      *websocket.Conn
	closed    bool
	closeCh   chan struct{}
	graceCh   chan struct{}
	opts      websocketTransportOptions
	pingTimer *time.Timer
}

type websocketTransportOptions struct {
	encType            EncodingType
	protoType          ProtocolType
	pingInterval       time.Duration
	writeTimeout       time.Duration
	compressionMinSize int
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
		err := t.conn.WriteControl(websocket.PingMessage, []byte("ping"), deadline)
		if err != nil {
			t.Close(DisconnectServerError)
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

func (t *websocketTransport) Name() string {
	return transportWebsocket
}

func (t *websocketTransport) Protocol() ProtocolType {
	return t.opts.protoType
}

func (t *websocketTransport) Encoding() EncodingType {
	return t.opts.encType
}

func (t *websocketTransport) Write(data []byte) error {
	select {
	case <-t.closeCh:
		return nil
	default:
		if t.opts.compressionMinSize > 0 {
			t.conn.EnableWriteCompression(len(data) > t.opts.compressionMinSize)
		}
		if t.opts.writeTimeout > 0 {
			_ = t.conn.SetWriteDeadline(time.Now().Add(t.opts.writeTimeout))
		}

		var messageType = websocket.TextMessage
		if t.Protocol() == ProtocolTypeProtobuf {
			messageType = websocket.BinaryMessage
		}

		err := t.conn.WriteMessage(messageType, data)
		if err != nil {
			return err
		}

		if t.opts.writeTimeout > 0 {
			_ = t.conn.SetWriteDeadline(time.Time{})
		}
		return nil
	}
}

func (t *websocketTransport) Close(disconnect *Disconnect) error {
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

	if disconnect != nil {
		reason, err := json.Marshal(disconnect)
		if err != nil {
			return err
		}
		msg := websocket.FormatCloseMessage(disconnect.Code, string(reason))
		err = t.conn.WriteControl(websocket.CloseMessage, msg, time.Now().Add(time.Second))
		if err != nil {
			return t.conn.Close()
		}

		// Wait for closing handshake completion.
		tm := timers.AcquireTimer(5 * time.Second)
		select {
		case <-t.graceCh:
		case <-tm.C:
		}
		timers.ReleaseTimer(tm)
		return t.conn.Close()
	}
	return t.conn.Close()
}

// Defaults.
const (
	DefaultWebsocketPingInterval     = 25 * time.Second
	DefaultWebsocketWriteTimeout     = 1 * time.Second
	DefaultWebsocketMessageSizeLimit = 65536 // 64KB
)

// WebsocketConfig represents config for WebsocketHandler.
type WebsocketConfig struct {
	// Compression allows to enable websocket permessage-deflate
	// compression support for raw websocket connections. It does
	// not guarantee that compression will be used - i.e. it only
	// says that server will try to negotiate it with client.
	Compression bool

	// CompressionLevel sets a level for websocket compression.
	// See posiible value description at https://golang.org/pkg/compress/flate/#NewWriter
	CompressionLevel int

	// CompressionMinSize allows to set minimal limit in bytes for
	// message to use compression when writing it into client connection.
	// By default it's 0 - i.e. all messages will be compressed when
	// WebsocketCompression enabled and compression negotiated with client.
	CompressionMinSize int

	// ReadBufferSize is a parameter that is used for raw websocket Upgrader.
	// If set to zero reasonable default value will be used.
	ReadBufferSize int

	// WriteBufferSize is a parameter that is used for raw websocket Upgrader.
	// If set to zero reasonable default value will be used.
	WriteBufferSize int

	// UseWriteBufferPool enables using buffer pool for writes.
	UseWriteBufferPool bool

	// CheckOrigin func to provide custom origin check logic.
	// nil means allow all origins.
	CheckOrigin func(r *http.Request) bool

	// PingInterval sets interval server will send ping messages to clients.
	// By default DefaultPingInterval will be used.
	PingInterval time.Duration

	// WriteTimeout is maximum time of write message operation.
	// Slow client will be disconnected.
	// By default DefaultWebsocketWriteTimeout will be used.
	WriteTimeout time.Duration

	// MessageSizeLimit sets the maximum size in bytes of allowed message from client.
	// By default DefaultWebsocketMaxMessageSize will be used.
	MessageSizeLimit int
}

// WebsocketHandler handles websocket client connections.
type WebsocketHandler struct {
	node     *Node
	upgrader *websocket.Upgrader
	config   WebsocketConfig
}

var writeBufferPool = &sync.Pool{}

// NewWebsocketHandler creates new WebsocketHandler.
func NewWebsocketHandler(n *Node, c WebsocketConfig) *WebsocketHandler {
	upgrader := &websocket.Upgrader{
		ReadBufferSize:    c.ReadBufferSize,
		EnableCompression: c.Compression,
	}
	if c.UseWriteBufferPool {
		upgrader.WriteBufferPool = writeBufferPool
	} else {
		upgrader.WriteBufferSize = c.WriteBufferSize
	}
	if c.CheckOrigin != nil {
		upgrader.CheckOrigin = c.CheckOrigin
	} else {
		upgrader.CheckOrigin = func(r *http.Request) bool {
			// Allow all connections.
			return true
		}
	}
	return &WebsocketHandler{
		node:     n,
		config:   c,
		upgrader: upgrader,
	}
}

func (s *WebsocketHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	transportConnectCount.WithLabelValues(transportWebsocket).Inc()

	compression := s.config.Compression
	compressionLevel := s.config.CompressionLevel
	compressionMinSize := s.config.CompressionMinSize

	conn, err := s.upgrader.Upgrade(rw, r, nil)
	if err != nil {
		s.node.logger.log(newLogEntry(LogLevelDebug, "websocket upgrade error", map[string]interface{}{"error": err.Error()}))
		return
	}

	if compression {
		err := conn.SetCompressionLevel(compressionLevel)
		if err != nil {
			s.node.logger.log(newLogEntry(LogLevelError, "websocket error setting compression level", map[string]interface{}{"error": err.Error()}))
		}
	}

	pingInterval := s.config.PingInterval
	if pingInterval == 0 {
		pingInterval = DefaultWebsocketPingInterval
	}
	writeTimeout := s.config.WriteTimeout
	if writeTimeout == 0 {
		writeTimeout = DefaultWebsocketWriteTimeout
	}
	messageSizeLimit := s.config.MessageSizeLimit
	if messageSizeLimit == 0 {
		messageSizeLimit = DefaultWebsocketMessageSizeLimit
	}

	if messageSizeLimit > 0 {
		conn.SetReadLimit(int64(messageSizeLimit))
	}
	if pingInterval > 0 {
		pongWait := pingInterval * 10 / 9
		_ = conn.SetReadDeadline(time.Now().Add(pongWait))
		conn.SetPongHandler(func(string) error {
			_ = conn.SetReadDeadline(time.Now().Add(pongWait))
			return nil
		})
	}

	var protocol = ProtocolTypeJSON
	if r.URL.Query().Get("format") == "protobuf" || r.URL.Query().Get("protocol") == "protobuf" {
		protocol = ProtocolTypeProtobuf
	}

	var enc = EncodingTypeJSON
	if r.URL.Query().Get("encoding") == "binary" {
		enc = EncodingTypeBinary
	}

	// Separate goroutine for better GC of caller's data.
	go func() {
		opts := websocketTransportOptions{
			pingInterval:       pingInterval,
			writeTimeout:       writeTimeout,
			compressionMinSize: compressionMinSize,
			encType:            enc,
			protoType:          protocol,
		}

		graceCh := make(chan struct{})
		transport := newWebsocketTransport(conn, opts, graceCh)

		select {
		case <-s.node.NotifyShutdown():
			transport.Close(DisconnectShutdown)
			return
		default:
		}

		ctxCh := make(chan struct{})
		defer close(ctxCh)

		c, err := NewClient(newCustomCancelContext(r.Context(), ctxCh), s.node, transport)
		if err != nil {
			s.node.logger.log(newLogEntry(LogLevelError, "error creating client", map[string]interface{}{"transport": transportWebsocket}))
			return
		}
		s.node.logger.log(newLogEntry(LogLevelDebug, "client connection established", map[string]interface{}{"client": c.ID(), "transport": transportWebsocket}))
		defer func(started time.Time) {
			s.node.logger.log(newLogEntry(LogLevelDebug, "client connection completed", map[string]interface{}{"client": c.ID(), "transport": transportWebsocket, "duration": time.Since(started)}))
		}(time.Now())
		defer c.Close(nil)

		var handleMu sync.RWMutex
		var closed bool
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				close(graceCh)
				return
			}
			handleMu.RLock()
			if closed {
				handleMu.RUnlock()
				continue
			}
			handleMu.RUnlock()
			go func() {
				// We use goroutine here for proper handling of Websocket closing handshake.
				// See https://github.com/gorilla/websocket/issues/448.
				handleMu.Lock()
				closed = !c.Handle(data)
				handleMu.Unlock()
			}()
		}
	}()
}
