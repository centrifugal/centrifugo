package centrifuge

import (
	"context"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/centrifugal/centrifuge/internal/proto"
	"github.com/centrifugal/centrifuge/internal/proto/apiproto"
)

// APIConfig ...
type APIConfig struct{}

// APIHandler is responsible for processing API commands over HTTP.
type APIHandler struct {
	node   *Node
	config APIConfig
	api    *apiExecutor
}

// NewAPIHandler creates new APIHandler.
func NewAPIHandler(n *Node, c APIConfig) *APIHandler {
	return &APIHandler{
		node:   n,
		config: c,
		api:    newAPIExecutor(n),
	}
}

func (s *APIHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	defer func(started time.Time) {
		apiHandlerDurationSummary.Observe(float64(time.Since(started).Seconds()))
	}(time.Now())

	var data []byte
	var err error

	data, err = ioutil.ReadAll(r.Body)
	if err != nil {
		s.node.logger.log(newLogEntry(LogLevelError, "error reading API request body", map[string]interface{}{"error": err.Error()}))
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if len(data) == 0 {
		s.node.logger.log(newLogEntry(LogLevelError, "no data in API request"))
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	var enc apiproto.Encoding

	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(strings.ToLower(contentType), "application/octet-stream") {
		enc = apiproto.EncodingProtobuf
	} else {
		enc = apiproto.EncodingJSON
	}

	encoder := apiproto.GetReplyEncoder(enc)
	defer apiproto.PutReplyEncoder(enc, encoder)

	decoder := apiproto.GetCommandDecoder(enc, data)
	defer apiproto.PutCommandDecoder(enc, decoder)

	for {
		command, err := decoder.Decode()
		if err != nil {
			if err == io.EOF {
				break
			}
			s.node.logger.log(newLogEntry(LogLevelError, "error decoding API data", map[string]interface{}{"error": err.Error()}))
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}
		now := time.Now()
		rep, err := s.handleAPICommand(r.Context(), enc, command)
		apiCommandDurationSummary.WithLabelValues(strings.ToLower(apiproto.MethodType_name[int32(command.Method)])).Observe(float64(time.Since(now).Seconds()))
		if err != nil {
			s.node.logger.log(newLogEntry(LogLevelError, "error handling API command", map[string]interface{}{"error": err.Error()}))
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		err = encoder.Encode(rep)
		if err != nil {
			s.node.logger.log(newLogEntry(LogLevelError, "error encoding API reply", map[string]interface{}{"error": err.Error()}))
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
	}
	resp := encoder.Finish()
	w.Header().Set("Content-Type", contentType)
	w.Write(resp)
}

func (s *APIHandler) handleAPICommand(ctx context.Context, enc apiproto.Encoding, cmd *apiproto.Command) (*apiproto.Reply, error) {
	var err error

	method := cmd.Method
	params := cmd.Params

	rep := &apiproto.Reply{
		ID: cmd.ID,
	}

	var replyRes proto.Raw

	decoder := apiproto.GetDecoder(enc)
	defer apiproto.PutDecoder(enc, decoder)

	encoder := apiproto.GetEncoder(enc)
	defer apiproto.PutEncoder(enc, encoder)

	switch method {
	case apiproto.MethodTypePublish:
		cmd, err := decoder.DecodePublish(params)
		if err != nil {
			s.node.logger.log(newLogEntry(LogLevelError, "error decoding publish params", map[string]interface{}{"error": err.Error()}))
			rep.Error = apiproto.ErrorBadRequest
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
	case apiproto.MethodTypeBroadcast:
		cmd, err := decoder.DecodeBroadcast(params)
		if err != nil {
			s.node.logger.log(newLogEntry(LogLevelError, "error decoding broadcast params", map[string]interface{}{"error": err.Error()}))
			rep.Error = apiproto.ErrorBadRequest
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
	case apiproto.MethodTypeUnsubscribe:
		cmd, err := decoder.DecodeUnsubscribe(params)
		if err != nil {
			s.node.logger.log(newLogEntry(LogLevelError, "error decoding unsubscribe params", map[string]interface{}{"error": err.Error()}))
			rep.Error = apiproto.ErrorBadRequest
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
	case apiproto.MethodTypeDisconnect:
		cmd, err := decoder.DecodeDisconnect(params)
		if err != nil {
			s.node.logger.log(newLogEntry(LogLevelError, "error decoding disconnect params", map[string]interface{}{"error": err.Error()}))
			rep.Error = apiproto.ErrorBadRequest
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
	case apiproto.MethodTypePresence:
		cmd, err := decoder.DecodePresence(params)
		if err != nil {
			s.node.logger.log(newLogEntry(LogLevelError, "error decoding presence params", map[string]interface{}{"error": err.Error()}))
			rep.Error = apiproto.ErrorBadRequest
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
	case apiproto.MethodTypePresenceStats:
		cmd, err := decoder.DecodePresenceStats(params)
		if err != nil {
			s.node.logger.log(newLogEntry(LogLevelError, "error decoding presence stats params", map[string]interface{}{"error": err.Error()}))
			rep.Error = apiproto.ErrorBadRequest
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
	case apiproto.MethodTypeHistory:
		cmd, err := decoder.DecodeHistory(params)
		if err != nil {
			s.node.logger.log(newLogEntry(LogLevelError, "error decoding history params", map[string]interface{}{"error": err.Error()}))
			rep.Error = apiproto.ErrorBadRequest
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
	case apiproto.MethodTypeHistoryRemove:
		cmd, err := decoder.DecodeHistoryRemove(params)
		if err != nil {
			s.node.logger.log(newLogEntry(LogLevelError, "error decoding history remove params", map[string]interface{}{"error": err.Error()}))
			rep.Error = apiproto.ErrorBadRequest
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
	case apiproto.MethodTypeChannels:
		resp := s.api.Channels(ctx, &apiproto.ChannelsRequest{})
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
	case apiproto.MethodTypeInfo:
		resp := s.api.Info(ctx, &apiproto.InfoRequest{})
		if resp.Error != nil {
			rep.Error = resp.Error
		} else {
			if resp.Result != nil {
				replyRes, err = encoder.EncodeInfo(resp.Result)
				if err != nil {
					return nil, err
				}
			}
		}
	default:
		rep.Error = apiproto.ErrorMethodNotFound
	}

	if replyRes != nil {
		rep.Result = replyRes
	}

	return rep, nil
}
