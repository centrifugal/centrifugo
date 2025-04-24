package api

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/centrifugal/centrifugo/v6/internal/apiproto"

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

func logNonRetryableConsumingError(err error, method string) {
	log.Error().Err(err).Str("method", method).Msg("non retryable error during consuming, skip message")
}

type Dispatcher struct {
	handler *ConsumingHandler
}

func NewDispatcher(handler *ConsumingHandler) *Dispatcher {
	return &Dispatcher{
		handler: handler,
	}
}

func (d *Dispatcher) Publish(ctx context.Context, req *apiproto.PublishRequest) error {
	resp := d.handler.api.Publish(ctx, req)
	if d.handler.config.UseOpenTelemetry && resp.Error != nil {
		span := trace.SpanFromContext(ctx)
		span.SetStatus(codes.Error, resp.Error.Error())
	}
	if resp.Error != nil && resp.Error.Code == apiproto.ErrorInternal.Code {
		return resp.Error
	}
	if resp.Error != nil {
		logNonRetryableConsumingError(resp.Error, "publish")
	}
	return nil
}

func (d *Dispatcher) Broadcast(ctx context.Context, req *apiproto.BroadcastRequest) error {
	resp := d.handler.api.Broadcast(ctx, req)
	if d.handler.config.UseOpenTelemetry && resp.Error != nil {
		span := trace.SpanFromContext(ctx)
		span.SetStatus(codes.Error, resp.Error.Error())
	}
	if resp.Error != nil && resp.Error.Code == apiproto.ErrorInternal.Code {
		return resp.Error
	}
	if resp.Error != nil {
		logNonRetryableConsumingError(resp.Error, "broadcast")
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

type ConsumedPublication struct {
	Data []byte
	// IdempotencyKey is used to prevent duplicate messages.
	IdempotencyKey string
	// Delta is used to indicate that the message is a delta update.
	Delta bool
	// Tags are used to attach metadata to the message.
	Tags map[string]string
	// Version of publication.
	Version uint64
	// VersionEpoch of publication.
	VersionEpoch string
}

func (d *Dispatcher) DispatchPublication(
	ctx context.Context, channels []string, pub ConsumedPublication,
) error {
	if len(channels) == 0 {
		return nil
	}
	if len(channels) == 1 {
		req := &apiproto.PublishRequest{
			Data:           pub.Data,
			Channel:        channels[0],
			IdempotencyKey: pub.IdempotencyKey,
			Delta:          pub.Delta,
			Tags:           pub.Tags,
			Version:        pub.Version,
			VersionEpoch:   pub.VersionEpoch,
		}
		return d.Publish(ctx, req)
	}
	req := &apiproto.BroadcastRequest{
		Data:           pub.Data,
		Channels:       channels,
		IdempotencyKey: pub.IdempotencyKey,
		Delta:          pub.Delta,
		Tags:           pub.Tags,
		Version:        pub.Version,
		VersionEpoch:   pub.VersionEpoch,
	}
	return d.Broadcast(ctx, req)
}

// Dispatch processes commands received from asynchronous consumers.
func (d *Dispatcher) dispatchMethodPayload(ctx context.Context, method string, payload []byte) error {
	switch method {
	case "publish":
		_, err := d.handler.handlePublish(ctx, payload)
		if err != nil {
			var apiError *apiproto.Error
			if errors.As(err, &apiError) && apiError.Code == apiproto.ErrorInternal.Code {
				return err
			}
			logNonRetryableConsumingError(err, method)
			return nil
		}
		return nil
	case "broadcast":
		// This one is special as we need to iterate over responses. Ideally we want to use
		// code gen for Dispatch but need to make sure that broadcast logic is preserved.
		res, err := d.handler.handleBroadcast(ctx, payload)
		if err != nil {
			var apiError *apiproto.Error
			if errors.As(err, &apiError) && apiError.Code == apiproto.ErrorInternal.Code {
				return err
			}
			logNonRetryableConsumingError(err, method)
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
		_, err := d.handler.handleSubscribe(ctx, payload)
		if err != nil {
			var apiError *apiproto.Error
			if errors.As(err, &apiError) && apiError.Code == apiproto.ErrorInternal.Code {
				return err
			}
			logNonRetryableConsumingError(err, method)
			return nil
		}
		return nil
	case "unsubscribe":
		_, err := d.handler.handleUnsubscribe(ctx, payload)
		if err != nil {
			var apiError *apiproto.Error
			if errors.As(err, &apiError) && apiError.Code == apiproto.ErrorInternal.Code {
				return err
			}
			logNonRetryableConsumingError(err, method)
			return nil
		}
		return nil
	case "disconnect":
		_, err := d.handler.handleDisconnect(ctx, payload)
		if err != nil {
			var apiError *apiproto.Error
			if errors.As(err, &apiError) && apiError.Code == apiproto.ErrorInternal.Code {
				return err
			}
			logNonRetryableConsumingError(err, method)
			return nil
		}
		return nil
	case "history_remove":
		_, err := d.handler.handleHistoryRemove(ctx, payload)
		if err != nil {
			var apiError *apiproto.Error
			if errors.As(err, &apiError) && apiError.Code == apiproto.ErrorInternal.Code {
				return err
			}
			logNonRetryableConsumingError(err, method)
			return nil
		}
		return nil
	case "refresh":
		_, err := d.handler.handleRefresh(ctx, payload)
		if err != nil {
			var apiError *apiproto.Error
			if errors.As(err, &apiError) && apiError.Code == apiproto.ErrorInternal.Code {
				return err
			}
			logNonRetryableConsumingError(err, method)
			return nil
		}
		return nil
	default:
		// Skip unsupported.
		log.Info().Msg("skip unsupported API command method")
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

// DispatchCommand processes commands received from asynchronous consumers.
func (d *Dispatcher) DispatchCommand(ctx context.Context, method string, payload []byte) error {
	if method != "" {
		// If method already known – we can skip decoding into MethodWithRequestPayload.
		return d.dispatchMethodPayload(ctx, method, payload)
	}
	// Otherwise we expect payload to be MethodWithRequestPayload.
	var e MethodWithRequestPayload
	err := json.Unmarshal(payload, &e)
	if err != nil {
		log.Error().Err(err).Msg("skip malformed consumed message")
		return nil
	}
	return d.dispatchMethodPayload(ctx, e.Method, e.Payload)
}
