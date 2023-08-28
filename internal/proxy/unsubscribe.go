package proxy

import (
	"context"

	"github.com/centrifugal/centrifugo/v5/internal/proxyproto"
)

// UnsubscribeProxy allows to send Unsubscribe requests.
type UnsubscribeProxy interface {
	ProxyUnsubscribe(context.Context, *proxyproto.UnsubscribeRequest) (*proxyproto.UnsubscribeResponse, error)
	// Protocol for metrics and logging.
	Protocol() string
	// UseBase64 for bytes in requests from Centrifugo to application backend.
	UseBase64() bool
	// IncludeMeta ...
	IncludeMeta() bool
}
