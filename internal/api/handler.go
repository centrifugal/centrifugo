package api

import (
	"context"
	"errors"
	"io"
	"net/http"

	. "github.com/centrifugal/centrifugo/v6/internal/apiproto"

	"github.com/centrifugal/centrifuge"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
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

// OldRoute handles all methods inside one /api handler.
// The plan is to remove it in Centrifugo v6.
func (s *Handler) OldRoute() http.HandlerFunc {
	return s.handleAPI
}

func (s *Handler) handleAPI(w http.ResponseWriter, r *http.Request) {
	select {
	case <-s.node.NotifyShutdown():
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	default:
	}

	var data []byte
	var err error

	data, err = io.ReadAll(r.Body)
	if err != nil {
		log.Error().Err(err).Msg("error reading API request body")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if len(data) == 0 {
		log.Error().Err(errors.New("no data in API request")).Msg("bad request")
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	encoder := GetReplyEncoder()
	defer PutReplyEncoder(encoder)

	decoder := GetCommandDecoder(data)
	defer PutCommandDecoder(decoder)

	for {
		command, decodeErr := decoder.Decode()
		if decodeErr != nil && decodeErr != io.EOF {
			log.Error().Err(decodeErr).Msg("error decoding API data")
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}
		if command != nil {
			if s.config.UseOpenTelemetry {
				span := trace.SpanFromContext(r.Context())
				span.SetAttributes(attribute.String("method", Command_MethodType_name[int32(command.Method)]))
			}
			rep, err := s.handleAPICommand(r.Context(), command)
			if err != nil {
				if s.config.UseOpenTelemetry {
					span := trace.SpanFromContext(r.Context())
					span.SetStatus(codes.Error, err.Error())
				}
				log.Error().Err(err).Msg("error handling API command")
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}
			if s.config.UseOpenTelemetry && rep.Error != nil {
				span := trace.SpanFromContext(r.Context())
				span.SetStatus(codes.Error, rep.Error.Error())
			}
			err = encoder.Encode(rep)
			if err != nil {
				log.Error().Err(err).Msg("error encoding API reply")
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}
		}
		if decodeErr == io.EOF {
			break
		}
	}
	s.writeJson(w, encoder.Finish())
}

func (s *Handler) handleAPICommand(ctx context.Context, cmd *Command) (*Reply, error) {

	method := cmd.Method
	params := cmd.Params

	rep := &Reply{
		Id: cmd.Id,
	}

	var replyRes Raw

	decoder := GetParamsDecoder()
	defer PutParamsDecoder(decoder)

	encoder := GetResultEncoder()
	defer PutResultEncoder(encoder)

	switch method {
	case Command_PUBLISH:
		cmd, err := decoder.DecodePublish(params)
		if err != nil {
			log.Error().Err(err).Msg("error decoding publish params")
			rep.Error = ErrorBadRequest
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
	case Command_BROADCAST:
		cmd, err := decoder.DecodeBroadcast(params)
		if err != nil {
			log.Error().Err(err).Msg("error decoding broadcast params")
			rep.Error = ErrorBadRequest
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
	case Command_SUBSCRIBE:
		cmd, err := decoder.DecodeSubscribe(params)
		if err != nil {
			log.Error().Err(err).Msg("error decoding subscribe params")
			rep.Error = ErrorBadRequest
			return rep, nil
		}
		resp := s.api.Subscribe(ctx, cmd)
		if resp.Error != nil {
			rep.Error = resp.Error
		} else {
			if resp.Result != nil {
				replyRes, err = encoder.EncodeSubscribe(resp.Result)
				if err != nil {
					return nil, err
				}
			}
		}
	case Command_UNSUBSCRIBE:
		cmd, err := decoder.DecodeUnsubscribe(params)
		if err != nil {
			log.Error().Err(err).Msg("error decoding unsubscribe params")
			rep.Error = ErrorBadRequest
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
	case Command_DISCONNECT:
		cmd, err := decoder.DecodeDisconnect(params)
		if err != nil {
			log.Error().Err(err).Msg("error decoding disconnect params")
			rep.Error = ErrorBadRequest
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
	case Command_PRESENCE:
		cmd, err := decoder.DecodePresence(params)
		if err != nil {
			log.Error().Err(err).Msg("error decoding presence params")
			rep.Error = ErrorBadRequest
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
	case Command_PRESENCE_STATS:
		cmd, err := decoder.DecodePresenceStats(params)
		if err != nil {
			log.Error().Err(err).Msg("error decoding presence stats params")
			rep.Error = ErrorBadRequest
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
	case Command_HISTORY:
		cmd, err := decoder.DecodeHistory(params)
		if err != nil {
			log.Error().Err(err).Msg("error decoding history params")
			rep.Error = ErrorBadRequest
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
	case Command_HISTORY_REMOVE:
		cmd, err := decoder.DecodeHistoryRemove(params)
		if err != nil {
			log.Error().Err(err).Msg("error decoding history remove params")
			rep.Error = ErrorBadRequest
			return rep, nil
		}
		resp := s.api.HistoryRemove(ctx, cmd)
		if resp.Error != nil {
			rep.Error = resp.Error
		} else {
			if resp.Result != nil {
				replyRes, err = encoder.EncodeHistoryRemove(resp.Result)
				if err != nil {
					return nil, err
				}
			}
		}
	case Command_INFO:
		resp := s.api.Info(ctx, &InfoRequest{})
		if resp.Error != nil {
			rep.Error = resp.Error
		} else {
			if resp.Result != nil {
				var err error
				replyRes, err = encoder.EncodeInfo(resp.Result)
				if err != nil {
					return nil, err
				}
			}
		}
	case Command_RPC:
		cmd, err := decoder.DecodeRPC(params)
		if err != nil {
			log.Error().Err(err).Msg("error decoding rpc params")
			rep.Error = ErrorBadRequest
			return rep, nil
		}
		resp := s.api.RPC(ctx, cmd)
		if resp.Error != nil {
			rep.Error = resp.Error
		} else {
			if resp.Result != nil {
				replyRes, err = encoder.EncodeRPC(resp.Result)
				if err != nil {
					return nil, err
				}
			}
		}
	case Command_REFRESH:
		cmd, err := decoder.DecodeRefresh(params)
		if err != nil {
			log.Error().Err(err).Msg("error decoding refresh params")
			rep.Error = ErrorBadRequest
			return rep, nil
		}
		resp := s.api.Refresh(ctx, cmd)
		if resp.Error != nil {
			rep.Error = resp.Error
		} else {
			if resp.Result != nil {
				replyRes, err = encoder.EncodeRefresh(resp.Result)
				if err != nil {
					return nil, err
				}
			}
		}
	case Command_CHANNELS:
		cmd, err := decoder.DecodeChannels(params)
		if err != nil {
			log.Error().Err(err).Msg("error decoding channels params")
			rep.Error = ErrorBadRequest
			return rep, nil
		}
		resp := s.api.Channels(ctx, cmd)
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
	default:
		rep.Error = ErrorNotFound
	}

	if replyRes != nil {
		rep.Result = replyRes
	}

	return rep, nil
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
