package wt

import (
	"context"
	"net/http"
	"time"

	"github.com/centrifugal/centrifuge"
	"github.com/centrifugal/protocol"
	"github.com/quic-go/webtransport-go"
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
		s.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "error upgrading to webtransport", map[string]any{"error": err.Error()}))
		rw.WriteHeader(http.StatusBadRequest)
		return
	}
	rw.WriteHeader(http.StatusOK)

	acceptCtx, acceptCtxCancel := context.WithTimeout(r.Context(), bidiStreamAcceptTimeout)
	stream, err := conn.AcceptStream(acceptCtx)
	if err != nil {
		acceptCtxCancel()
		s.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "stream accept error", map[string]any{"error": err.Error()}))
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
		s.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error creating client", map[string]any{"transport": transportName}))
		return
	}
	defer func() { _ = closeFn() }()

	if s.node.LogEnabled(centrifuge.LogLevelDebug) {
		s.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelDebug, "client connection established", map[string]any{"client": c.ID(), "transport": transportName}))
		defer func(started time.Time) {
			s.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelDebug, "client connection completed", map[string]any{"client": c.ID(), "transport": transportName, "duration": time.Since(started).String()}))
		}(time.Now())
	}

	var decoder protocol.StreamCommandDecoder
	if protoType == centrifuge.ProtocolTypeJSON {
		decoder = protocol.NewJSONStreamCommandDecoder(stream)
	} else {
		decoder = protocol.NewProtobufStreamCommandDecoder(stream)
	}

	for {
		cmd, cmdSize, err := decoder.Decode()
		if err != nil {
			return
		}
		ok := c.HandleCommand(cmd, cmdSize)
		if !ok {
			return
		}
	}
}
