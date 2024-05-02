package proxy

import (
	"context"
	"fmt"
	"time"

	"github.com/centrifugal/centrifugo/v5/internal/proxyproto"

	"google.golang.org/grpc"
)

// GRPCCacheEmptyProxy ...
type GRPCCacheEmptyProxy struct {
	config Config
	client proxyproto.CentrifugoProxyClient
}

var _ CacheEmptyProxy = (*GRPCCacheEmptyProxy)(nil)

// NewGRPCCacheEmptyProxy ...
func NewGRPCCacheEmptyProxy(p Config) (*GRPCCacheEmptyProxy, error) {
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
	return &GRPCCacheEmptyProxy{
		config: p,
		client: proxyproto.NewCentrifugoProxyClient(conn),
	}, nil
}

// Protocol ...
func (p *GRPCCacheEmptyProxy) Protocol() string {
	return "grpc"
}

// UseBase64 ...
func (p *GRPCCacheEmptyProxy) UseBase64() bool {
	return p.config.BinaryEncoding
}

// CacheEmpty ...
func (p *GRPCCacheEmptyProxy) NotifyCacheEmpty(ctx context.Context, req *proxyproto.NotifyCacheEmptyRequest) (*proxyproto.NotifyCacheEmptyResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(p.config.Timeout))
	defer cancel()
	return p.client.NotifyCacheEmpty(grpcRequestContext(ctx, p.config), req, grpc.ForceCodec(grpcCodec))
}
