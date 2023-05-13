package api

import (
	"context"
	"io"
	"net/http"

	. "github.com/centrifugal/centrifugo/v4/internal/apiproto"

	"github.com/centrifugal/centrifuge"
)

// Config configures APIHandler.
type Config struct{}

// Handler is responsible for processing API commands over HTTP.
type Handler struct {
	mux    *http.ServeMux
	node   *centrifuge.Node
	config Config
	api    *Executor
}

var paramsDecoder = NewJSONRequestDecoder()
var responseEncoder = NewJSONResponseEncoder()

// NewHandler creates new APIHandler.
func NewHandler(n *centrifuge.Node, apiExecutor *Executor, c Config) *Handler {
	m := new(http.ServeMux)
	h := &Handler{
		mux:    m,
		node:   n,
		config: c,
		api:    apiExecutor,
	}
	h.RegisterRoutes(m)
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

func (s *Handler) RegisterRoutes(m *http.ServeMux) {
	for path, handler := range s.Routes() {
		m.HandleFunc(path, handler)
	}
}

// OldRoute handles all methods inside one /api handler.
// The plan is to remove it in Centrifugo v6.
func (s *Handler) OldRoute() http.HandlerFunc {
	return s.handleAPI
}

func (s *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
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
		s.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error reading API request body", map[string]interface{}{"error": err.Error()}))
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if len(data) == 0 {
		s.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "no data in API request"))
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
			s.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error decoding API data", map[string]interface{}{"error": decodeErr.Error()}))
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}
		if command != nil {
			rep, err := s.handleAPICommand(r.Context(), command)
			if err != nil {
				s.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error handling API command", map[string]interface{}{"error": err.Error()}))
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}
			err = encoder.Encode(rep)
			if err != nil {
				s.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error encoding API reply", map[string]interface{}{"error": err.Error()}))
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
			s.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error decoding publish params", map[string]interface{}{"error": err.Error()}))
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
			s.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error decoding broadcast params", map[string]interface{}{"error": err.Error()}))
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
			s.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error decoding subscribe params", map[string]interface{}{"error": err.Error()}))
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
			s.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error decoding unsubscribe params", map[string]interface{}{"error": err.Error()}))
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
			s.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error decoding disconnect params", map[string]interface{}{"error": err.Error()}))
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
			s.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error decoding presence params", map[string]interface{}{"error": err.Error()}))
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
			s.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error decoding presence stats params", map[string]interface{}{"error": err.Error()}))
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
			s.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error decoding history params", map[string]interface{}{"error": err.Error()}))
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
			s.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error decoding history remove params", map[string]interface{}{"error": err.Error()}))
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
			s.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error decoding rpc params", map[string]interface{}{"error": err.Error()}))
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
			s.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error decoding refresh params", map[string]interface{}{"error": err.Error()}))
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
			s.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error decoding channels params", map[string]interface{}{"error": err.Error()}))
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
		rep.Error = ErrorMethodNotFound
	}

	if replyRes != nil {
		rep.Result = replyRes
	}

	return rep, nil
}

func (s *Handler) handleReadDataError(r *http.Request, w http.ResponseWriter, err error) {
	s.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error reading API request body", map[string]interface{}{"error": err.Error(), "path": r.URL.Path}))
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}

func (s *Handler) handleUnmarshalError(r *http.Request, w http.ResponseWriter, err error) {
	s.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error decoding API request body", map[string]interface{}{"error": err.Error(), "path": r.URL.Path}))
	http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
}

func (s *Handler) handleMarshalError(r *http.Request, w http.ResponseWriter, err error) {
	s.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error encoding API response", map[string]interface{}{"error": err.Error(), "path": r.URL.Path}))
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}

func (s *Handler) writeJson(w http.ResponseWriter, data []byte) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}
