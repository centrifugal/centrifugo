package uniws

import (
	"net/http"
	"sync"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/convert"
	"github.com/centrifugal/centrifugo/v6/internal/logging"
	"github.com/centrifugal/centrifugo/v6/internal/websocket"

	"github.com/centrifugal/centrifuge"
	"github.com/centrifugal/protocol"
	"github.com/rs/zerolog/log"
	"github.com/segmentio/encoding/json"
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

// Support passing connect request in URL params.
// The value should be a properly encoded JSON object representing protocol.ConnectRequest.
const connectUrlParam = "cf_connect"

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

func (s *Handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	compression := s.config.Compression
	compressionLevel := s.config.CompressionLevel
	compressionMinSize := s.config.CompressionMinSize

	var req *protocol.ConnectRequest
	connectRequestString := r.URL.Query().Get(connectUrlParam)
	if connectRequestString != "" {
		_, err := json.Parse(convert.StringToBytes(connectRequestString), &req, json.ZeroCopy)
		if err != nil {
			log.Info().Err(err).Str("transport", transportName).Msg("error unmarshalling connect request")
			http.Error(rw, "invalid connect request", http.StatusBadRequest)
			return
		}
	}

	conn, _, err := s.upgrade.Upgrade(rw, r, nil)
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

	framePingInterval := s.pingPong.PingInterval
	if framePingInterval <= 0 {
		// Always use frame ping/pong in unidirectional WebSocket transport.
		framePingInterval = DefaultWebsocketPingInterval
	}
	framePongTimeout := s.pingPong.PongTimeout
	if framePongTimeout <= 0 {
		framePongTimeout = DefaultWebsocketPongTimeout
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

	// Separate goroutine for better GC of caller's data.
	go func() {
		opts := websocketTransportOptions{
			framePingInterval:       framePingInterval,
			framePongTimeout:        framePongTimeout,
			writeTimeout:            writeTimeout,
			compressionMinSize:      compressionMinSize,
			pingPongConfig:          s.pingPong,
			joinMessages:            s.config.JoinPushMessages,
			protoMajor:              r.ProtoMajor,
			disableClosingHandshake: s.config.DisableClosingHandshake,
			disableDisconnectPush:   s.config.DisableDisconnectPush,
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

		pongWait := framePingInterval + framePongTimeout
		_ = conn.SetReadDeadline(time.Now().Add(pongWait))
		conn.SetPongHandler(func([]byte) error {
			_ = conn.SetReadDeadline(time.Now().Add(pongWait))
			return nil
		})

		waitClose := func() {
			// https://github.com/gorilla/websocket/issues/448
			conn.SetPingHandler(nil)
			conn.SetPongHandler(nil)
			if s.config.DisableClosingHandshake {
				close(graceCh)
				return
			}
			_ = conn.SetReadDeadline(time.Now().Add(closeFrameWait))
			for {
				if _, _, err := conn.NextReader(); err != nil {
					close(graceCh)
					return
				}
			}
		}

		if req == nil {
			_, data, err := conn.ReadMessage()
			if err != nil {
				waitClose()
				return
			}
			_, err = json.Parse(data, &req, json.ZeroCopy)
			if err != nil {
				log.Info().Err(err).Str("transport", transportName).Msg("error unmarshalling connect request")
				if !s.config.DisableClosingHandshake {
					err = conn.WriteControl(
						websocket.CloseMessage,
						websocket.FormatCloseMessage(
							int(centrifuge.DisconnectBadRequest.Code),
							centrifuge.DisconnectBadRequest.Reason,
						),
						time.Now().Add(writeTimeout))
					if err != nil {
						return
					}
				}
				waitClose()
				return
			}
		}

		c.ProtocolConnect(req)

		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				break
			}
		}
		waitClose()
	}()
}
