package proxystream

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/centrifugal/centrifugo/v5/internal/proxy"
	"github.com/centrifugal/centrifugo/v5/internal/proxyproto"
	"github.com/centrifugal/centrifugo/v5/internal/proxyutil"

	"google.golang.org/grpc"
)

var grpcCodec = proxyproto.Codec{}

type rpcCredentials struct {
	key   string
	value string
}

func (t rpcCredentials) GetRequestMetadata(_ context.Context, _ ...string) (map[string]string, error) {
	return map[string]string{
		t.key: t.value,
	}, nil
}

func (t rpcCredentials) RequireTransportSecurity() bool {
	return false
}

type PerCallData struct {
	Meta json.RawMessage
}

type Proxy struct {
	config proxy.Proxy
	client proxyproto.CentrifugoProxyClient
}

func NewProxy(c proxy.Proxy) (*Proxy, error) {
	host, err := proxyutil.GetGrpcHost(c.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("error getting grpc host: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(c.Timeout))
	defer cancel()

	dialConfig := proxyutil.DialConfig{
		GrpcCertFile: c.GrpcCertFile,
	}
	if c.GrpcCredentialsKey != "" {
		perRpcCredentials := &rpcCredentials{
			key:   c.GrpcCredentialsKey,
			value: c.GrpcCredentialsValue,
		}
		dialConfig.PerRPCCredentials = perRpcCredentials
	}

	dialOpts, err := proxyutil.GetDialOpts(dialConfig)
	if err != nil {
		return nil, fmt.Errorf("error creating GRPC dial options: %v", err)
	}
	conn, err := grpc.DialContext(ctx, host, dialOpts...)
	if err != nil {
		return nil, fmt.Errorf("error connecting to GRPC proxy server: %v", err)
	}
	return &Proxy{
		config: c,
		client: proxyproto.NewCentrifugoProxyClient(conn),
	}, nil
}

// SubscribeUnidirectional ...
func (p *Proxy) SubscribeUnidirectional(ctx context.Context, req *proxyproto.SubscribeRequest) (proxyproto.CentrifugoProxy_SubscribeUnidirectionalClient, error) {
	return p.client.SubscribeUnidirectional(proxyutil.GrpcRequestContext(ctx, p.config.HttpHeaders, p.config.GrpcMetadata), req, grpc.ForceCodec(grpcCodec))
}

// SubscribeBidirectional ...
func (p *Proxy) SubscribeBidirectional(ctx context.Context) (proxyproto.CentrifugoProxy_SubscribeBidirectionalClient, error) {
	return p.client.SubscribeBidirectional(proxyutil.GrpcRequestContext(ctx, p.config.HttpHeaders, p.config.GrpcMetadata), grpc.ForceCodec(grpcCodec))
}
