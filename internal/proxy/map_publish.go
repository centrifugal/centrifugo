package proxy

import (
	"context"

	"github.com/centrifugal/centrifugo/v6/internal/proxyproto"
)

// MapPublishProxy allows to send MapPublish requests.
type MapPublishProxy interface {
	ProxyMapPublish(context.Context, *proxyproto.MapPublishRequest) (*proxyproto.MapPublishResponse, error)
	// Protocol for metrics and logging.
	Protocol() string
	// UseBase64 for bytes in requests from Centrifugo to application backend.
	UseBase64() bool
	// IncludeMeta ...
	IncludeMeta() bool
}
