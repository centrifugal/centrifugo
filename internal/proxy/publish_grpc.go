package proxy

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"google.golang.org/grpc"

	"github.com/centrifugal/centrifugo/v3/internal/proxy/proxyproto"
)

// GRPCPublishProxy ...
type GRPCPublishProxy struct {
	endpoint string
	timeout  time.Duration
	client   proxyproto.CentrifugoProxyClient
	config   Config
}

var _ PublishProxy = (*GRPCPublishProxy)(nil)

// NewGRPCPublishProxy ...
func NewGRPCPublishProxy(endpoint string, config Config) (*GRPCPublishProxy, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), config.PublishTimeout)
	defer cancel()
	dialOpts, err := getDialOpts(config)
	if err != nil {
		return nil, fmt.Errorf("error creating GRPC dial options: %v", err)
	}
	conn, err := grpc.DialContext(ctx, u.Host, dialOpts...)
	if err != nil {
		return nil, fmt.Errorf("error connecting to GRPC proxy server: %v", err)
	}
	return &GRPCPublishProxy{
		endpoint: endpoint,
		timeout:  config.PublishTimeout,
		client:   proxyproto.NewCentrifugoProxyClient(conn),
		config:   config,
	}, nil
}

// ProxyPublish proxies Publish to application backend.
func (p *GRPCPublishProxy) ProxyPublish(ctx context.Context, req *proxyproto.PublishRequest) (*proxyproto.PublishResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, p.config.PublishTimeout)
	defer cancel()
	return p.client.Publish(grpcRequestContext(ctx, p.config), req, grpc.ForceCodec(p.config.GRPCConfig.Codec))
}

// Protocol ...
func (p *GRPCPublishProxy) Protocol() string {
	return "grpc"
}

// UseBase64 ...
func (p *GRPCPublishProxy) UseBase64() bool {
	return p.config.BinaryEncoding
}
