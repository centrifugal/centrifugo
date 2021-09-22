package proxy

import (
	"context"
	"fmt"
	"time"

	"github.com/centrifugal/centrifugo/v3/internal/proxyproto"

	"google.golang.org/grpc"
)

// GRPCRefreshProxy ...
type GRPCRefreshProxy struct {
	proxy  Proxy
	client proxyproto.CentrifugoProxyClient
}

var _ RefreshProxy = (*GRPCRefreshProxy)(nil)

// NewGRPCRefreshProxy ...
func NewGRPCRefreshProxy(p Proxy) (*GRPCRefreshProxy, error) {
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
	return &GRPCRefreshProxy{
		proxy:  p,
		client: proxyproto.NewCentrifugoProxyClient(conn),
	}, nil
}

// ProxyRefresh proxies refresh to application backend.
func (p *GRPCRefreshProxy) ProxyRefresh(ctx context.Context, req *proxyproto.RefreshRequest) (*proxyproto.RefreshResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(p.proxy.Timeout))
	defer cancel()
	return p.client.Refresh(grpcRequestContext(ctx, p.proxy), req, grpc.ForceCodec(grpcCodec))
}

// Protocol ...
func (p *GRPCRefreshProxy) Protocol() string {
	return "grpc"
}

// UseBase64 ...
func (p *GRPCRefreshProxy) UseBase64() bool {
	return p.proxy.BinaryEncoding
}

// IncludeMeta ...
func (p *GRPCRefreshProxy) IncludeMeta() bool {
	return p.proxy.IncludeConnectionMeta
}
