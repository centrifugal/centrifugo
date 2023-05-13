package proxy

import (
	"context"
	"fmt"
	"time"

	"github.com/centrifugal/centrifugo/v5/internal/proxyproto"

	"google.golang.org/grpc"
)

// GRPCSubscribeProxy ...
type GRPCSubscribeProxy struct {
	proxy  Proxy
	client proxyproto.CentrifugoProxyClient
}

var _ SubscribeProxy = (*GRPCSubscribeProxy)(nil)

// NewGRPCSubscribeProxy ...
func NewGRPCSubscribeProxy(p Proxy) (*GRPCSubscribeProxy, error) {
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
	return &GRPCSubscribeProxy{
		proxy:  p,
		client: proxyproto.NewCentrifugoProxyClient(conn),
	}, nil
}

// ProxySubscribe proxies Subscribe to application backend.
func (p *GRPCSubscribeProxy) ProxySubscribe(ctx context.Context, req *proxyproto.SubscribeRequest) (*proxyproto.SubscribeResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(p.proxy.Timeout))
	defer cancel()
	return p.client.Subscribe(grpcRequestContext(ctx, p.proxy), req, grpc.ForceCodec(grpcCodec))
}

// Protocol ...
func (p *GRPCSubscribeProxy) Protocol() string {
	return "grpc"
}

// UseBase64 ...
func (p *GRPCSubscribeProxy) UseBase64() bool {
	return p.proxy.BinaryEncoding
}

// IncludeMeta ...
func (p *GRPCSubscribeProxy) IncludeMeta() bool {
	return p.proxy.IncludeConnectionMeta
}
