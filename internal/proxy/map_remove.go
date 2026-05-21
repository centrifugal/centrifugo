package proxy

import (
	"context"

	"github.com/centrifugal/centrifugo/v6/internal/proxyproto"
)

// MapRemoveProxy allows to send MapRemove requests.
type MapRemoveProxy interface {
	ProxyMapRemove(context.Context, *proxyproto.MapRemoveRequest) (*proxyproto.MapRemoveResponse, error)
	// Protocol for metrics and logging.
	Protocol() string
	// UseBase64 for bytes in requests from Centrifugo to application backend.
	UseBase64() bool
	// IncludeMeta ...
	IncludeMeta() bool
}
