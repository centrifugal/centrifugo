package proxy

import (
	"time"
)

// Config for proxy.
type Config struct {
	// ExtraHTTPHeaders is a list of extra HTTP headers to proxy.
	ExtraHTTPHeaders []string
	// ConnectEndpoint ...
	ConnectEndpoint string
	// ConnectTimeout ...
	ConnectTimeout time.Duration
	// RefreshEndpoint ...
	RefreshEndpoint string
	// RefreshTimeout ...
	RefreshTimeout time.Duration
	// RPCEndpoint ...
	RPCEndpoint string
	// RPCTimeout ...
	RPCTimeout time.Duration
}
