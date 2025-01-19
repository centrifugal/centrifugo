package wt

import (
	"context"
	"net/http"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/logging"

	"github.com/centrifugal/centrifuge"
	"github.com/centrifugal/protocol"
	"github.com/quic-go/webtransport-go"
	"github.com/rs/zerolog/log"
)

// Handler for WebTransport.
type Handler struct {
	config   Config
	server   *webtransport.Server
	node     *centrifuge.Node
	pingPong centrifuge.PingPongConfig
}

// NewHandler creates new Handler.
func NewHandler(node *centrifuge.Node, wtServer *webtransport.Server, config Config, pingPong centrifuge.PingPongConfig) *Handler {
	return &Handler{config: config, server: wtServer, node: node, pingPong: pingPong}
}

const bidiStreamAcceptTimeout = 10 * time.Second

// ServeHTTP upgrades connection to WebTransport and creates centrifuge.Client from it.
func (s *Handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	conn, err := s.server.Upgrade(rw, r)
	if err != nil {
		log.Info().Str("transport", transportName).Err(err).Msg("error upgrading to webtransport")
		rw.WriteHeader(http.StatusBadRequest)
		return
	}
	rw.WriteHeader(http.StatusOK)

	acceptCtx, acceptCtxCancel := context.WithTimeout(r.Context(), bidiStreamAcceptTimeout)
	stream, err := conn.AcceptStream(acceptCtx)
	if err != nil {
		acceptCtxCancel()
		log.Error().Err(err).Msg("stream accept error")
		rw.WriteHeader(http.StatusBadRequest)
		return
	}
	acceptCtxCancel()

	protoType := centrifuge.ProtocolTypeJSON
	if r.URL.RawQuery != "" && r.URL.Query().Get("cf_protocol") == "protobuf" {
		protoType = centrifuge.ProtocolTypeProtobuf
	}

	transport := newWebtransportTransport(protoType, conn, stream, s.pingPong)
	c, closeFn, err := centrifuge.NewClient(r.Context(), s.node, transport)
	if err != nil {
		log.Error().Err(err).Msg("error creating client")
		return
	}
	defer func() { _ = closeFn() }()

	if logging.Enabled(logging.DebugLevel) {
		log.Debug().Str("transport", transportName).Str("client", c.ID()).Msg("client connection established")
		defer func(started time.Time) {
			log.Debug().Str("transport", transportName).Str("client", c.ID()).Str("duration", time.Since(started).String()).Msg("client connection completed")
		}(time.Now())
	}

	var decoder protocol.StreamCommandDecoder
	if protoType == centrifuge.ProtocolTypeJSON {
		decoder = protocol.GetStreamCommandDecoderLimited(protocol.TypeJSON, stream, int64(s.config.MessageSizeLimit))
		defer protocol.PutStreamCommandDecoder(protocol.TypeJSON, decoder)
	} else {
		decoder = protocol.NewProtobufStreamCommandDecoder(stream, int64(s.config.MessageSizeLimit))
		defer protocol.PutStreamCommandDecoder(protocol.TypeProtobuf, decoder)
	}

	for {
		cmd, cmdSize, err := decoder.Decode()
		if err != nil {
			log.Error().Err(err).Str("transport", transportName).Str("client", c.ID()).Msg("error decoding command")
			return
		}
		ok := c.HandleCommand(cmd, cmdSize)
		if !ok {
			return
		}
	}
}
