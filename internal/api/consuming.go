package api

import (
	"context"
	"errors"

	"github.com/centrifugal/centrifugo/v5/internal/apiproto"

	"github.com/centrifugal/centrifuge"
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
	h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "non retryable error during consuming", map[string]any{"error": err.Error(), "method": method}))
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
