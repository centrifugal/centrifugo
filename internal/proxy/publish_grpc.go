package proxy

import (
	"context"
	"fmt"

	"github.com/centrifugal/centrifugo/v6/internal/proxyproto"

	"google.golang.org/grpc"
)

// GRPCPublishProxy ...
type GRPCPublishProxy struct {
	config Config
	client proxyproto.CentrifugoProxyClient
}

var _ PublishProxy = (*GRPCPublishProxy)(nil)

// NewGRPCPublishProxy ...
func NewGRPCPublishProxy(name string, p Config) (*GRPCPublishProxy, error) {
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
	return &GRPCPublishProxy{
		config: p,
		client: proxyproto.NewCentrifugoProxyClient(conn),
	}, nil
}

// ProxyPublish proxies Publish to application backend.
func (p *GRPCPublishProxy) ProxyPublish(ctx context.Context, req *proxyproto.PublishRequest) (*proxyproto.PublishResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, p.config.Timeout.ToDuration())
	defer cancel()
	return p.client.Publish(grpcRequestContext(ctx, p.config), req, grpc.ForceCodec(grpcCodec))
}

// Protocol ...
func (p *GRPCPublishProxy) Protocol() string {
	return "grpc"
}

// UseBase64 ...
func (p *GRPCPublishProxy) UseBase64() bool {
	return p.config.BinaryEncoding
}

// IncludeMeta ...
func (p *GRPCPublishProxy) IncludeMeta() bool {
	return p.config.IncludeConnectionMeta
}
