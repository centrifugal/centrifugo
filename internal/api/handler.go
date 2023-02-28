package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	. "github.com/centrifugal/centrifugo/v4/internal/apiproto"

	"github.com/centrifugal/centrifuge"
)

// Config configures APIHandler.
type Config struct {
}

// Handler is responsible for processing API commands over HTTP.
type Handler struct {
	mux    *http.ServeMux
	node   *centrifuge.Node
	config Config
	api    *Executor
}

// NewHandler creates new APIHandler.
func NewHandler(n *centrifuge.Node, apiExecutor *Executor, c Config) *Handler {
	m := new(http.ServeMux)
	h := &Handler{
		mux:    m,
		node:   n,
		config: c,
		api:    apiExecutor,
	}

	m.HandleFunc("/publish", h.handlePublish)
	m.HandleFunc("/broadcast", h.handleBroadcast)
	m.HandleFunc("/subscribe", h.handleSubscribe)
	m.HandleFunc("/unsubscribe", h.handleUnsubscribe)
	m.HandleFunc("/disconnect", h.handleDisconnect)
	m.HandleFunc("/presence", h.handlePresence)
	m.HandleFunc("/presence_stats", h.handlePresenceStats)
	m.HandleFunc("/history", h.handleHistory)
	m.HandleFunc("/history_remove", h.handleHistoryRemove)
	m.HandleFunc("/info", h.handleInfo)
	m.HandleFunc("/rpc", h.handleRPC)
	m.HandleFunc("/refresh", h.handleRefresh)
	m.HandleFunc("/channels", h.handleChannels)
	return h
}

func (s *Handler) OldRoute() http.HandlerFunc {
	return s.handleAPI
}

func (s *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Handler) handleChannels(w http.ResponseWriter, r *http.Request) {
	req := new(ChannelsRequest)

	data, err := io.ReadAll(r.Body)
	if err != nil {
		s.handleReadDataErr(w, err)
		return
	}

	if err = json.Unmarshal(data, req); err != nil {
		s.handleUnmarshalError(w, err)
		return
	}

	res := s.api.Channels(r.Context(), req)
	data, err = json.Marshal(res)
	if err != nil {
		s.handleMarshalError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}

func (s *Handler) handleRefresh(w http.ResponseWriter, r *http.Request) {
	req := new(RefreshRequest)

	data, err := io.ReadAll(r.Body)
	if err != nil {
		s.handleReadDataErr(w, err)
		return
	}

	if err = json.Unmarshal(data, req); err != nil {
		s.handleUnmarshalError(w, err)
		return
	}

	res := s.api.Refresh(r.Context(), req)
	data, err = json.Marshal(res)
	if err != nil {
		s.handleMarshalError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}

func (s *Handler) handleRPC(w http.ResponseWriter, r *http.Request) {
	req := new(RPCRequest)

	data, err := io.ReadAll(r.Body)
	if err != nil {
		s.handleReadDataErr(w, err)
		return
	}

	if err = json.Unmarshal(data, req); err != nil {
		s.handleUnmarshalError(w, err)
		return
	}

	res := s.api.RPC(r.Context(), req)
	data, err = json.Marshal(res)
	if err != nil {
		s.handleMarshalError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}

func (s *Handler) handleInfo(w http.ResponseWriter, r *http.Request) {
	req := new(InfoRequest)

	data, err := io.ReadAll(r.Body)
	if err != nil {
		s.handleReadDataErr(w, err)
		return
	}

	if err = json.Unmarshal(data, req); err != nil {
		s.handleUnmarshalError(w, err)
		return
	}

	res := s.api.Info(r.Context(), req)
	data, err = json.Marshal(res)
	if err != nil {
		s.handleMarshalError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}

func (s *Handler) handleHistoryRemove(w http.ResponseWriter, r *http.Request) {
	req := new(HistoryRemoveRequest)

	data, err := io.ReadAll(r.Body)
	if err != nil {
		s.handleReadDataErr(w, err)
		return
	}

	if err = json.Unmarshal(data, req); err != nil {
		s.handleUnmarshalError(w, err)
		return
	}

	res := s.api.HistoryRemove(r.Context(), req)
	data, err = json.Marshal(res)
	if err != nil {
		s.handleMarshalError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}

func (s *Handler) handleHistory(w http.ResponseWriter, r *http.Request) {
	req := new(HistoryRequest)

	data, err := io.ReadAll(r.Body)
	if err != nil {
		s.handleReadDataErr(w, err)
		return
	}

	if err = json.Unmarshal(data, req); err != nil {
		s.handleUnmarshalError(w, err)
		return
	}

	res := s.api.History(r.Context(), req)
	data, err = json.Marshal(res)
	if err != nil {
		s.handleMarshalError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}

func (s *Handler) handlePresenceStats(w http.ResponseWriter, r *http.Request) {
	req := new(PresenceStatsRequest)

	data, err := io.ReadAll(r.Body)
	if err != nil {
		s.handleReadDataErr(w, err)
		return
	}

	if err = json.Unmarshal(data, req); err != nil {
		s.handleUnmarshalError(w, err)
		return
	}

	res := s.api.PresenceStats(r.Context(), req)
	data, err = json.Marshal(res)
	if err != nil {
		s.handleMarshalError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}

func (s *Handler) handlePresence(w http.ResponseWriter, r *http.Request) {
	req := new(PresenceRequest)

	data, err := io.ReadAll(r.Body)
	if err != nil {
		s.handleReadDataErr(w, err)
		return
	}

	if err = json.Unmarshal(data, req); err != nil {
		s.handleUnmarshalError(w, err)
		return
	}

	res := s.api.Presence(r.Context(), req)
	data, err = json.Marshal(res)
	if err != nil {
		s.handleMarshalError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}

func (s *Handler) handleDisconnect(w http.ResponseWriter, r *http.Request) {
	req := new(DisconnectRequest)

	data, err := io.ReadAll(r.Body)
	if err != nil {
		s.handleReadDataErr(w, err)
		return
	}

	if err = json.Unmarshal(data, req); err != nil {
		s.handleUnmarshalError(w, err)
		return
	}

	res := s.api.Disconnect(r.Context(), req)
	data, err = json.Marshal(res)
	if err != nil {
		s.handleMarshalError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}

func (s *Handler) handleUnsubscribe(w http.ResponseWriter, r *http.Request) {
	req := new(UnsubscribeRequest)

	data, err := io.ReadAll(r.Body)
	if err != nil {
		s.handleReadDataErr(w, err)
		return
	}

	if err = json.Unmarshal(data, req); err != nil {
		s.handleUnmarshalError(w, err)
		return
	}

	res := s.api.Unsubscribe(r.Context(), req)
	data, err = json.Marshal(res)
	if err != nil {
		s.handleMarshalError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}

func (s *Handler) handleSubscribe(w http.ResponseWriter, r *http.Request) {
	req := new(SubscribeRequest)

	data, err := io.ReadAll(r.Body)
	if err != nil {
		s.handleReadDataErr(w, err)
		return
	}

	if err = json.Unmarshal(data, req); err != nil {
		s.handleUnmarshalError(w, err)
		return
	}

	res := s.api.Subscribe(r.Context(), req)
	data, err = json.Marshal(res)
	if err != nil {
		s.handleMarshalError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}

func (s *Handler) handleBroadcast(w http.ResponseWriter, r *http.Request) {
	req := new(BroadcastRequest)

	data, err := io.ReadAll(r.Body)
	if err != nil {
		s.handleReadDataErr(w, err)
		return
	}

	if err = json.Unmarshal(data, req); err != nil {
		s.handleUnmarshalError(w, err)
		return
	}

	res := s.api.Broadcast(r.Context(), req)
	data, err = json.Marshal(res)
	if err != nil {
		s.handleMarshalError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}

func (s *Handler) handlePublish(w http.ResponseWriter, r *http.Request) {
	req := new(PublishRequest)

	data, err := io.ReadAll(r.Body)
	if err != nil {
		s.handleReadDataErr(w, err)
		return
	}

	if err = json.Unmarshal(data, req); err != nil {
		s.handleUnmarshalError(w, err)
		return
	}

	res := s.api.Publish(r.Context(), req)
	data, err = json.Marshal(res)
	if err != nil {
		s.handleMarshalError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
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
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(encoder.Finish())
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

func (s *Handler) handleReadDataErr(w http.ResponseWriter, err error) {
	s.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error reading API request body", map[string]interface{}{"error": err.Error()}))
	http.Error(w, "Internal Server Error", http.StatusInternalServerError)
}

func (s *Handler) handleUnmarshalError(w http.ResponseWriter, err error) {
	s.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error decoding API data", map[string]interface{}{"error": err.Error()}))
	http.Error(w, "Bad Request", http.StatusBadRequest)
}

func (s *Handler) handleMarshalError(w http.ResponseWriter, err error) {
	s.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error encoding API reply", map[string]interface{}{"error": err.Error()}))
	http.Error(w, "Internal Server Error", http.StatusInternalServerError)
}
