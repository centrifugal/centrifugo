package proxy

import (
	"context"

	"github.com/centrifugal/centrifugo/v6/internal/proxyproto"
)

// SubRefreshProxy allows to send sub refresh requests.
type SubRefreshProxy interface {
	ProxySubRefresh(context.Context, *proxyproto.SubRefreshRequest) (*proxyproto.SubRefreshResponse, error)
	// Protocol for metrics and logging.
	Protocol() string
	// UseBase64 for bytes in requests from Centrifugo to application backend.
	UseBase64() bool
	// IncludeMeta ...
	IncludeMeta() bool
}
