package centrifuge

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/centrifugal/centrifuge/internal/proto"
	"github.com/centrifugal/centrifuge/internal/proto/apiproto"
	"github.com/centrifugal/centrifuge/internal/queue"

	"github.com/gorilla/websocket"
	"github.com/igm/sockjs-go/sockjs"
)

const (
	transportWebsocket = "websocket"
	transportSockJS    = "sockjs"
)

// APIConfig ...
type APIConfig struct{}

// APIHandler is responsible for processing API commands over HTTP.
type APIHandler struct {
	node   *Node
	config APIConfig
	api    *apiExecutor
}

// NewAPIHandler creates new APIHandler.
func NewAPIHandler(n *Node, c APIConfig) *APIHandler {
	return &APIHandler{
		node:   n,
		config: c,
		api:    newAPIExecutor(n),
	}
}

func (s *APIHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	defer func(started time.Time) {
		apiHandlerDurationSummary.Observe(float64(time.Since(started).Seconds()))
	}(time.Now())

	var data []byte
	var err error

	data, err = ioutil.ReadAll(r.Body)
	if err != nil {
		s.node.logger.log(newLogEntry(LogLevelError, "error reading API request body", map[string]interface{}{"error": err.Error()}))
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if len(data) == 0 {
		s.node.logger.log(newLogEntry(LogLevelError, "no data in API request"))
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	var enc apiproto.Encoding

	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(strings.ToLower(contentType), "application/octet-stream") {
		enc = apiproto.EncodingProtobuf
	} else {
		enc = apiproto.EncodingJSON
	}

	encoder := apiproto.GetReplyEncoder(enc)
	defer apiproto.PutReplyEncoder(enc, encoder)

	decoder := apiproto.GetCommandDecoder(enc, data)
	defer apiproto.PutCommandDecoder(enc, decoder)

	for {
		command, err := decoder.Decode()
		if err != nil {
			if err == io.EOF {
				break
			}
			s.node.logger.log(newLogEntry(LogLevelError, "error decoding API data", map[string]interface{}{"error": err.Error()}))
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}
		now := time.Now()
		rep, err := s.handleAPICommand(r.Context(), enc, command)
		apiCommandDurationSummary.WithLabelValues(strings.ToLower(apiproto.MethodType_name[int32(command.Method)])).Observe(float64(time.Since(now).Seconds()))
		if err != nil {
			s.node.logger.log(newLogEntry(LogLevelError, "error handling API command", map[string]interface{}{"error": err.Error()}))
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		err = encoder.Encode(rep)
		if err != nil {
			s.node.logger.log(newLogEntry(LogLevelError, "error encoding API reply", map[string]interface{}{"error": err.Error()}))
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
	}
	resp := encoder.Finish()
	w.Header().Set("Content-Type", contentType)
	w.Write(resp)
}

func (s *APIHandler) handleAPICommand(ctx context.Context, enc apiproto.Encoding, cmd *apiproto.Command) (*apiproto.Reply, error) {
	var err error

	method := cmd.Method
	params := cmd.Params

	rep := &apiproto.Reply{
		ID: cmd.ID,
	}

	var replyRes proto.Raw

	decoder := apiproto.GetDecoder(enc)
	defer apiproto.PutDecoder(enc, decoder)

	encoder := apiproto.GetEncoder(enc)
	defer apiproto.PutEncoder(enc, encoder)

	switch method {
	case apiproto.MethodTypePublish:
		cmd, err := decoder.DecodePublish(params)
		if err != nil {
			s.node.logger.log(newLogEntry(LogLevelError, "error decoding publish params", map[string]interface{}{"error": err.Error()}))
			rep.Error = apiproto.ErrBadRequest
			return rep, nil
		}
		resp := s.api.Publish(ctx, cmd)
		if resp.Error != nil {
			rep.Error = resp.Error
		} else {
			if resp.Result != nil {
				replyRes, err = encoder.EncodePublish(resp.Result)
				if err != nil {
					return nil, err
				}
			}
		}
	case apiproto.MethodTypeBroadcast:
		cmd, err := decoder.DecodeBroadcast(params)
		if err != nil {
			s.node.logger.log(newLogEntry(LogLevelError, "error decoding broadcast params", map[string]interface{}{"error": err.Error()}))
			rep.Error = apiproto.ErrBadRequest
			return rep, nil
		}
		resp := s.api.Broadcast(ctx, cmd)
		if resp.Error != nil {
			rep.Error = resp.Error
		} else {
			if resp.Result != nil {
				replyRes, err = encoder.EncodeBroadcast(resp.Result)
				if err != nil {
					return nil, err
				}
			}
		}
	case apiproto.MethodTypeUnsubscribe:
		cmd, err := decoder.DecodeUnsubscribe(params)
		if err != nil {
			s.node.logger.log(newLogEntry(LogLevelError, "error decoding unsubscribe params", map[string]interface{}{"error": err.Error()}))
			rep.Error = apiproto.ErrBadRequest
			return rep, nil
		}
		resp := s.api.Unsubscribe(ctx, cmd)
		if resp.Error != nil {
			rep.Error = resp.Error
		} else {
			if resp.Result != nil {
				replyRes, err = encoder.EncodeUnsubscribe(resp.Result)
				if err != nil {
					return nil, err
				}
			}
		}
	case apiproto.MethodTypeDisconnect:
		cmd, err := decoder.DecodeDisconnect(params)
		if err != nil {
			s.node.logger.log(newLogEntry(LogLevelError, "error decoding disconnect params", map[string]interface{}{"error": err.Error()}))
			rep.Error = apiproto.ErrBadRequest
			return rep, nil
		}
		resp := s.api.Disconnect(ctx, cmd)
		if resp.Error != nil {
			rep.Error = resp.Error
		} else {
			if resp.Result != nil {
				replyRes, err = encoder.EncodeDisconnect(resp.Result)
				if err != nil {
					return nil, err
				}
			}
		}
	case apiproto.MethodTypePresence:
		cmd, err := decoder.DecodePresence(params)
		if err != nil {
			s.node.logger.log(newLogEntry(LogLevelError, "error decoding presence params", map[string]interface{}{"error": err.Error()}))
			rep.Error = apiproto.ErrBadRequest
			return rep, nil
		}
		resp := s.api.Presence(ctx, cmd)
		if resp.Error != nil {
			rep.Error = resp.Error
		} else {
			if resp.Result != nil {
				replyRes, err = encoder.EncodePresence(resp.Result)
				if err != nil {
					return nil, err
				}
			}
		}
	case apiproto.MethodTypePresenceStats:
		cmd, err := decoder.DecodePresenceStats(params)
		if err != nil {
			s.node.logger.log(newLogEntry(LogLevelError, "error decoding presence stats params", map[string]interface{}{"error": err.Error()}))
			rep.Error = apiproto.ErrBadRequest
			return rep, nil
		}
		resp := s.api.PresenceStats(ctx, cmd)
		if resp.Error != nil {
			rep.Error = resp.Error
		} else {
			if resp.Result != nil {
				replyRes, err = encoder.EncodePresenceStats(resp.Result)
				if err != nil {
					return nil, err
				}
			}
		}
	case apiproto.MethodTypeHistory:
		cmd, err := decoder.DecodeHistory(params)
		if err != nil {
			s.node.logger.log(newLogEntry(LogLevelError, "error decoding history params", map[string]interface{}{"error": err.Error()}))
			rep.Error = apiproto.ErrBadRequest
			return rep, nil
		}
		resp := s.api.History(ctx, cmd)
		if resp.Error != nil {
			rep.Error = resp.Error
		} else {
			if resp.Result != nil {
				replyRes, err = encoder.EncodeHistory(resp.Result)
				if err != nil {
					return nil, err
				}
			}
		}
	case apiproto.MethodTypeChannels:
		resp := s.api.Channels(ctx, &apiproto.ChannelsRequest{})
		if resp.Error != nil {
			rep.Error = resp.Error
		} else {
			if resp.Result != nil {
				replyRes, err = encoder.EncodeChannels(resp.Result)
				if err != nil {
					return nil, err
				}
			}
		}
	case apiproto.MethodTypeInfo:
		resp := s.api.Info(ctx, &apiproto.InfoRequest{})
		if resp.Error != nil {
			rep.Error = resp.Error
		} else {
			if resp.Result != nil {
				replyRes, err = encoder.EncodeInfo(resp.Result)
				if err != nil {
					return nil, err
				}
			}
		}
	default:
		rep.Error = apiproto.ErrMethodNotFound
	}

	if replyRes != nil {
		rep.Result = replyRes
	}

	return rep, nil
}

const (
	// We don't use specific websocket close codes because our client
	// have no notion about transport specifics.
	sockjsCloseStatus = 3000
)

type sockjsTransport struct {
	mu      sync.RWMutex
	closed  bool
	closeCh chan struct{}
	session sockjs.Session
	writer  *writer
}

func newSockjsTransport(s sockjs.Session, w *writer) *sockjsTransport {
	t := &sockjsTransport{
		session: s,
		writer:  w,
		closeCh: make(chan struct{}),
	}
	w.onWrite(t.write)
	return t
}

func (t *sockjsTransport) Name() string {
	return transportSockJS
}

func (t *sockjsTransport) Encoding() proto.Encoding {
	return proto.EncodingJSON
}

func (t *sockjsTransport) Send(reply *proto.PreparedReply) error {
	data := reply.Data()
	disconnect := t.writer.write(data)
	if disconnect != nil {
		// Close in goroutine to not block message broadcast.
		go t.Close(disconnect)
	}
	return nil
}

func (t *sockjsTransport) write(data ...[]byte) error {
	select {
	case <-t.closeCh:
		return nil
	default:
		for _, payload := range data {
			// TODO: can actually be sent in single message as streaming JSON.
			transportMessagesSent.WithLabelValues(transportSockJS).Inc()
			transportBytesOut.WithLabelValues(transportSockJS).Add(float64(len(data)))
			err := t.session.Send(string(payload))
			if err != nil {
				t.Close(&Disconnect{Reason: "error sending message", Reconnect: true})
				return err
			}
		}
		return nil
	}
}

func (t *sockjsTransport) Close(disconnect *Disconnect) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		// Already closed, noop.
		return nil
	}
	t.closed = true
	close(t.closeCh)
	if disconnect == nil {
		disconnect = DisconnectNormal
	}
	reason, err := json.Marshal(disconnect)
	if err != nil {
		return err
	}
	return t.session.Close(sockjsCloseStatus, string(reason))
}

// SockjsConfig ...
type SockjsConfig struct {
	// HandlerPrefix sets prefix for SockJS handler endpoint path.
	HandlerPrefix string

	// URL is URL address to SockJS client javascript library.
	URL string

	// HeartbeatDelay sets how often to send heartbeat frames to clients.
	HeartbeatDelay time.Duration

	// WebsocketReadBufferSize is a parameter that is used for raw websocket Upgrader.
	// If set to zero reasonable default value will be used.
	WebsocketReadBufferSize int

	// WebsocketWriteBufferSize is a parameter that is used for raw websocket Upgrader.
	// If set to zero reasonable default value will be used.
	WebsocketWriteBufferSize int
}

// SockjsHandler ...
type SockjsHandler struct {
	node    *Node
	config  SockjsConfig
	handler http.Handler
}

// NewSockjsHandler ...
func NewSockjsHandler(n *Node, c SockjsConfig) *SockjsHandler {
	sockjs.WebSocketReadBufSize = c.WebsocketReadBufferSize
	sockjs.WebSocketWriteBufSize = c.WebsocketWriteBufferSize

	options := sockjs.DefaultOptions

	// Override sockjs url. It's important to use the same SockJS
	// library version on client and server sides when using iframe
	// based SockJS transports, otherwise SockJS will raise error
	// about version mismatch.
	options.SockJSURL = c.URL

	options.HeartbeatDelay = c.HeartbeatDelay

	s := &SockjsHandler{
		node:   n,
		config: c,
	}

	handler := newSockJSHandler(s, c.HandlerPrefix, options)
	s.handler = handler
	return s
}

func (s *SockjsHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	s.handler.ServeHTTP(rw, r)
}

// newSockJSHandler returns SockJS handler bind to sockjsPrefix url prefix.
// SockJS handler has several handlers inside responsible for various tasks
// according to SockJS protocol.
func newSockJSHandler(s *SockjsHandler, sockjsPrefix string, sockjsOpts sockjs.Options) http.Handler {
	return sockjs.NewHandler(sockjsPrefix, sockjsOpts, s.sockJSHandler)
}

// sockJSHandler called when new client connection comes to SockJS endpoint.
func (s *SockjsHandler) sockJSHandler(sess sockjs.Session) {
	transportConnectCount.WithLabelValues(transportSockJS).Inc()

	// Separate goroutine for better GC of caller's data.
	go func() {
		config := s.node.Config()
		writerConf := writerConfig{
			MaxQueueSize: config.ClientQueueMaxSize,
		}
		writer := newWriter(writerConf)
		defer writer.close()
		transport := newSockjsTransport(sess, writer)
		c := newClient(sess.Request().Context(), s.node, transport, clientConfig{})
		defer c.Close(nil)

		s.node.logger.log(newLogEntry(LogLevelDebug, "SockJS connection established", map[string]interface{}{"client": c.ID()}))
		defer func(started time.Time) {
			s.node.logger.log(newLogEntry(LogLevelDebug, "SockJS connection completed", map[string]interface{}{"client": c.ID(), "time": time.Since(started)}))
		}(time.Now())

		for {
			if msg, err := sess.Recv(); err == nil {
				ok := handleClientData(s.node, c, []byte(msg), transport, writer)
				if !ok {
					return
				}
				continue
			}
			break
		}
	}()
}

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
		} else {
			if t.opts.writeTimeout > 0 {
				t.conn.SetWriteDeadline(time.Time{})
			}
			transportMessagesSent.WithLabelValues(transportWebsocket).Add(float64(len(data)))
			transportBytesOut.WithLabelValues(transportWebsocket).Add(float64(bytesOut))
		}
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
	// WebsocketCompression allows to enable websocket permessage-deflate
	// compression support for raw websocket connections. It does not guarantee
	// that compression will be used - i.e. it only says that Centrifugo will
	// try to negotiate it with client.
	WebsocketCompression bool

	// WebsocketCompressionLevel sets a level for websocket compression.
	// See posiible value description at https://golang.org/pkg/compress/flate/#NewWriter
	WebsocketCompressionLevel int

	// WebsocketCompressionMinSize allows to set minimal limit in bytes for message to use
	// compression when writing it into client connection. By default it's 0 - i.e. all messages
	// will be compressed when WebsocketCompression enabled and compression negotiated with client.
	WebsocketCompressionMinSize int

	// WebsocketReadBufferSize is a parameter that is used for raw websocket Upgrader.
	// If set to zero reasonable default value will be used.
	WebsocketReadBufferSize int

	// WebsocketWriteBufferSize is a parameter that is used for raw websocket Upgrader.
	// If set to zero reasonable default value will be used.
	WebsocketWriteBufferSize int
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

	wsCompression := s.config.WebsocketCompression
	wsCompressionLevel := s.config.WebsocketCompressionLevel
	wsCompressionMinSize := s.config.WebsocketCompressionMinSize
	wsReadBufferSize := s.config.WebsocketReadBufferSize
	wsWriteBufferSize := s.config.WebsocketWriteBufferSize

	upgrader := websocket.Upgrader{
		ReadBufferSize:    wsReadBufferSize,
		WriteBufferSize:   wsWriteBufferSize,
		EnableCompression: wsCompression,
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

	if wsCompression {
		err := conn.SetCompressionLevel(wsCompressionLevel)
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
			compressionMinSize: wsCompressionMinSize,
			enc:                enc,
		}
		writerConf := writerConfig{
			MaxQueueSize: config.ClientQueueMaxSize,
		}
		writer := newWriter(writerConf)
		defer writer.close()
		transport := newWebsocketTransport(conn, writer, opts)
		c := newClient(r.Context(), s.node, transport, clientConfig{})
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

type writerConfig struct {
	MaxQueueSize int
}

// writer helps to manage per-connection message queue.
type writer struct {
	mu       sync.Mutex
	config   writerConfig
	writeFn  func(...[]byte) error
	messages queue.Queue
	closed   bool
}

func newWriter(config writerConfig) *writer {
	w := &writer{
		config:   config,
		messages: queue.New(),
	}
	go w.runWriteRoutine()
	return w
}

const (
	mergeQueueMessages = true
	maxMessagesInFrame = 4
)

func (w *writer) runWriteRoutine() {
	for {
		// Wait for message from queue.
		msg, ok := w.messages.Wait()
		if !ok {
			if w.messages.Closed() {
				return
			}
			continue
		}

		var writeErr error

		messageCount := w.messages.Len()
		if mergeQueueMessages && messageCount > 0 {
			// There are several more messages left in queue, try to send them in single frame,
			// but no more than maxMessagesInFrame.

			// Limit message count to get from queue with (maxMessagesInFrame - 1)
			// (as we already have one message received from queue above).
			messagesCap := messageCount + 1
			if messagesCap > maxMessagesInFrame {
				messagesCap = maxMessagesInFrame
			}

			msgs := make([][]byte, 0, messagesCap)
			msgs = append(msgs, msg)

			for messageCount > 0 {
				messageCount--
				if len(msgs) >= maxMessagesInFrame {
					break
				}
				msg, ok := w.messages.Remove()
				if ok {
					msgs = append(msgs, msg)
				} else {
					if w.messages.Closed() {
						return
					}
					break
				}
			}
			if len(msgs) > 0 {
				writeErr = w.writeFn(msgs...)
			}
		} else {
			// Write single message without allocating new [][]byte slice.
			writeErr = w.writeFn(msg)
		}
		if writeErr != nil {
			// Write failed, transport must close itself, here we just return from routine.
			return
		}
	}
}

func (w *writer) write(data []byte) *Disconnect {
	ok := w.messages.Add(data)
	if !ok {
		return nil
	}
	if w.messages.Size() > w.config.MaxQueueSize {
		return &Disconnect{Reason: "slow", Reconnect: true}
	}
	return nil
}

func (w *writer) onWrite(writeFn func(...[]byte) error) {
	w.writeFn = writeFn
}

func (w *writer) close() error {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return nil
	}
	w.messages.Close()
	w.closed = true
	w.mu.Unlock()
	return nil
}

// common data handling logic for Websocket and Sockjs handlers.
func handleClientData(n *Node, c *client, data []byte, transport Transport, writer *writer) bool {
	transportBytesIn.WithLabelValues(transport.Name()).Add(float64(len(data)))

	if len(data) == 0 {
		n.logger.log(newLogEntry(LogLevelError, "empty client request received"))
		transport.Close(DisconnectBadRequest)
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
			transport.Close(DisconnectBadRequest)
			proto.PutCommandDecoder(enc, decoder)
			proto.PutReplyEncoder(enc, encoder)
			return false
		}
		rep, disconnect := c.handle(cmd)
		if disconnect != nil {
			n.logger.log(newLogEntry(LogLevelInfo, "disconnect after handling command", map[string]interface{}{"command": fmt.Sprintf("%v", cmd), "client": c.ID(), "user": c.UserID(), "reason": disconnect.Reason}))
			transport.Close(disconnect)
			proto.PutCommandDecoder(enc, decoder)
			proto.PutReplyEncoder(enc, encoder)
			return false
		}
		if rep != nil {
			err = encoder.Encode(rep)
			numReplies++
			if err != nil {
				n.logger.log(newLogEntry(LogLevelError, "error encoding reply", map[string]interface{}{"reply": fmt.Sprintf("%v", rep), "client": c.ID(), "user": c.UserID(), "error": err.Error()}))
				transport.Close(DisconnectServerError)
				return false
			}
		}
	}

	if numReplies > 0 {
		disconnect := writer.write(encoder.Finish())
		if disconnect != nil {
			n.logger.log(newLogEntry(LogLevelInfo, "disconnect after sending data to transport", map[string]interface{}{"client": c.ID(), "user": c.UserID(), "reason": disconnect.Reason}))
			transport.Close(disconnect)
			proto.PutCommandDecoder(enc, decoder)
			proto.PutReplyEncoder(enc, encoder)
			return false
		}
	}

	proto.PutCommandDecoder(enc, decoder)
	proto.PutReplyEncoder(enc, encoder)

	return true
}
