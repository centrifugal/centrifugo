package proxy

import (
	"context"

	"github.com/centrifugal/centrifugo/v5/internal/proxyproto"
)

// CacheEmptyProxy allows loading cached data from a source of truth.
type CacheEmptyProxy interface {
	NotifyCacheEmpty(context.Context, *proxyproto.NotifyCacheEmptyRequest) (*proxyproto.NotifyCacheEmptyResponse, error)
	// Protocol for metrics and logging.
	Protocol() string
}
