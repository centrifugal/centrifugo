package centrifuge

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/centrifugal/centrifuge/internal/proto"

	"github.com/gorilla/websocket"
)

const (
	transportWebsocket = "websocket"
)

const (
	// We don't use specific websocket close codes because our client
	// have no notion about transport specifics.
	websocketCloseStatus = 3000
)

// websocketTransport is a wrapper struct over websocket connection to fit session
// interface so client will accept it.
type websocketTransport struct {
	mu        sync.RWMutex
	conn      *websocket.Conn
	closed    bool
	closeCh   chan struct{}
	opts      *websocketTransportOptions
	pingTimer *time.Timer
	writer    *writer
}

type websocketTransportOptions struct {
	enc                proto.Encoding
	pingInterval       time.Duration
	writeTimeout       time.Duration
	compressionMinSize int
}

func newWebsocketTransport(conn *websocket.Conn, writer *writer, opts *websocketTransportOptions) *websocketTransport {
	transport := &websocketTransport{
		conn:    conn,
		closeCh: make(chan struct{}),
		opts:    opts,
		writer:  writer,
	}
	writer.onWrite(transport.write)
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

func (t *websocketTransport) Encoding() proto.Encoding {
	return t.opts.enc
}

func (t *websocketTransport) Send(reply *proto.PreparedReply) error {
	data := reply.Data()
	disconnect := t.writer.write(data)
	if disconnect != nil {
		// Close in goroutine to not block message broadcast.
		go t.Close(disconnect)
	}
	return nil
}

func (t *websocketTransport) write(data ...[]byte) error {
	select {
	case <-t.closeCh:
		return nil
	default:
		if t.opts.compressionMinSize > 0 {
			t.conn.EnableWriteCompression(len(data) > t.opts.compressionMinSize)
		}
		if t.opts.writeTimeout > 0 {
			t.conn.SetWriteDeadline(time.Now().Add(t.opts.writeTimeout))
		}

		var err error
		var messageType = websocket.TextMessage
		if t.Encoding() == proto.EncodingProtobuf {
			messageType = websocket.BinaryMessage
		}
		writer, err := t.conn.NextWriter(messageType)
		if err != nil {
			t.Close(&Disconnect{Reason: "error sending message", Reconnect: true})
			return err
		}
		bytesOut := 0
		for _, payload := range data {
			n, err := writer.Write(payload)
			if n != len(payload) || err != nil {
				t.Close(&Disconnect{Reason: "error sending message", Reconnect: true})
				return err
			}
			bytesOut += len(data)
		}
		err = writer.Close()
		if err != nil {
			t.Close(&Disconnect{Reason: "error sending message", Reconnect: true})
			return err
		}
		if t.opts.writeTimeout > 0 {
			t.conn.SetWriteDeadline(time.Time{})
		}
		transportMessagesSent.WithLabelValues(transportWebsocket).Add(float64(len(data)))
		transportBytesOut.WithLabelValues(transportWebsocket).Add(float64(bytesOut))
		return err
	}
}

func (t *websocketTransport) Close(disconnect *Disconnect) error {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return nil
	}
	close(t.closeCh)
	t.closed = true
	if t.pingTimer != nil {
		t.pingTimer.Stop()
	}
	t.mu.Unlock()
	if disconnect != nil {
		deadline := time.Now().Add(time.Second)
		reason, err := json.Marshal(disconnect)
		if err != nil {
			return err
		}
		msg := websocket.FormatCloseMessage(websocketCloseStatus, string(reason))
		t.conn.WriteControl(websocket.CloseMessage, msg, deadline)
		return t.conn.Close()
	}
	return t.conn.Close()
}

// WebsocketConfig represents config for WebsocketHandler.
type WebsocketConfig struct {
	// Compression allows to enable websocket permessage-deflate
	// compression support for raw websocket connections. It does not guarantee
	// that compression will be used - i.e. it only says that Centrifugo will
	// try to negotiate it with client.
	Compression bool

	// CompressionLevel sets a level for websocket compression.
	// See posiible value description at https://golang.org/pkg/compress/flate/#NewWriter
	CompressionLevel int

	// CompressionMinSize allows to set minimal limit in bytes for message to use
	// compression when writing it into client connection. By default it's 0 - i.e. all messages
	// will be compressed when WebsocketCompression enabled and compression negotiated with client.
	CompressionMinSize int

	// ReadBufferSize is a parameter that is used for raw websocket Upgrader.
	// If set to zero reasonable default value will be used.
	ReadBufferSize int

	// WriteBufferSize is a parameter that is used for raw websocket Upgrader.
	// If set to zero reasonable default value will be used.
	WriteBufferSize int
}

// WebsocketHandler ...
type WebsocketHandler struct {
	node   *Node
	config WebsocketConfig
}

// NewWebsocketHandler ...
func NewWebsocketHandler(n *Node, c WebsocketConfig) *WebsocketHandler {
	return &WebsocketHandler{
		node:   n,
		config: c,
	}
}

func (s *WebsocketHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	transportConnectCount.WithLabelValues(transportWebsocket).Inc()

	compression := s.config.Compression
	compressionLevel := s.config.CompressionLevel
	compressionMinSize := s.config.CompressionMinSize

	upgrader := websocket.Upgrader{
		ReadBufferSize:    s.config.ReadBufferSize,
		WriteBufferSize:   s.config.WriteBufferSize,
		EnableCompression: s.config.Compression,
		CheckOrigin: func(r *http.Request) bool {
			// Allow all connections.
			return true
		},
	}

	conn, err := upgrader.Upgrade(rw, r, nil)
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

	config := s.node.Config()
	pingInterval := config.ClientPingInterval
	writeTimeout := config.ClientMessageWriteTimeout
	maxRequestSize := config.ClientRequestMaxSize

	if maxRequestSize > 0 {
		conn.SetReadLimit(int64(maxRequestSize))
	}
	if pingInterval > 0 {
		pongWait := pingInterval * 10 / 9
		conn.SetReadDeadline(time.Now().Add(pongWait))
		conn.SetPongHandler(func(string) error { conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })
	}

	var enc = proto.EncodingJSON
	if r.URL.Query().Get("format") == "protobuf" {
		enc = proto.EncodingProtobuf
	}

	// Separate goroutine for better GC of caller's data.
	go func() {
		opts := &websocketTransportOptions{
			pingInterval:       pingInterval,
			writeTimeout:       writeTimeout,
			compressionMinSize: compressionMinSize,
			enc:                enc,
		}
		writerConf := writerConfig{
			MaxQueueSize: config.ClientQueueMaxSize,
		}
		writer := newWriter(writerConf)
		defer writer.close()
		transport := newWebsocketTransport(conn, writer, opts)
		c := newClient(r.Context(), s.node, transport)
		defer c.Close(nil)

		s.node.logger.log(newLogEntry(LogLevelDebug, "websocket connection established", map[string]interface{}{"client": c.ID()}))
		defer func(started time.Time) {
			s.node.logger.log(newLogEntry(LogLevelDebug, "websocket connection completed", map[string]interface{}{"client": c.ID(), "time": time.Since(started)}))
		}(time.Now())

		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			ok := handleClientData(s.node, c, data, transport, writer)
			if !ok {
				return
			}
		}
	}()
}

// common data handling logic for Websocket and Sockjs handlers.
func handleClientData(n *Node, c *client, data []byte, transport Transport, writer *writer) bool {
	transportBytesIn.WithLabelValues(transport.Name()).Add(float64(len(data)))

	if len(data) == 0 {
		n.logger.log(newLogEntry(LogLevelError, "empty client request received"))
		c.Close(DisconnectBadRequest)
		return false
	}

	enc := transport.Encoding()

	encoder := proto.GetReplyEncoder(enc)
	decoder := proto.GetCommandDecoder(enc, data)
	var numReplies int

	for {
		cmd, err := decoder.Decode()
		if err != nil {
			if err == io.EOF {
				break
			}
			n.logger.log(newLogEntry(LogLevelInfo, "error decoding request", map[string]interface{}{"client": c.ID(), "user": c.UserID(), "error": err.Error()}))
			c.Close(DisconnectBadRequest)
			proto.PutCommandDecoder(enc, decoder)
			proto.PutReplyEncoder(enc, encoder)
			return false
		}
		rep, disconnect := c.handle(cmd)
		if disconnect != nil {
			n.logger.log(newLogEntry(LogLevelInfo, "disconnect after handling command", map[string]interface{}{"command": fmt.Sprintf("%v", cmd), "client": c.ID(), "user": c.UserID(), "reason": disconnect.Reason}))
			c.Close(disconnect)
			proto.PutCommandDecoder(enc, decoder)
			proto.PutReplyEncoder(enc, encoder)
			return false
		}
		if rep != nil {
			err = encoder.Encode(rep)
			numReplies++
			if err != nil {
				n.logger.log(newLogEntry(LogLevelError, "error encoding reply", map[string]interface{}{"reply": fmt.Sprintf("%v", rep), "client": c.ID(), "user": c.UserID(), "error": err.Error()}))
				c.Close(DisconnectServerError)
				return false
			}
		}
	}

	if numReplies > 0 {
		disconnect := writer.write(encoder.Finish())
		if disconnect != nil {
			n.logger.log(newLogEntry(LogLevelInfo, "disconnect after sending data to transport", map[string]interface{}{"client": c.ID(), "user": c.UserID(), "reason": disconnect.Reason}))
			c.Close(disconnect)
			proto.PutCommandDecoder(enc, decoder)
			proto.PutReplyEncoder(enc, encoder)
			return false
		}
	}

	proto.PutCommandDecoder(enc, decoder)
	proto.PutReplyEncoder(enc, encoder)

	return true
}
