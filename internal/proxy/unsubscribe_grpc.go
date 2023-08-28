package proxy

import (
	"context"
	"fmt"
	"time"

	"github.com/centrifugal/centrifugo/v5/internal/proxyproto"

	"google.golang.org/grpc"
)

// GRPCUnsubscribeProxy ...
type GRPCUnsubscribeProxy struct {
	proxy  Proxy
	client proxyproto.CentrifugoProxyClient
}

var _ UnsubscribeProxy = (*GRPCUnsubscribeProxy)(nil)

// NewGRPCUnsubscribeProxy ...
func NewGRPCUnsubscribeProxy(p Proxy) (*GRPCUnsubscribeProxy, error) {
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
	return &GRPCUnsubscribeProxy{
		proxy:  p,
		client: proxyproto.NewCentrifugoProxyClient(conn),
	}, nil
}

// ProxyUnsubscribe proxies Unsubscribe to application backend.
func (p *GRPCUnsubscribeProxy) ProxyUnsubscribe(ctx context.Context, req *proxyproto.UnsubscribeRequest) (*proxyproto.UnsubscribeResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(p.proxy.Timeout))
	defer cancel()
	return p.client.Unsubscribe(grpcRequestContext(ctx, p.proxy), req, grpc.ForceCodec(grpcCodec))
}

// Protocol ...
func (p *GRPCUnsubscribeProxy) Protocol() string {
	return "grpc"
}

// UseBase64 ...
func (p *GRPCUnsubscribeProxy) UseBase64() bool {
	return p.proxy.BinaryEncoding
}

// IncludeMeta ...
func (p *GRPCUnsubscribeProxy) IncludeMeta() bool {
	return p.proxy.IncludeConnectionMeta
}
