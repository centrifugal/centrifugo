package proxy

import (
	"context"

	"github.com/centrifugal/centrifugo/v6/internal/proxyproto"
)

// PublishProxy allows to send Publish requests.
type PublishProxy interface {
	ProxyPublish(context.Context, *proxyproto.PublishRequest) (*proxyproto.PublishResponse, error)
	// Protocol for metrics and logging.
	Protocol() string
	// UseBase64 for bytes in requests from Centrifugo to application backend.
	UseBase64() bool
	// IncludeMeta ...
	IncludeMeta() bool
}
