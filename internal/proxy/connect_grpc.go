package proxy

import (
	"context"
	"fmt"

	"github.com/centrifugal/centrifugo/v6/internal/proxyproto"

	"google.golang.org/grpc"
)

// GRPCConnectProxy ...
type GRPCConnectProxy struct {
	config Config
	client proxyproto.CentrifugoProxyClient
}

var _ ConnectProxy = (*GRPCConnectProxy)(nil)

// NewGRPCConnectProxy ...
func NewGRPCConnectProxy(name string, p Config) (*GRPCConnectProxy, error) {
	host, err := getGrpcHost(p.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("error getting grpc host: %v", err)
	}
	dialOpts, err := getDialOpts(name, p)
	if err != nil {
		return nil, fmt.Errorf("error creating GRPC dial options: %v", err)
	}
	conn, err := grpc.NewClient(host, dialOpts...)
	if err != nil {
		return nil, fmt.Errorf("error connecting to GRPC proxy server: %v", err)
	}
	return &GRPCConnectProxy{
		config: p,
		client: proxyproto.NewCentrifugoProxyClient(conn),
	}, nil
}

// Protocol ...
func (p *GRPCConnectProxy) Protocol() string {
	return "grpc"
}

func (p *GRPCConnectProxy) Name() string {
	return "default"
}

// UseBase64 ...
func (p *GRPCConnectProxy) UseBase64() bool {
	return p.config.BinaryEncoding
}

// ProxyConnect proxies connect control to application backend.
func (p *GRPCConnectProxy) ProxyConnect(ctx context.Context, req *proxyproto.ConnectRequest) (*proxyproto.ConnectResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, p.config.Timeout.ToDuration())
	defer cancel()
	return p.client.Connect(grpcRequestContext(ctx, p.config), req, grpc.ForceCodec(grpcCodec))
}
