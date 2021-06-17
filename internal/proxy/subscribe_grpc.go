package proxy

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/centrifugal/centrifugo/v3/internal/proxy/proxyproto"

	"google.golang.org/grpc"
)

// GRPCSubscribeProxy ...
type GRPCSubscribeProxy struct {
	endpoint string
	timeout  time.Duration
	client   proxyproto.CentrifugoProxyClient
	config   Config
}

var _ SubscribeProxy = (*GRPCSubscribeProxy)(nil)

// NewGRPCSubscribeProxy ...
func NewGRPCSubscribeProxy(endpoint string, config Config) (*GRPCSubscribeProxy, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), config.SubscribeTimeout)
	defer cancel()
	dialOpts, err := getDialOpts(config)
	if err != nil {
		return nil, fmt.Errorf("error creating GRPC dial options: %v", err)
	}
	conn, err := grpc.DialContext(ctx, u.Host, dialOpts...)
	if err != nil {
		return nil, fmt.Errorf("error connecting to GRPC proxy server: %v", err)
	}
	return &GRPCSubscribeProxy{
		endpoint: endpoint,
		timeout:  config.SubscribeTimeout,
		client:   proxyproto.NewCentrifugoProxyClient(conn),
		config:   config,
	}, nil
}

// ProxySubscribe proxies Subscribe to application backend.
func (p *GRPCSubscribeProxy) ProxySubscribe(ctx context.Context, req *proxyproto.SubscribeRequest) (*proxyproto.SubscribeResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, p.config.SubscribeTimeout)
	defer cancel()
	return p.client.Subscribe(grpcRequestContext(ctx, p.config), req, grpc.ForceCodec(p.config.GRPCConfig.Codec))
}

// Protocol ...
func (p *GRPCSubscribeProxy) Protocol() string {
	return "grpc"
}

// UseBase64 ...
func (p *GRPCSubscribeProxy) UseBase64() bool {
	return p.config.BinaryEncoding
}
