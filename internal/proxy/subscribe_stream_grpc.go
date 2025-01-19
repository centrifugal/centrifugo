package proxy

import (
	"context"
	"fmt"

	"github.com/centrifugal/centrifugo/v6/internal/proxyproto"

	"google.golang.org/grpc"
)

type SubscribeStreamProxy struct {
	config Config
	client proxyproto.CentrifugoProxyClient
}

func NewSubscribeStreamProxy(name string, p Config) (*SubscribeStreamProxy, error) {
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

	return &SubscribeStreamProxy{
		config: p,
		client: proxyproto.NewCentrifugoProxyClient(conn),
	}, nil
}

// SubscribeUnidirectional ...
func (p *SubscribeStreamProxy) SubscribeUnidirectional(ctx context.Context, req *proxyproto.SubscribeRequest) (proxyproto.CentrifugoProxy_SubscribeUnidirectionalClient, error) {
	return p.client.SubscribeUnidirectional(grpcRequestContext(ctx, p.config), req, grpc.ForceCodec(grpcCodec))
}

// SubscribeBidirectional ...
func (p *SubscribeStreamProxy) SubscribeBidirectional(ctx context.Context) (proxyproto.CentrifugoProxy_SubscribeBidirectionalClient, error) {
	return p.client.SubscribeBidirectional(grpcRequestContext(ctx, p.config), grpc.ForceCodec(grpcCodec))
}
