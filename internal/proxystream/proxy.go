package proxystream

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"time"

	"github.com/centrifugal/centrifugo/v5/internal/proxystreamproto"
	"github.com/centrifugal/centrifugo/v5/internal/proxyutil"
	"github.com/centrifugal/centrifugo/v5/internal/tools"

	"google.golang.org/grpc"
)

var grpcCodec = proxystreamproto.Codec{}

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

// Config model.
type Config struct {
	// Name is a unique name of proxy to reference.
	Name string `mapstructure:"name" json:"name"`
	// Endpoint - GRPC service endpoint.
	Endpoint string `mapstructure:"endpoint" json:"endpoint"`
	// Timeout for proxy request.
	Timeout tools.Duration `mapstructure:"timeout" json:"timeout,omitempty"`

	// HTTPHeaders is a list of HTTP headers to proxy.  No headers used by proxy by default.
	// If GRPC proxy is used then request HTTP headers set to outgoing request metadata.
	HttpHeaders []string `mapstructure:"http_headers" json:"http_headers,omitempty"`
	// GRPCMetadata is a list of GRPC metadata keys to proxy. No meta keys used by proxy by
	// default. If HTTP proxy is used then these keys become outgoing request HTTP headers.
	GrpcMetadata []string `mapstructure:"grpc_metadata" json:"grpc_metadata,omitempty"`

	// IncludeConnectionMeta to each proxy request.
	IncludeConnectionMeta bool `mapstructure:"include_connection_meta" json:"include_connection_meta,omitempty"`

	// GrpcCertFile is a path to GRPC cert file on disk.
	GrpcCertFile string `mapstructure:"grpc_cert_file" json:"grpc_cert_file,omitempty"`
	// GrpcCredentialsKey is a custom key to add into per-RPC credentials.
	GrpcCredentialsKey string `mapstructure:"grpc_credentials_key" json:"grpc_credentials_key,omitempty"`
	// GrpcCredentialsValue is a custom value for GrpcCredentialsKey.
	GrpcCredentialsValue string `mapstructure:"grpc_credentials_value" json:"grpc_credentials_value,omitempty"`

	testGrpcDialer func(context.Context, string) (net.Conn, error)
}

type PerCallData struct {
	Meta json.RawMessage
}

type Proxy struct {
	config Config
	client proxystreamproto.CentrifugoProxyStreamClient
}

func NewProxy(c Config) (*Proxy, error) {
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
		client: proxystreamproto.NewCentrifugoProxyStreamClient(conn),
	}, nil
}

// ConnectUnidirectional ...
func (p *Proxy) ConnectUnidirectional(ctx context.Context, req *proxystreamproto.ConnectRequest) (proxystreamproto.CentrifugoProxyStream_ConnectUnidirectionalClient, error) {
	return p.client.ConnectUnidirectional(proxyutil.GrpcRequestContext(ctx, p.config.HttpHeaders, p.config.GrpcMetadata), req, grpc.ForceCodec(grpcCodec))
}

// ConnectBidirectional ...
func (p *Proxy) ConnectBidirectional(ctx context.Context) (proxystreamproto.CentrifugoProxyStream_ConnectBidirectionalClient, error) {
	return p.client.ConnectBidirectional(proxyutil.GrpcRequestContext(ctx, p.config.HttpHeaders, p.config.GrpcMetadata), grpc.ForceCodec(grpcCodec))
}

// SubscribeUnidirectional ...
func (p *Proxy) SubscribeUnidirectional(ctx context.Context, req *proxystreamproto.SubscribeRequest) (proxystreamproto.CentrifugoProxyStream_SubscribeUnidirectionalClient, error) {
	return p.client.SubscribeUnidirectional(proxyutil.GrpcRequestContext(ctx, p.config.HttpHeaders, p.config.GrpcMetadata), req, grpc.ForceCodec(grpcCodec))
}

// SubscribeBidirectional ...
func (p *Proxy) SubscribeBidirectional(ctx context.Context) (proxystreamproto.CentrifugoProxyStream_SubscribeBidirectionalClient, error) {
	return p.client.SubscribeBidirectional(proxyutil.GrpcRequestContext(ctx, p.config.HttpHeaders, p.config.GrpcMetadata), grpc.ForceCodec(grpcCodec))
}
