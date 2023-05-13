package uniws

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/centrifugal/centrifuge"
	"github.com/centrifugal/protocol"
	"github.com/gorilla/websocket"
)

// Handler handles WebSocket client connections. Usually WebSocket protocol
// is a bidirectional connection between a client and a server for low-latency
// communication. Here we utilize only one direction - giving users an additional
// option for unidirectional transport.
type Handler struct {
	node    *centrifuge.Node
	upgrade *websocket.Upgrader
	config  Config
}

var writeBufferPool = &sync.Pool{}

// NewHandler creates new Handler.
func NewHandler(n *centrifuge.Node, c Config) *Handler {
	upgrade := &websocket.Upgrader{
		ReadBufferSize:    c.ReadBufferSize,
		EnableCompression: c.Compression,
	}
	if c.UseWriteBufferPool {
		upgrade.WriteBufferPool = writeBufferPool
	} else {
		upgrade.WriteBufferSize = c.WriteBufferSize
	}
	if c.CheckOrigin != nil {
		upgrade.CheckOrigin = c.CheckOrigin
	} else {
		upgrade.CheckOrigin = sameHostOriginCheck()
	}
	return &Handler{
		node:    n,
		config:  c,
		upgrade: upgrade,
	}
}

type ConnectRequest struct {
	Token   string                       `json:"token,omitempty"`
	Data    json.RawMessage              `json:"data,omitempty"`
	Subs    map[string]*SubscribeRequest `json:"subs,omitempty"`
	Name    string                       `json:"name,omitempty"`
	Version string                       `json:"version,omitempty"`
}

type SubscribeRequest struct {
	Recover bool   `json:"recover,omitempty"`
	Epoch   string `json:"epoch,omitempty"`
	Offset  uint64 `json:"offset,omitempty"`
}

func (s *Handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	compression := s.config.Compression
	compressionLevel := s.config.CompressionLevel
	compressionMinSize := s.config.CompressionMinSize

	conn, err := s.upgrade.Upgrade(rw, r, nil)
	if err != nil {
		s.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelDebug, "websocket upgrade error", map[string]interface{}{"error": err.Error()}))
		return
	}

	if compression {
		err := conn.SetCompressionLevel(compressionLevel)
		if err != nil {
			s.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "websocket error setting compression level", map[string]interface{}{"error": err.Error()}))
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

	// Separate goroutine for better GC of caller's data.
	go func() {
		opts := websocketTransportOptions{
			pingInterval:       pingInterval,
			writeTimeout:       writeTimeout,
			compressionMinSize: compressionMinSize,
			pingPongConfig:     s.config.PingPongConfig,
		}

		graceCh := make(chan struct{})
		transport := newWebsocketTransport(conn, opts, graceCh)

		select {
		case <-s.node.NotifyShutdown():
			_ = transport.Close(centrifuge.DisconnectShutdown)
			return
		default:
		}

		ctxCh := make(chan struct{})
		defer close(ctxCh)

		c, closeFn, err := centrifuge.NewClient(NewCancelContext(r.Context(), ctxCh), s.node, transport)
		if err != nil {
			s.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error creating client", map[string]interface{}{"transport": transport.Name()}))
			return
		}
		defer func() { _ = closeFn() }()

		if s.node.LogEnabled(centrifuge.LogLevelDebug) {
			s.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelDebug, "client connection established", map[string]interface{}{"client": c.ID(), "transport": transport.Name()}))
			defer func(started time.Time) {
				s.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelDebug, "client connection completed", map[string]interface{}{"client": c.ID(), "transport": transport.Name(), "duration": time.Since(started)}))
			}(time.Now())
		}

		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}

		var req protocol.ConnectRequest
		err = json.Unmarshal(data, &req)
		if err != nil {
			return
		}

		connectRequest := centrifuge.ConnectRequest{
			Token:   req.Token,
			Data:    req.Data,
			Name:    req.Name,
			Version: req.Version,
		}
		if req.Subs != nil {
			subs := make(map[string]centrifuge.SubscribeRequest, len(req.Subs))
			for k, v := range req.Subs {
				subs[k] = centrifuge.SubscribeRequest{
					Recover: v.Recover,
					Offset:  v.Offset,
					Epoch:   v.Epoch,
				}
			}
			connectRequest.Subs = subs
		}

		c.Connect(connectRequest)

		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				break
			}
		}

		// https://github.com/gorilla/websocket/issues/448
		conn.SetPingHandler(nil)
		conn.SetPongHandler(nil)
		conn.SetCloseHandler(nil)
		_ = conn.SetReadDeadline(time.Now().Add(closeFrameWait))
		for {
			if _, _, err := conn.NextReader(); err != nil {
				close(graceCh)
				break
			}
		}
	}()
}
