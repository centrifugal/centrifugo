package proxy

import (
	"context"

	"github.com/centrifugal/centrifugo/v6/internal/proxyproto"
)

// SubscribeProxy allows to send Subscribe requests.
type SubscribeProxy interface {
	ProxySubscribe(context.Context, *proxyproto.SubscribeRequest) (*proxyproto.SubscribeResponse, error)
	// Protocol for metrics and logging.
	Protocol() string
	// UseBase64 for bytes in requests from Centrifugo to application backend.
	UseBase64() bool
	// IncludeMeta ...
	IncludeMeta() bool
}
