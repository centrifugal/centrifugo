package api

import (
	"context"
	"io"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/centrifugal/centrifuge"
)

// Config configures APIHandler.
type Config struct{}

// Handler is responsible for processing API commands over HTTP.
type Handler struct {
	node   *centrifuge.Node
	config Config
	api    *apiExecutor
}

// NewHandler creates new APIHandler.
func NewHandler(n *centrifuge.Node, c Config) *Handler {
	return &Handler{
		node:   n,
		config: c,
		api:    newAPIExecutor(n, "http"),
	}
}

func (s *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	select {
	case <-s.node.NotifyShutdown():
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	default:
	}

	var data []byte
	var err error

	data, err = ioutil.ReadAll(r.Body)
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

	var enc Encoding

	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(strings.ToLower(contentType), "application/octet-stream") {
		enc = EncodingProtobuf
	} else {
		enc = EncodingJSON
	}

	encoder := GetReplyEncoder(enc)
	defer PutReplyEncoder(enc, encoder)

	decoder := GetCommandDecoder(enc, data)
	defer PutCommandDecoder(enc, decoder)

	for {
		command, err := decoder.Decode()
		if err != nil {
			if err == io.EOF {
				break
			}
			s.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error decoding API data", map[string]interface{}{"error": err.Error()}))
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}
		rep, err := s.handleAPICommand(r.Context(), enc, command)
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
	resp := encoder.Finish()
	w.Header().Set("Content-Type", contentType)
	w.Write(resp)
}

func (s *Handler) handleAPICommand(ctx context.Context, enc Encoding, cmd *Command) (*Reply, error) {

	method := cmd.Method
	params := cmd.Params

	rep := &Reply{
		ID: cmd.ID,
	}

	var replyRes Raw

	decoder := GetDecoder(enc)
	defer PutDecoder(enc, decoder)

	encoder := GetEncoder(enc)
	defer PutEncoder(enc, encoder)

	switch method {
	case MethodTypePublish:
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
	case MethodTypeBroadcast:
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
	case MethodTypeUnsubscribe:
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
	case MethodTypeDisconnect:
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
	case MethodTypePresence:
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
	case MethodTypePresenceStats:
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
	case MethodTypeHistory:
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
	case MethodTypeHistoryRemove:
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
	case MethodTypeChannels:
		resp := s.api.Channels(ctx, &ChannelsRequest{})
		if resp.Error != nil {
			rep.Error = resp.Error
		} else {
			if resp.Result != nil {
				var err error
				replyRes, err = encoder.EncodeChannels(resp.Result)
				if err != nil {
					return nil, err
				}
			}
		}
	case MethodTypeInfo:
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
	default:
		rep.Error = ErrorMethodNotFound
	}

	if replyRes != nil {
		rep.Result = replyRes
	}

	return rep, nil
}
