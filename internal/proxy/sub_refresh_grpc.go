package proxy

import (
	"context"
	"fmt"
	"time"

	"github.com/centrifugal/centrifugo/v5/internal/proxyproto"

	"google.golang.org/grpc"
)

// GRPCSubRefreshProxy ...
type GRPCSubRefreshProxy struct {
	proxy  Proxy
	client proxyproto.CentrifugoProxyClient
}

var _ SubRefreshProxy = (*GRPCSubRefreshProxy)(nil)

// NewGRPCSubRefreshProxy ...
func NewGRPCSubRefreshProxy(p Proxy) (*GRPCSubRefreshProxy, error) {
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
	return &GRPCSubRefreshProxy{
		proxy:  p,
		client: proxyproto.NewCentrifugoProxyClient(conn),
	}, nil
}

// ProxySubRefresh proxies refresh to application backend.
func (p *GRPCSubRefreshProxy) ProxySubRefresh(ctx context.Context, req *proxyproto.SubRefreshRequest) (*proxyproto.SubRefreshResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(p.proxy.Timeout))
	defer cancel()
	return p.client.SubRefresh(grpcRequestContext(ctx, p.proxy), req, grpc.ForceCodec(grpcCodec))
}

// Protocol ...
func (p *GRPCSubRefreshProxy) Protocol() string {
	return "grpc"
}

// UseBase64 ...
func (p *GRPCSubRefreshProxy) UseBase64() bool {
	return p.proxy.BinaryEncoding
}

// IncludeMeta ...
func (p *GRPCSubRefreshProxy) IncludeMeta() bool {
	return p.proxy.IncludeConnectionMeta
}
