package proxy

import (
	"context"
	"fmt"

	"github.com/centrifugal/centrifugo/v6/internal/proxyproto"

	"google.golang.org/grpc"
)

// GRPCSubRefreshProxy ...
type GRPCSubRefreshProxy struct {
	config Config
	client proxyproto.CentrifugoProxyClient
}

var _ SubRefreshProxy = (*GRPCSubRefreshProxy)(nil)

// NewGRPCSubRefreshProxy ...
func NewGRPCSubRefreshProxy(name string, p Config) (*GRPCSubRefreshProxy, error) {
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
	return &GRPCSubRefreshProxy{
		config: p,
		client: proxyproto.NewCentrifugoProxyClient(conn),
	}, nil
}

// ProxySubRefresh proxies refresh to application backend.
func (p *GRPCSubRefreshProxy) ProxySubRefresh(ctx context.Context, req *proxyproto.SubRefreshRequest) (*proxyproto.SubRefreshResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, p.config.Timeout.ToDuration())
	defer cancel()
	return p.client.SubRefresh(grpcRequestContext(ctx, p.config), req, grpc.ForceCodec(grpcCodec))
}

// Protocol ...
func (p *GRPCSubRefreshProxy) Protocol() string {
	return "grpc"
}

// UseBase64 ...
func (p *GRPCSubRefreshProxy) UseBase64() bool {
	return p.config.BinaryEncoding
}

// IncludeMeta ...
func (p *GRPCSubRefreshProxy) IncludeMeta() bool {
	return p.config.IncludeConnectionMeta
}
