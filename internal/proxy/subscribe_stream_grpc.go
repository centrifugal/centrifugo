package proxy

import (
	"context"
	"fmt"
	"time"

	"github.com/centrifugal/centrifugo/v5/internal/proxyproto"
	"github.com/centrifugal/centrifugo/v5/internal/proxyutil"

	"google.golang.org/grpc"
)

type SubscribeStreamProxy struct {
	config Proxy
	client proxyproto.CentrifugoProxyClient
}

func NewSubscribeStreamProxy(p Proxy) (*SubscribeStreamProxy, error) {
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

	return &SubscribeStreamProxy{
		config: p,
		client: proxyproto.NewCentrifugoProxyClient(conn),
	}, nil
}

// SubscribeUnidirectional ...
func (p *SubscribeStreamProxy) SubscribeUnidirectional(ctx context.Context, req *proxyproto.SubscribeRequest) (proxyproto.CentrifugoProxy_SubscribeUnidirectionalClient, error) {
	return p.client.SubscribeUnidirectional(proxyutil.GrpcRequestContext(ctx, p.config.HttpHeaders, p.config.GrpcMetadata), req, grpc.ForceCodec(grpcCodec))
}

// SubscribeBidirectional ...
func (p *SubscribeStreamProxy) SubscribeBidirectional(ctx context.Context) (proxyproto.CentrifugoProxy_SubscribeBidirectionalClient, error) {
	return p.client.SubscribeBidirectional(proxyutil.GrpcRequestContext(ctx, p.config.HttpHeaders, p.config.GrpcMetadata), grpc.ForceCodec(grpcCodec))
}
