package proxy

import (
	"context"

	"github.com/centrifugal/centrifugo/v6/internal/proxyproto"
)

// ConnectProxy allows to proxy connect requests to application backend to
// authenticate client connection.
type ConnectProxy interface {
	ProxyConnect(context.Context, *proxyproto.ConnectRequest) (*proxyproto.ConnectResponse, error)
	// Name returns proxy name.
	Name() string
	// Protocol for metrics and logging.
	Protocol() string
	// UseBase64 for bytes in requests from Centrifugo to application backend.
	UseBase64() bool
}
