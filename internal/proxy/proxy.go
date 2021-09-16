package proxy

import "github.com/centrifugal/centrifugo/v3/internal/tools"

func getEncoding(useBase64 bool) string {
	if useBase64 {
		return "binary"
	}
	return "json"
}

// Proxy model.
type Proxy struct {
	// Name is a unique name of proxy to reference.
	Name string `mapstructure:"name" json:"name"`
	// Type of proxy: http or grpc.
	Type string `mapstructure:"type" json:"type"`
	// Endpoint - HTTP address or GRPC service url.
	Endpoint string `mapstructure:"endpoint" json:"endpoint"`
	// Timeout for proxy request.
	Timeout tools.Duration `mapstructure:"timeout" json:"timeout,omitempty"`

	// HTTPHeaders is a list of HTTP headers to proxy. No headers used by proxy by default.
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

	// GrpcSettings is a GRPC specific configuration.
	GrpcSettings *GrpcSettings `mapstructure:"grpc_settings" json:"grpc_settings,omitempty"`
	// HttpSettings is a HTTP specific configuration.
	HttpSettings *HttpSettings `mapstructure:"http_settings" json:"http_settings,omitempty"`
}

type GrpcSettings struct {
	CertFile         string `mapstructure:"cert_file" json:"cert_file,omitempty"`
	CredentialsKey   string `mapstructure:"credentials_key" json:"credentials_key,omitempty"`
	CredentialsValue string `mapstructure:"credentials_value" json:"credentials_value,omitempty"`
}

type HttpSettings struct {
}
