package proxy

import (
	"context"
	"fmt"

	"github.com/centrifugal/centrifugo/v6/internal/proxyproto"

	"google.golang.org/grpc"
)

// GRPCSharedPollRefreshProxy ...
type GRPCSharedPollRefreshProxy struct {
	config Config
	client proxyproto.CentrifugoProxyClient
}

var _ SharedPollRefreshProxy = (*GRPCSharedPollRefreshProxy)(nil)

// NewGRPCSharedPollRefreshProxy ...
func NewGRPCSharedPollRefreshProxy(name string, p Config) (*GRPCSharedPollRefreshProxy, error) {
	host, err := getGrpcHost(p.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("error getting grpc host: %v", err)
	}
	dialOpts, err := getDialOpts(name, p)
	if err != nil {
		return nil, fmt.Errorf("error creating GRPC dial options: %v", err)
	}
	conn, err := grpc.NewClient(host, dialOpts...)
	if err != nil {
		return nil, fmt.Errorf("error connecting to GRPC proxy server: %v", err)
	}
	return &GRPCSharedPollRefreshProxy{
		config: p,
		client: proxyproto.NewCentrifugoProxyClient(conn),
	}, nil
}

// ProxySharedPollRefresh proxies SharedPollRefresh to application backend.
func (p *GRPCSharedPollRefreshProxy) ProxySharedPollRefresh(ctx context.Context, req *proxyproto.SharedPollRefreshRequest) (*proxyproto.SharedPollRefreshResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, p.config.Timeout.ToDuration())
	defer cancel()
	return p.client.SharedPollRefresh(grpcRequestContext(ctx, p.config), req)
}

// Protocol ...
func (p *GRPCSharedPollRefreshProxy) Protocol() string {
	return "grpc"
}
