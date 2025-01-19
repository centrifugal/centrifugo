package proxy

import (
	"context"
	"fmt"

	"github.com/centrifugal/centrifugo/v6/internal/proxyproto"

	"google.golang.org/grpc"
)

// GRPCSubscribeProxy ...
type GRPCSubscribeProxy struct {
	config Config
	client proxyproto.CentrifugoProxyClient
}

var _ SubscribeProxy = (*GRPCSubscribeProxy)(nil)

// NewGRPCSubscribeProxy ...
func NewGRPCSubscribeProxy(name string, p Config) (*GRPCSubscribeProxy, error) {
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
	return &GRPCSubscribeProxy{
		config: p,
		client: proxyproto.NewCentrifugoProxyClient(conn),
	}, nil
}

// ProxySubscribe proxies Subscribe to application backend.
func (p *GRPCSubscribeProxy) ProxySubscribe(ctx context.Context, req *proxyproto.SubscribeRequest) (*proxyproto.SubscribeResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, p.config.Timeout.ToDuration())
	defer cancel()
	return p.client.Subscribe(grpcRequestContext(ctx, p.config), req, grpc.ForceCodec(grpcCodec))
}

// Protocol ...
func (p *GRPCSubscribeProxy) Protocol() string {
	return "grpc"
}

// UseBase64 ...
func (p *GRPCSubscribeProxy) UseBase64() bool {
	return p.config.BinaryEncoding
}

// IncludeMeta ...
func (p *GRPCSubscribeProxy) IncludeMeta() bool {
	return p.config.IncludeConnectionMeta
}
