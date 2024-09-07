package proxy

import (
	"context"
	"encoding/json"
	"net"
	"strings"

	"github.com/centrifugal/centrifugo/v5/internal/tools"
)

// Config for proxy.
type Config struct {
	// Name is a unique name of proxy to reference.
	Name string `mapstructure:"name" json:"name" envconfig:"name"`

	// Enabled must be true to tell Centrifugo to use the configured proxy.
	Enabled bool `mapstructure:"enabled" json:"enabled" envconfig:"enabled"`

	// Endpoint - HTTP address or GRPC service endpoint.
	Endpoint string `mapstructure:"endpoint" json:"endpoint" envconfig:"endpoint"`
	// Timeout for proxy request.
	Timeout tools.Duration `mapstructure:"timeout" json:"timeout,omitempty" envconfig:"timeout"`

	// HTTPHeaders is a list of HTTP headers to proxy. No headers used by proxy by default.
	// If GRPC proxy is used then request HTTP headers set to outgoing request metadata.
	HttpHeaders []string `mapstructure:"http_headers" json:"http_headers,omitempty" envconfig:"http_headers"`
	// GRPCMetadata is a list of GRPC metadata keys to proxy. No meta keys used by proxy by
	// default. If HTTP proxy is used then these keys become outgoing request HTTP headers.
	GrpcMetadata []string `mapstructure:"grpc_metadata" json:"grpc_metadata,omitempty" envconfig:"grpc_metadata"`

	// StaticHttpHeaders is a static set of key/value pairs to attach to HTTP proxy request as
	// headers. Headers received from HTTP client request or metadata from GRPC client request
	// both have priority over values set in StaticHttpHeaders map.
	StaticHttpHeaders map[string]string `mapstructure:"static_http_headers" json:"static_http_headers,omitempty" envconfig:"static_http_headers"`

	// BinaryEncoding makes proxy send data as base64 string (assuming it contains custom
	// non-JSON payload).
	BinaryEncoding bool `mapstructure:"binary_encoding" json:"binary_encoding,omitempty" envconfig:"binary_encoding"`
	// IncludeConnectionMeta to each proxy request (except connect where it's obtained).
	IncludeConnectionMeta bool `mapstructure:"include_connection_meta" json:"include_connection_meta,omitempty" envconfig:"include_connection_meta"`

	// GrpcTLS is a common configuration for GRPC TLS.
	GrpcTLS tools.TLSConfig `mapstructure:"grpc_tls" json:"grpc_tls,omitempty" envconfig:"grpc_tls"`
	// GrpcCertFile is a path to GRPC cert file on disk.
	GrpcCertFile string `mapstructure:"grpc_cert_file" json:"grpc_cert_file,omitempty" envconfig:"grpc_cert_file"`
	// GrpcCredentialsKey is a custom key to add into per-RPC credentials.
	GrpcCredentialsKey string `mapstructure:"grpc_credentials_key" json:"grpc_credentials_key,omitempty" envconfig:"grpc_credentials_key"`
	// GrpcCredentialsValue is a custom value for GrpcCredentialsKey.
	GrpcCredentialsValue string `mapstructure:"grpc_credentials_value" json:"grpc_credentials_value,omitempty" envconfig:"grpc_credentials_value"`
	// GrpcCompression enables compression for outgoing calls (gzip).
	GrpcCompression bool `mapstructure:"grpc_compression" json:"grpc_compression,omitempty" envconfig:"grpc_compression"`

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

func GetConnectProxy(p Config) (ConnectProxy, error) {
	if isHttpEndpoint(p.Endpoint) {
		return NewHTTPConnectProxy(p)
	}
	return NewGRPCConnectProxy(p)
}

func GetRefreshProxy(p Config) (RefreshProxy, error) {
	if isHttpEndpoint(p.Endpoint) {
		return NewHTTPRefreshProxy(p)
	}
	return NewGRPCRefreshProxy(p)
}

func GetRpcProxy(p Config) (RPCProxy, error) {
	if isHttpEndpoint(p.Endpoint) {
		return NewHTTPRPCProxy(p)
	}
	return NewGRPCRPCProxy(p)
}

func GetSubRefreshProxy(p Config) (SubRefreshProxy, error) {
	if isHttpEndpoint(p.Endpoint) {
		return NewHTTPSubRefreshProxy(p)
	}
	return NewGRPCSubRefreshProxy(p)
}

func GetPublishProxy(p Config) (PublishProxy, error) {
	if isHttpEndpoint(p.Endpoint) {
		return NewHTTPPublishProxy(p)
	}
	return NewGRPCPublishProxy(p)
}

func GetSubscribeProxy(p Config) (SubscribeProxy, error) {
	if isHttpEndpoint(p.Endpoint) {
		return NewHTTPSubscribeProxy(p)
	}
	return NewGRPCSubscribeProxy(p)
}

func GetCacheEmptyProxy(p Config) (CacheEmptyProxy, error) {
	if isHttpEndpoint(p.Endpoint) {
		return NewHTTPCacheEmptyProxy(p)
	}
	return NewGRPCCacheEmptyProxy(p)
}

type PerCallData struct {
	Meta json.RawMessage
}
