package proxy

import (
	"context"
	"fmt"

	"github.com/centrifugal/centrifugo/v6/internal/proxyproto"

	"google.golang.org/grpc"
)

// GRPCMapRemoveProxy ...
type GRPCMapRemoveProxy struct {
	config Config
	client proxyproto.CentrifugoProxyClient
}

var _ MapRemoveProxy = (*GRPCMapRemoveProxy)(nil)

// NewGRPCMapRemoveProxy ...
func NewGRPCMapRemoveProxy(name string, p Config) (*GRPCMapRemoveProxy, error) {
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
	return &GRPCMapRemoveProxy{
		config: p,
		client: proxyproto.NewCentrifugoProxyClient(conn),
	}, nil
}

// ProxyMapRemove proxies MapRemove to application backend.
func (p *GRPCMapRemoveProxy) ProxyMapRemove(ctx context.Context, req *proxyproto.MapRemoveRequest) (*proxyproto.MapRemoveResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, p.config.Timeout.ToDuration())
	defer cancel()
	return p.client.MapRemove(grpcRequestContext(ctx, p.config), req)
}

// Protocol ...
func (p *GRPCMapRemoveProxy) Protocol() string {
	return "grpc"
}

// UseBase64 ...
func (p *GRPCMapRemoveProxy) UseBase64() bool {
	return p.config.BinaryEncoding
}

// IncludeMeta ...
func (p *GRPCMapRemoveProxy) IncludeMeta() bool {
	return p.config.IncludeConnectionMeta
}
