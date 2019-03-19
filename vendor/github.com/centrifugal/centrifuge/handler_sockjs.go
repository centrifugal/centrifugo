package centrifuge

import (
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/centrifugal/centrifuge/internal/proto"

	"github.com/igm/sockjs-go/sockjs"
)

const (
	transportSockJS = "sockjs"
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

func (t *sockjsTransport) Info() TransportInfo {
	return TransportInfo{
		Request: t.session.Request(),
	}
}

func (t *sockjsTransport) Send(reply *preparedReply) error {
	data := reply.Data()
	disconnect := t.writer.write(data)
	if disconnect != nil {
		// Close in goroutine to not block message broadcast.
		go t.Close(disconnect)
		return io.EOF
	}
	return nil
}

func (t *sockjsTransport) write(data ...[]byte) error {
	select {
	case <-t.closeCh:
		return nil
	default:
		if len(data) == 1 {
			payload := data[0]
			err := t.session.Send(string(payload))
			if err != nil {
				go t.Close(DisconnectWriteError)
				return err
			}
			transportMessagesSent.WithLabelValues(transportSockJS).Inc()
		} else {
			buf := getBuffer()
			for _, payload := range data {
				buf.Write(payload)
			}
			err := t.session.Send(buf.String())
			if err != nil {
				go t.Close(DisconnectWriteError)
				putBuffer(buf)
				return err
			}
			putBuffer(buf)
			transportMessagesSent.WithLabelValues(transportSockJS).Add(float64(len(data)))
		}
		return nil
	}
}

func (t *sockjsTransport) Close(disconnect *Disconnect) error {
	t.mu.Lock()
	if t.closed {
		// Already closed, noop.
		t.mu.Unlock()
		return nil
	}
	t.closed = true
	t.mu.Unlock()

	// Send messages remaining in queue.
	t.writer.close()

	t.mu.Lock()
	close(t.closeCh)
	t.mu.Unlock()

	if disconnect == nil {
		disconnect = DisconnectNormal
	}
	reason, err := json.Marshal(disconnect)
	if err != nil {
		return err
	}
	return t.session.Close(uint32(disconnect.Code), string(reason))
}

// SockjsConfig represents config for SockJS handler.
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

// SockjsHandler accepts SockJS connections.
type SockjsHandler struct {
	node    *Node
	config  SockjsConfig
	handler http.Handler
}

// NewSockjsHandler creates new SockjsHandler.
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

		select {
		case <-s.node.NotifyShutdown():
			transport.Close(DisconnectShutdown)
			return
		default:
		}

		c, err := newClient(sess.Request().Context(), s.node, transport)
		if err != nil {
			s.node.logger.log(newLogEntry(LogLevelError, "error creating client", map[string]interface{}{"transport": transportSockJS}))
			return
		}
		s.node.logger.log(newLogEntry(LogLevelDebug, "client connection established", map[string]interface{}{"client": c.ID(), "transport": transportSockJS}))
		defer func(started time.Time) {
			s.node.logger.log(newLogEntry(LogLevelDebug, "client connection completed", map[string]interface{}{"client": c.ID(), "transport": transportSockJS, "duration": time.Since(started)}))
		}(time.Now())
		defer c.close(nil)

		for {
			if msg, err := sess.Recv(); err == nil {
				ok := c.handleRawData([]byte(msg), writer)
				if !ok {
					return
				}
				continue
			}
			break
		}
	}()
}
