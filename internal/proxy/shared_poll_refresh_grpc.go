package proxy

import (
	"context"
	"fmt"

	"github.com/centrifugal/centrifugo/v6/internal/proxyproto"
)

// GRPCSharedPollRefreshProxy ...
type GRPCSharedPollRefreshProxy struct {
	config Config
}

var _ SharedPollRefreshProxy = (*GRPCSharedPollRefreshProxy)(nil)

// NewGRPCSharedPollRefreshProxy ...
func NewGRPCSharedPollRefreshProxy(_ string, _ Config) (*GRPCSharedPollRefreshProxy, error) {
	return nil, fmt.Errorf("gRPC shared poll refresh proxy is not supported yet, use HTTP endpoint")
}

// ProxySharedPollRefresh ...
func (p *GRPCSharedPollRefreshProxy) ProxySharedPollRefresh(_ context.Context, _ *proxyproto.SharedPollRefreshRequest) (*proxyproto.SharedPollRefreshResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

// Protocol ...
func (p *GRPCSharedPollRefreshProxy) Protocol() string {
	return "grpc"
}
