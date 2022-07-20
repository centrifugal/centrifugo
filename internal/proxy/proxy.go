package proxy

import (
	"context"
	"encoding/json"
	"net"
	"strings"

	"github.com/centrifugal/centrifugo/v4/internal/tools"
)

// Proxy model.
type Proxy struct {
	// Name is a unique name of proxy to reference.
	Name string `mapstructure:"name" json:"name"`
	// Endpoint - HTTP address or GRPC service endpoint.
	Endpoint string `mapstructure:"endpoint" json:"endpoint"`
	// Timeout for proxy request.
	Timeout tools.Duration `mapstructure:"timeout" json:"timeout,omitempty"`

	// HTTPHeaders is a list of HTTP headers to proxy.  No headers used by proxy by default.
	// If GRPC proxy is used then request HTTP headers set to outgoing request metadata.
	HttpHeaders []string `mapstructure:"http_headers" json:"http_headers,omitempty"`
	// GRPCMetadata is a list of GRPC metadata keys to proxy. No meta keys used by proxy by
	// default. If HTTP proxy is used then these keys become outgoing request HTTP headers.
	GrpcMetadata []string `mapstructure:"grpc_metadata" json:"grpc_metadata,omitempty"`

	// BinaryEncoding makes proxy send data as base64 string (assuming it contains custom
	// non-JSON payload).
	BinaryEncoding bool `mapstructure:"binary_encoding" json:"binary_encoding,omitempty"`
	// IncludeConnectionMeta to each proxy request (except connect where it's obtained).
	IncludeConnectionMeta bool `mapstructure:"include_connection_meta" json:"include_connection_meta,omitempty"`

	// GrpcCertFile is a path to GRPC cert file on disk.
	GrpcCertFile string `mapstructure:"grpc_cert_file" json:"grpc_cert_file,omitempty"`
	// GrpcCredentialsKey is a custom key to add into per-RPC credentials.
	GrpcCredentialsKey string `mapstructure:"grpc_credentials_key" json:"grpc_credentials_key,omitempty"`
	// GrpcCredentialsValue is a custom value for GrpcCredentialsKey.
	GrpcCredentialsValue string `mapstructure:"grpc_credentials_value" json:"grpc_credentials_value,omitempty"`

	testGrpcDialer func(context.Context, string) (net.Conn, error)
}

func getEncoding(useBase64 bool) string {
	if useBase64 {
		return "binary"
	}
	return "json"
}

func isHttpEndpoint(endpoint string) bool {
	return strings.HasPrefix(endpoint, "http://") || strings.HasPrefix(endpoint, "https://")
}

func GetConnectProxy(p Proxy) (ConnectProxy, error) {
	if isHttpEndpoint(p.Endpoint) {
		return NewHTTPConnectProxy(p)
	}
	return NewGRPCConnectProxy(p)
}

func GetRefreshProxy(p Proxy) (RefreshProxy, error) {
	if isHttpEndpoint(p.Endpoint) {
		return NewHTTPRefreshProxy(p)
	}
	return NewGRPCRefreshProxy(p)
}

func GetRpcProxy(p Proxy) (RPCProxy, error) {
	if isHttpEndpoint(p.Endpoint) {
		return NewHTTPRPCProxy(p)
	}
	return NewGRPCRPCProxy(p)
}

func GetPublishProxy(p Proxy) (PublishProxy, error) {
	if isHttpEndpoint(p.Endpoint) {
		return NewHTTPPublishProxy(p)
	}
	return NewGRPCPublishProxy(p)
}

func GetSubscribeProxy(p Proxy) (SubscribeProxy, error) {
	if isHttpEndpoint(p.Endpoint) {
		return NewHTTPSubscribeProxy(p)
	}
	return NewGRPCSubscribeProxy(p)
}

type PerCallData struct {
	Meta json.RawMessage
}
