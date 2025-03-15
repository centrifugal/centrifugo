package api

import (
	"net/http"

	. "github.com/centrifugal/centrifugo/v6/internal/apiproto"

	"github.com/centrifugal/centrifuge"
	"github.com/rs/zerolog/log"
)

// Config configures Handler.
type Config struct {
	UseOpenTelemetry      bool
	UseTransportErrorMode bool
}

// Handler is responsible for processing API commands over HTTP.
type Handler struct {
	mux    *http.ServeMux
	node   *centrifuge.Node
	config Config
	api    *Executor
}

var paramsDecoder = NewJSONRequestDecoder()
var responseEncoder = NewJSONResponseEncoder()

// NewHandler creates new Handler.
func NewHandler(n *centrifuge.Node, apiExecutor *Executor, c Config) *Handler {
	m := new(http.ServeMux)
	h := &Handler{
		mux:    m,
		node:   n,
		config: c,
		api:    apiExecutor,
	}
	return h
}

func (s *Handler) Routes() map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"/batch":          s.handleBatch,
		"/publish":        s.handlePublish,
		"/broadcast":      s.handleBroadcast,
		"/subscribe":      s.handleSubscribe,
		"/unsubscribe":    s.handleUnsubscribe,
		"/disconnect":     s.handleDisconnect,
		"/presence":       s.handlePresence,
		"/presence_stats": s.handlePresenceStats,
		"/history":        s.handleHistory,
		"/history_remove": s.handleHistoryRemove,
		"/info":           s.handleInfo,
		"/rpc":            s.handleRPC,
		"/refresh":        s.handleRefresh,
		"/channels":       s.handleChannels,
	}
}

func (s *Handler) handleReadDataError(r *http.Request, w http.ResponseWriter, err error) {
	log.Error().Err(err).Str("path", r.URL.Path).Msg("error reading API request body")
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}

func (s *Handler) handleUnmarshalError(r *http.Request, w http.ResponseWriter, err error) {
	log.Error().Err(err).Str("path", r.URL.Path).Msg("error decoding API request body")
	http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
}

func (s *Handler) handleMarshalError(r *http.Request, w http.ResponseWriter, err error) {
	log.Error().Err(err).Str("path", r.URL.Path).Msg("error encoding API response")
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}

func (s *Handler) writeJson(w http.ResponseWriter, data []byte) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}

func (s *Handler) writeJsonCustomStatus(w http.ResponseWriter, statusCode int, data []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_, _ = w.Write(data)
}

func (s *Handler) useTransportErrorMode(r *http.Request) bool {
	return s.config.UseTransportErrorMode || r.Header.Get("X-Centrifugo-Error-Mode") == "transport"
}
