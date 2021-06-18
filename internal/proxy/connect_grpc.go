package proxy

import (
	"context"
	"fmt"
	"net/url"

	"github.com/centrifugal/centrifugo/v3/internal/proxy/proxyproto"

	"google.golang.org/grpc"
)

// GRPCConnectProxy ...
type GRPCConnectProxy struct {
	endpoint string
	client   proxyproto.CentrifugoProxyClient
	config   Config
}

var _ ConnectProxy = (*GRPCConnectProxy)(nil)

// NewGRPCConnectProxy ...
func NewGRPCConnectProxy(endpoint string, config Config) (*GRPCConnectProxy, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), config.ConnectTimeout)
	defer cancel()
	dialOpts, err := getDialOpts(config)
	if err != nil {
		return nil, fmt.Errorf("error creating GRPC dial options: %v", err)
	}
	conn, err := grpc.DialContext(ctx, u.Host, dialOpts...)
	if err != nil {
		return nil, fmt.Errorf("error connecting to GRPC proxy server: %v", err)
	}
	return &GRPCConnectProxy{
		endpoint: endpoint,
		client:   proxyproto.NewCentrifugoProxyClient(conn),
		config:   config,
	}, nil
}

// Protocol ...
func (p *GRPCConnectProxy) Protocol() string {
	return "grpc"
}

// UseBase64 ...
func (p *GRPCConnectProxy) UseBase64() bool {
	return p.config.BinaryEncoding
}

// ProxyConnect proxies connect control to application backend.
func (p *GRPCConnectProxy) ProxyConnect(ctx context.Context, req *proxyproto.ConnectRequest) (*proxyproto.ConnectResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, p.config.ConnectTimeout)
	defer cancel()
	return p.client.Connect(grpcRequestContext(ctx, p.config), req, grpc.ForceCodec(p.config.GRPCConfig.Codec))
}
