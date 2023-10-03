package proxy

import (
	"context"
	"fmt"
	"time"

	"github.com/centrifugal/centrifugo/v5/internal/proxyproto"

	"google.golang.org/grpc"
)

// GRPCPublishProxy ...
type GRPCPublishProxy struct {
	config Config
	client proxyproto.CentrifugoProxyClient
}

var _ PublishProxy = (*GRPCPublishProxy)(nil)

// NewGRPCPublishProxy ...
func NewGRPCPublishProxy(p Config) (*GRPCPublishProxy, error) {
	host, err := getGrpcHost(p.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("error getting grpc host: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(p.Timeout))
	defer cancel()
	dialOpts, err := getDialOpts(p)
	if err != nil {
		return nil, fmt.Errorf("error creating GRPC dial options: %v", err)
	}
	conn, err := grpc.DialContext(ctx, host, dialOpts...)
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
	ctx, cancel := context.WithTimeout(ctx, time.Duration(p.config.Timeout))
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
