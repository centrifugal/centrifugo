package centrifuge

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/centrifugal/centrifuge/internal/proto"
	"github.com/igm/sockjs-go/sockjs"
)

const (
	transportSockJS = "sockjs"
)

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
		c := newClient(sess.Request().Context(), s.node, transport)
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
