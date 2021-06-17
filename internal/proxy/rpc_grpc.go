package proxy

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/centrifugal/centrifugo/v3/internal/proxy/proxyproto"

	"google.golang.org/grpc"
)

// GRPCRPCProxy ...
type GRPCRPCProxy struct {
	endpoint string
	timeout  time.Duration
	client   proxyproto.CentrifugoProxyClient
	config   Config
}

var _ RPCProxy = (*GRPCRPCProxy)(nil)

// NewGRPCRPCProxy ...
func NewGRPCRPCProxy(endpoint string, config Config) (*GRPCRPCProxy, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), config.RPCTimeout)
	defer cancel()
	dialOpts, err := getDialOpts(config)
	if err != nil {
		return nil, fmt.Errorf("error creating GRPC dial options: %v", err)
	}
	conn, err := grpc.DialContext(ctx, u.Host, dialOpts...)
	if err != nil {
		return nil, fmt.Errorf("error connecting to GRPC proxy server: %v", err)
	}
	return &GRPCRPCProxy{
		endpoint: endpoint,
		timeout:  config.RPCTimeout,
		client:   proxyproto.NewCentrifugoProxyClient(conn),
		config:   config,
	}, nil
}

// ProxyRPC ...
func (p *GRPCRPCProxy) ProxyRPC(ctx context.Context, req *proxyproto.RPCRequest) (*proxyproto.RPCResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, p.config.RPCTimeout)
	defer cancel()
	return p.client.RPC(grpcRequestContext(ctx, p.config), req, grpc.ForceCodec(p.config.GRPCConfig.Codec))
}

// Protocol ...
func (p *GRPCRPCProxy) Protocol() string {
	return "grpc"
}

// UseBase64 ...
func (p *GRPCRPCProxy) UseBase64() bool {
	return p.config.BinaryEncoding
}
