package proxy

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/centrifugal/centrifugo/v3/internal/proxyproto"

	"google.golang.org/grpc"
)

// GRPCRefreshProxy ...
type GRPCRefreshProxy struct {
	endpoint string
	timeout  time.Duration
	client   proxyproto.CentrifugoProxyClient
	config   Config
}

var _ RefreshProxy = (*GRPCRefreshProxy)(nil)

// NewGRPCRefreshProxy ...
func NewGRPCRefreshProxy(endpoint string, config Config) (*GRPCRefreshProxy, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), config.RefreshTimeout)
	defer cancel()
	dialOpts, err := getDialOpts(config)
	if err != nil {
		return nil, fmt.Errorf("error creating GRPC dial options: %v", err)
	}
	conn, err := grpc.DialContext(ctx, u.Host, dialOpts...)
	if err != nil {
		return nil, fmt.Errorf("error connecting to GRPC proxy server: %v", err)
	}
	return &GRPCRefreshProxy{
		endpoint: endpoint,
		timeout:  config.RefreshTimeout,
		client:   proxyproto.NewCentrifugoProxyClient(conn),
		config:   config,
	}, nil
}

// ProxyRefresh proxies refresh to application backend.
func (p *GRPCRefreshProxy) ProxyRefresh(ctx context.Context, req *proxyproto.RefreshRequest) (*proxyproto.RefreshResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, p.config.RefreshTimeout)
	defer cancel()
	return p.client.Refresh(grpcRequestContext(ctx, p.config), req, grpc.ForceCodec(p.config.GRPCConfig.Codec))
}

// Protocol ...
func (p *GRPCRefreshProxy) Protocol() string {
	return "grpc"
}

// UseBase64 ...
func (p *GRPCRefreshProxy) UseBase64() bool {
	return p.config.BinaryEncoding
}

// IncludeMeta ...
func (p *GRPCRefreshProxy) IncludeMeta() bool {
	return p.config.IncludeConnectionMeta
}
