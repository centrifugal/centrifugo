package uniws

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/logging"

	"github.com/centrifugal/centrifuge"
	"github.com/centrifugal/protocol"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

// Handler handles WebSocket client connections. Usually WebSocket protocol
// is a bidirectional connection between a client and a server for low-latency
// communication. Here we utilize only one direction - giving users an additional
// option for unidirectional transport.
type Handler struct {
	node     *centrifuge.Node
	upgrade  *websocket.Upgrader
	config   Config
	pingPong centrifuge.PingPongConfig
}

var writeBufferPool = &sync.Pool{}

// NewHandler creates new Handler.
func NewHandler(
	n *centrifuge.Node, c Config, CheckOrigin func(r *http.Request) bool, pingPong centrifuge.PingPongConfig,
) *Handler {
	upgrade := &websocket.Upgrader{
		ReadBufferSize:    c.ReadBufferSize,
		EnableCompression: c.Compression,
	}
	if c.UseWriteBufferPool {
		upgrade.WriteBufferPool = writeBufferPool
	} else {
		upgrade.WriteBufferSize = c.WriteBufferSize
	}
	if CheckOrigin != nil {
		upgrade.CheckOrigin = CheckOrigin
	} else {
		upgrade.CheckOrigin = sameHostOriginCheck()
	}
	return &Handler{
		node:     n,
		config:   c,
		upgrade:  upgrade,
		pingPong: pingPong,
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
		log.Error().Err(err).Str("transport", transportName).Msg("websocket upgrade error")
		return
	}

	if compression {
		err := conn.SetCompressionLevel(compressionLevel)
		if err != nil {
			log.Error().Err(err).Msg("websocket error setting compression level")
		}
	}

	pingInterval := s.pingPong.PingInterval
	if pingInterval == 0 {
		pingInterval = DefaultWebsocketPingInterval
	}
	writeTimeout := s.config.WriteTimeout.ToDuration()
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
			pingPongConfig:     s.pingPong,
			joinMessages:       s.config.JoinPushMessages,
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
			log.Error().Err(err).Str("transport", transportName).Msg("error creating client")
			return
		}
		defer func() { _ = closeFn() }()

		if logging.Enabled(logging.DebugLevel) {
			log.Debug().Str("transport", transportName).Str("client", c.ID()).Msg("client connection established")
			defer func(started time.Time) {
				log.Debug().Str("transport", transportName).Str("client", c.ID()).
					Str("duration", time.Since(started).String()).Msg("client connection completed")
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
