package api

import (
	"context"
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

func (h *ConsumingHandler) logNonRetryableConsumingError(err error, method string) {
	log.Error().Err(err).Str("method", method).Msg("non retryable error during consuming, skip message")
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

// Dispatch processes commands received from asynchronous consumers.
func (h *ConsumingHandler) Dispatch(ctx context.Context, method string, payload []byte) error {
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
		// code gen for Dispatch but need to make sure that broadcast logic is preserved.
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
