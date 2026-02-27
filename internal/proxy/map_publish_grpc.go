package proxy

import (
	"context"
	"fmt"

	"github.com/centrifugal/centrifugo/v6/internal/proxyproto"

	"google.golang.org/grpc"
)

// GRPCMapPublishProxy ...
type GRPCMapPublishProxy struct {
	config Config
	client proxyproto.CentrifugoProxyClient
}

var _ MapPublishProxy = (*GRPCMapPublishProxy)(nil)

// NewGRPCMapPublishProxy ...
func NewGRPCMapPublishProxy(name string, p Config) (*GRPCMapPublishProxy, error) {
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
	return &GRPCMapPublishProxy{
		config: p,
		client: proxyproto.NewCentrifugoProxyClient(conn),
	}, nil
}

// ProxyMapPublish proxies MapPublish to application backend.
func (p *GRPCMapPublishProxy) ProxyMapPublish(ctx context.Context, req *proxyproto.MapPublishRequest) (*proxyproto.MapPublishResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, p.config.Timeout.ToDuration())
	defer cancel()
	return p.client.MapPublish(grpcRequestContext(ctx, p.config), req)
}

// Protocol ...
func (p *GRPCMapPublishProxy) Protocol() string {
	return "grpc"
}

// UseBase64 ...
func (p *GRPCMapPublishProxy) UseBase64() bool {
	return p.config.BinaryEncoding
}

// IncludeMeta ...
func (p *GRPCMapPublishProxy) IncludeMeta() bool {
	return p.config.IncludeConnectionMeta
}
