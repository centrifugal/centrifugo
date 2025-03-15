package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/centrifugal/centrifugo/v6/internal/apiproto"

	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type LegacyCommand struct {
	Id     uint32            `json:"id,omitempty"`
	Method CommandMethodType `json:"method,omitempty"`
	Params apiproto.Raw      `json:"params,omitempty"`
}

type LegacyReply struct {
	Id     uint32          `json:"id,omitempty"`
	Error  *apiproto.Error `json:"error,omitempty"`
	Result apiproto.Raw    `json:"result,omitempty"`
}

type CommandMethodType int32

// UnmarshalJSON helps to unmarshal command method when set as string.
func (m *CommandMethodType) UnmarshalJSON(data []byte) error {
	if len(data) > 2 {
		if v, ok := Command_MethodType_value[strings.ToUpper(string(data[1:len(data)-1]))]; ok {
			*m = CommandMethodType(v)
			return nil
		}
	}
	val, err := strconv.Atoi(string(data))
	if err != nil {
		return err
	}
	*m = CommandMethodType(val)
	return nil
}

// GetParamsDecoder ...
func GetParamsDecoder() apiproto.RequestDecoder {
	return apiproto.NewJSONRequestDecoder()
}

// PutParamsDecoder ...
func PutParamsDecoder(_ apiproto.RequestDecoder) {}

// GetResultEncoder ...
func GetResultEncoder() apiproto.ResultEncoder {
	return apiproto.NewJSONResultEncoder()
}

// PutResultEncoder ...
func PutResultEncoder(_ apiproto.ResultEncoder) {}

var (
	jsonReplyEncoderPool sync.Pool
)

// ReplyEncoder ...
type ReplyEncoder interface {
	Reset()
	Encode(reply *LegacyReply) error
	Finish() []byte
}

var _ ReplyEncoder = (*JSONReplyEncoder)(nil)

// JSONReplyEncoder ...
type JSONReplyEncoder struct {
	count  int
	buffer bytes.Buffer
}

// NewJSONReplyEncoder ...
func NewJSONReplyEncoder() *JSONReplyEncoder {
	return &JSONReplyEncoder{}
}

// Reset ...
func (e *JSONReplyEncoder) Reset() {
	e.count = 0
	e.buffer.Reset()
}

// Encode ...
func (e *JSONReplyEncoder) Encode(r *LegacyReply) error {
	if e.count > 0 {
		e.buffer.WriteString("\n")
	}
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}
	e.buffer.Write(data)
	e.count++
	return nil
}

// Finish ...
func (e *JSONReplyEncoder) Finish() []byte {
	data := e.buffer.Bytes()
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)
	return dataCopy
}

// GetReplyEncoder ...
func GetReplyEncoder() ReplyEncoder {
	e := jsonReplyEncoderPool.Get()
	if e == nil {
		return NewJSONReplyEncoder()
	}
	return e.(ReplyEncoder)
}

// PutReplyEncoder ...
func PutReplyEncoder(e ReplyEncoder) {
	e.Reset()
	jsonReplyEncoderPool.Put(e)
}

// CommandDecoder ...
type CommandDecoder interface {
	Reset([]byte)
	Decode() (*LegacyCommand, error)
}

var jsonCommandDecoderPool sync.Pool

var _ CommandDecoder = (*JSONCommandDecoder)(nil)

// GetCommandDecoder ...
func GetCommandDecoder(data []byte) CommandDecoder {
	e := jsonCommandDecoderPool.Get()
	if e == nil {
		return NewJSONCommandDecoder(data)
	}
	decoder := e.(CommandDecoder)
	decoder.Reset(data)
	return decoder
}

// PutCommandDecoder ...
func PutCommandDecoder(d CommandDecoder) {
	jsonCommandDecoderPool.Put(d)
}

// JSONCommandDecoder ...
type JSONCommandDecoder struct {
	decoder *json.Decoder
}

// NewJSONCommandDecoder ...
func NewJSONCommandDecoder(data []byte) *JSONCommandDecoder {
	return &JSONCommandDecoder{
		decoder: json.NewDecoder(bytes.NewReader(data)),
	}
}

// Reset ...
func (d *JSONCommandDecoder) Reset(data []byte) {
	d.decoder = json.NewDecoder(bytes.NewReader(data))
}

// Decode ...
func (d *JSONCommandDecoder) Decode() (*LegacyCommand, error) {
	var c LegacyCommand
	err := d.decoder.Decode(&c)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// OldRoute handles all methods inside one /api handler.
// The plan is to remove it in Centrifugo v7.
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

const (
	Command_PUBLISH                CommandMethodType = 0
	Command_BROADCAST              CommandMethodType = 1
	Command_UNSUBSCRIBE            CommandMethodType = 2
	Command_DISCONNECT             CommandMethodType = 3
	Command_PRESENCE               CommandMethodType = 4
	Command_PRESENCE_STATS         CommandMethodType = 5
	Command_HISTORY                CommandMethodType = 6
	Command_HISTORY_REMOVE         CommandMethodType = 7
	Command_CHANNELS               CommandMethodType = 8
	Command_INFO                   CommandMethodType = 9
	Command_RPC                    CommandMethodType = 10
	Command_SUBSCRIBE              CommandMethodType = 11
	Command_REFRESH                CommandMethodType = 12
	Command_CONNECTIONS            CommandMethodType = 14
	Command_UPDATE_USER_STATUS     CommandMethodType = 15
	Command_GET_USER_STATUS        CommandMethodType = 16
	Command_DELETE_USER_STATUS     CommandMethodType = 17
	Command_BLOCK_USER             CommandMethodType = 18
	Command_UNBLOCK_USER           CommandMethodType = 19
	Command_REVOKE_TOKEN           CommandMethodType = 20
	Command_INVALIDATE_USER_TOKENS CommandMethodType = 21
	Command_DEVICE_REGISTER        CommandMethodType = 22
	Command_DEVICE_UPDATE          CommandMethodType = 23
	Command_DEVICE_REMOVE          CommandMethodType = 24
	Command_DEVICE_LIST            CommandMethodType = 25
	Command_DEVICE_TOPIC_LIST      CommandMethodType = 26
	Command_DEVICE_TOPIC_UPDATE    CommandMethodType = 27
	Command_USER_TOPIC_LIST        CommandMethodType = 28
	Command_USER_TOPIC_UPDATE      CommandMethodType = 29
	Command_SEND_PUSH_NOTIFICATION CommandMethodType = 30
	Command_UPDATE_PUSH_STATUS     CommandMethodType = 31
	Command_CANCEL_PUSH            CommandMethodType = 32
)

var Command_MethodType_value = map[string]int32{
	"PUBLISH":                0,
	"BROADCAST":              1,
	"UNSUBSCRIBE":            2,
	"DISCONNECT":             3,
	"PRESENCE":               4,
	"PRESENCE_STATS":         5,
	"HISTORY":                6,
	"HISTORY_REMOVE":         7,
	"CHANNELS":               8,
	"INFO":                   9,
	"RPC":                    10,
	"SUBSCRIBE":              11,
	"REFRESH":                12,
	"CONNECTIONS":            14,
	"UPDATE_USER_STATUS":     15,
	"GET_USER_STATUS":        16,
	"DELETE_USER_STATUS":     17,
	"BLOCK_USER":             18,
	"UNBLOCK_USER":           19,
	"REVOKE_TOKEN":           20,
	"INVALIDATE_USER_TOKENS": 21,
	"DEVICE_REGISTER":        22,
	"DEVICE_UPDATE":          23,
	"DEVICE_REMOVE":          24,
	"DEVICE_LIST":            25,
	"DEVICE_TOPIC_LIST":      26,
	"DEVICE_TOPIC_UPDATE":    27,
	"USER_TOPIC_LIST":        28,
	"USER_TOPIC_UPDATE":      29,
	"SEND_PUSH_NOTIFICATION": 30,
	"UPDATE_PUSH_STATUS":     31,
	"CANCEL_PUSH":            32,
}

var (
	Command_MethodType_name = map[int32]string{
		0:  "PUBLISH",
		1:  "BROADCAST",
		2:  "UNSUBSCRIBE",
		3:  "DISCONNECT",
		4:  "PRESENCE",
		5:  "PRESENCE_STATS",
		6:  "HISTORY",
		7:  "HISTORY_REMOVE",
		8:  "CHANNELS",
		9:  "INFO",
		10: "RPC",
		11: "SUBSCRIBE",
		12: "REFRESH",
		14: "CONNECTIONS",
		15: "UPDATE_USER_STATUS",
		16: "GET_USER_STATUS",
		17: "DELETE_USER_STATUS",
		18: "BLOCK_USER",
		19: "UNBLOCK_USER",
		20: "REVOKE_TOKEN",
		21: "INVALIDATE_USER_TOKENS",
		22: "DEVICE_REGISTER",
		23: "DEVICE_UPDATE",
		24: "DEVICE_REMOVE",
		25: "DEVICE_LIST",
		26: "DEVICE_TOPIC_LIST",
		27: "DEVICE_TOPIC_UPDATE",
		28: "USER_TOPIC_LIST",
		29: "USER_TOPIC_UPDATE",
		30: "SEND_PUSH_NOTIFICATION",
		31: "UPDATE_PUSH_STATUS",
		32: "CANCEL_PUSH",
	}
)

func (s *Handler) handleAPICommand(ctx context.Context, cmd *LegacyCommand) (*LegacyReply, error) {

	method := cmd.Method
	params := cmd.Params

	rep := &LegacyReply{
		Id: cmd.Id,
	}

	var replyRes apiproto.Raw

	decoder := GetParamsDecoder()
	defer PutParamsDecoder(decoder)

	encoder := GetResultEncoder()
	defer PutResultEncoder(encoder)

	switch method {
	case Command_PUBLISH:
		cmd, err := decoder.DecodePublish(params)
		if err != nil {
			log.Error().Err(err).Msg("error decoding publish params")
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
	case Command_BROADCAST:
		cmd, err := decoder.DecodeBroadcast(params)
		if err != nil {
			log.Error().Err(err).Msg("error decoding broadcast params")
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
	case Command_SUBSCRIBE:
		cmd, err := decoder.DecodeSubscribe(params)
		if err != nil {
			log.Error().Err(err).Msg("error decoding subscribe params")
			rep.Error = apiproto.ErrorBadRequest
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
	case Command_DISCONNECT:
		cmd, err := decoder.DecodeDisconnect(params)
		if err != nil {
			log.Error().Err(err).Msg("error decoding disconnect params")
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
	case Command_PRESENCE:
		cmd, err := decoder.DecodePresence(params)
		if err != nil {
			log.Error().Err(err).Msg("error decoding presence params")
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
	case Command_PRESENCE_STATS:
		cmd, err := decoder.DecodePresenceStats(params)
		if err != nil {
			log.Error().Err(err).Msg("error decoding presence stats params")
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
	case Command_HISTORY:
		cmd, err := decoder.DecodeHistory(params)
		if err != nil {
			log.Error().Err(err).Msg("error decoding history params")
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
	case Command_HISTORY_REMOVE:
		cmd, err := decoder.DecodeHistoryRemove(params)
		if err != nil {
			log.Error().Err(err).Msg("error decoding history remove params")
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
	case Command_INFO:
		resp := s.api.Info(ctx, &apiproto.InfoRequest{})
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
			rep.Error = apiproto.ErrorBadRequest
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
			rep.Error = apiproto.ErrorBadRequest
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
			rep.Error = apiproto.ErrorBadRequest
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
		rep.Error = apiproto.ErrorNotFound
	}

	if replyRes != nil {
		rep.Result = replyRes
	}

	return rep, nil
}
