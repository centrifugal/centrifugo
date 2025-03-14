package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/centrifugal/centrifugo/v6/internal/apiproto"
	"github.com/centrifugal/centrifugo/v6/internal/configtypes"

	"github.com/centrifugal/centrifuge"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// ConsumingHandlerConfig configures ConsumingHandler.
type ConsumingHandlerConfig struct {
	UseOpenTelemetry bool
}

// ConsumingHandler is responsible for processing API commands from asynchronous consumers.
type ConsumingHandler struct {
	node   *centrifuge.Node
	config ConsumingHandlerConfig
	api    *Executor
}

// NewConsumingHandler creates new ConsumingHandler.
func NewConsumingHandler(n *centrifuge.Node, apiExecutor *Executor, c ConsumingHandlerConfig) *ConsumingHandler {
	h := &ConsumingHandler{
		node:   n,
		config: c,
		api:    apiExecutor,
	}
	return h
}

func (h *ConsumingHandler) logNonRetryableConsumingError(err error, method string) {
	log.Error().Err(err).Str("method", method).Msg("non retryable error during consuming, skip message")
}

func (h *ConsumingHandler) DispatchPublication(
	ctx context.Context, data []byte, idempotencyKey string, delta bool, channels ...string,
) error {
	if len(channels) == 0 {
		return nil
	}
	if len(channels) == 1 {
		req := &apiproto.PublishRequest{
			Data:           data,
			Channel:        channels[0],
			IdempotencyKey: idempotencyKey,
			Delta:          delta,
		}
		return h.Publish(ctx, req)
	}
	req := &apiproto.BroadcastRequest{
		Data:           data,
		Channels:       channels,
		IdempotencyKey: idempotencyKey,
		Delta:          delta,
	}
	return h.Broadcast(ctx, req)
}

func (h *ConsumingHandler) dispatchCommand(ctx context.Context, cmd *apiproto.Command) error {
	replies := make([]*apiproto.Reply, 1)
	method := h.api.processCmd(ctx, cmd, 0, replies)
	reply := replies[0]
	if cmd.Publish != nil {
		if reply.Error != nil && reply.Error.Code == apiproto.ErrorInternal.Code {
			return reply.Error
		}
	} else if cmd.Broadcast != nil {
		if reply.Error != nil && reply.Error.Code == apiproto.ErrorInternal.Code {
			return reply.Error
		}
		for _, response := range reply.Broadcast.Responses {
			if response.Error != nil && response.Error.Code == apiproto.ErrorInternal.Code {
				// Any internal error in any channel response will result into a retry by a consumer.
				// To prevent duplicate messages publishers may use idempotency keys.
				return response.Error
			}
		}
	}
	if reply.Error != nil {
		h.logNonRetryableConsumingError(reply.Error, method)
	}
	return nil
}

func (h *ConsumingHandler) Publish(ctx context.Context, req *apiproto.PublishRequest) error {
	resp := h.api.Publish(ctx, req)
	if h.config.UseOpenTelemetry && resp.Error != nil {
		span := trace.SpanFromContext(ctx)
		span.SetStatus(codes.Error, resp.Error.Error())
	}
	if resp.Error != nil && resp.Error.Code == apiproto.ErrorInternal.Code {
		return resp.Error
	}
	if resp.Error != nil {
		h.logNonRetryableConsumingError(resp.Error, "publish")
	}
	return nil
}

func (h *ConsumingHandler) Broadcast(ctx context.Context, req *apiproto.BroadcastRequest) error {
	resp := h.api.Broadcast(ctx, req)
	if h.config.UseOpenTelemetry && resp.Error != nil {
		span := trace.SpanFromContext(ctx)
		span.SetStatus(codes.Error, resp.Error.Error())
	}
	if resp.Error != nil && resp.Error.Code == apiproto.ErrorInternal.Code {
		return resp.Error
	}
	if resp.Error != nil {
		h.logNonRetryableConsumingError(resp.Error, "broadcast")
		return nil
	}
	for _, response := range resp.Result.Responses {
		if response.Error != nil && response.Error.Code == apiproto.ErrorInternal.Code {
			// Any internal error in any channel response will result into a retry by a consumer.
			// To prevent duplicate messages publishers may use idempotency keys.
			return response.Error
		}
	}
	return nil
}

func (h *ConsumingHandler) dispatchMethodPayload(ctx context.Context, method string, payload []byte) error {
	switch method {
	case "publish":
		_, err := h.handlePublish(ctx, payload)
		if err != nil {
			var apiError *apiproto.Error
			if errors.As(err, &apiError) && apiError.Code == apiproto.ErrorInternal.Code {
				return err
			}
			h.logNonRetryableConsumingError(err, method)
			return nil
		}
		return nil
	case "broadcast":
		// This one is special as we need to iterate over responses. Ideally we want to use
		// code gen for DispatchMethodPayload but need to make sure that broadcast logic is preserved.
		res, err := h.handleBroadcast(ctx, payload)
		if err != nil {
			var apiError *apiproto.Error
			if errors.As(err, &apiError) && apiError.Code == apiproto.ErrorInternal.Code {
				return err
			}
			h.logNonRetryableConsumingError(err, method)
			return nil
		}
		for _, resp := range res.Responses {
			if resp.Error != nil && resp.Error.Code == apiproto.ErrorInternal.Code {
				// Any internal error in any channel response will result into a retry by a consumer.
				// To prevent duplicate messages publishers may use idempotency keys.
				return resp.Error
			}
		}
		return nil
	case "subscribe":
		_, err := h.handleSubscribe(ctx, payload)
		if err != nil {
			var apiError *apiproto.Error
			if errors.As(err, &apiError) && apiError.Code == apiproto.ErrorInternal.Code {
				return err
			}
			h.logNonRetryableConsumingError(err, method)
			return nil
		}
		return nil
	case "unsubscribe":
		_, err := h.handleUnsubscribe(ctx, payload)
		if err != nil {
			var apiError *apiproto.Error
			if errors.As(err, &apiError) && apiError.Code == apiproto.ErrorInternal.Code {
				return err
			}
			h.logNonRetryableConsumingError(err, method)
			return nil
		}
		return nil
	case "disconnect":
		_, err := h.handleDisconnect(ctx, payload)
		if err != nil {
			var apiError *apiproto.Error
			if errors.As(err, &apiError) && apiError.Code == apiproto.ErrorInternal.Code {
				return err
			}
			h.logNonRetryableConsumingError(err, method)
			return nil
		}
		return nil
	case "history_remove":
		_, err := h.handleHistoryRemove(ctx, payload)
		if err != nil {
			var apiError *apiproto.Error
			if errors.As(err, &apiError) && apiError.Code == apiproto.ErrorInternal.Code {
				return err
			}
			h.logNonRetryableConsumingError(err, method)
			return nil
		}
		return nil
	case "refresh":
		_, err := h.handleRefresh(ctx, payload)
		if err != nil {
			var apiError *apiproto.Error
			if errors.As(err, &apiError) && apiError.Code == apiproto.ErrorInternal.Code {
				return err
			}
			h.logNonRetryableConsumingError(err, method)
			return nil
		}
		return nil
	default:
		// Ignore unsupported.
		return nil
	}
}

var ErrInvalidData = errors.New("invalid data")

// JSONRawOrString can decode payload from bytes and from JSON string. This gives
// us better interoperability. For example, JSONB field is encoded as JSON string in
// Debezium PostgreSQL connector.
type JSONRawOrString json.RawMessage

func (j *JSONRawOrString) UnmarshalJSON(data []byte) error {
	if len(data) > 0 && data[0] == '"' {
		// Unmarshal as a string, then convert the string to json.RawMessage.
		var str string
		if err := json.Unmarshal(data, &str); err != nil {
			return err
		}
		*j = JSONRawOrString(str)
	} else {
		// Unmarshal directly as json.RawMessage
		*j = data
	}
	return nil
}

// MarshalJSON returns m as the JSON encoding of m.
func (j JSONRawOrString) MarshalJSON() ([]byte, error) {
	if j == nil {
		return []byte("null"), nil
	}
	return j, nil
}

type MethodWithRequestPayload struct {
	Method  string          `json:"method"`
	Payload JSONRawOrString `json:"payload"`
}

func (h *ConsumingHandler) DispatchAPICommand(ctx context.Context, mode configtypes.ConsumerContentMode, method string, payload []byte) error {
	switch mode {
	case configtypes.ConsumerContentModeMethodPayload:
		if method != "" {
			// If method is set then we expect payload to be encoded request.
			return h.dispatchMethodPayload(ctx, method, payload)
		}
		var e MethodWithRequestPayload
		err := json.Unmarshal(payload, &e)
		if err != nil {
			log.Error().Err(err).Msg("skip malformed consumed message")
			return nil
		}
		return h.dispatchMethodPayload(ctx, e.Method, e.Payload)
	case configtypes.ConsumerContentModeAPICommand:
		var decoder apiproto.CommandDecoder
		if bytes.HasPrefix(payload, []byte(`{`)) {
			decoder = apiproto.GetCommandDecoder(payload)
			defer apiproto.PutCommandDecoder(decoder)
		} else {
			decoder = apiproto.GetProtobufCommandDecoder(payload)
			defer apiproto.PutProtobufCommandDecoder(decoder)
		}
		cmd, err := decoder.Decode()
		if err != nil {
			log.Error().Err(err).Msg("skip malformed consumed message")
			return nil
		}
		return h.dispatchCommand(ctx, cmd)
	default:
		return fmt.Errorf("unknown content mode: %v", mode)
	}
}
