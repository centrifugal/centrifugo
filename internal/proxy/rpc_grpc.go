package proxy

import (
	"context"
	"fmt"
	"time"

	"github.com/centrifugal/centrifugo/v3/internal/proxyproto"

	"google.golang.org/grpc"
)

// GRPCRPCProxy ...
type GRPCRPCProxy struct {
	proxy  Proxy
	client proxyproto.CentrifugoProxyClient
}

var _ RPCProxy = (*GRPCRPCProxy)(nil)

// NewGRPCRPCProxy ...
func NewGRPCRPCProxy(p Proxy) (*GRPCRPCProxy, error) {
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
	return &GRPCRPCProxy{
		proxy:  p,
		client: proxyproto.NewCentrifugoProxyClient(conn),
	}, nil
}

// ProxyRPC ...
func (p *GRPCRPCProxy) ProxyRPC(ctx context.Context, req *proxyproto.RPCRequest) (*proxyproto.RPCResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(p.proxy.Timeout))
	defer cancel()
	return p.client.RPC(grpcRequestContext(ctx, p.proxy), req, grpc.ForceCodec(grpcCodec))
}

// Protocol ...
func (p *GRPCRPCProxy) Protocol() string {
	return "grpc"
}

// UseBase64 ...
func (p *GRPCRPCProxy) UseBase64() bool {
	return p.proxy.BinaryEncoding
}

// IncludeMeta ...
func (p *GRPCRPCProxy) IncludeMeta() bool {
	return p.proxy.IncludeConnectionMeta
}
