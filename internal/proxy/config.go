package proxy

import (
	"time"

	"github.com/centrifugal/centrifugo/v3/internal/proxyproto"

	"google.golang.org/grpc/encoding"
)

// Config for proxy.
type Config struct {
	// HTTPHeaders is a list of HTTP headers to proxy. No headers used by proxy by default.
	// If GRPC proxy is used then request HTTP headers set to outgoing request metadata.
	HTTPHeaders []string
	// GRPCMetadata is a list of GRPC metadata keys to proxy. No meta keys used by proxy by
	// default. If HTTP proxy is used then these keys become outgoing request HTTP headers.
	GRPCMetadata []string

	// BinaryEncoding makes proxy send data as base64 string (assuming it contains custom
	// non-JSON payload).
	BinaryEncoding bool

	// IncludeConnectionMeta to each proxy request (except connect where it's obtained).
	IncludeConnectionMeta bool

	// ConnectEndpoint - HTTP address or GRPC service url.
	ConnectEndpoint string
	// ConnectTimeout for ConnectEndpoint.
	ConnectTimeout time.Duration

	// RefreshEndpoint - HTTP address or GRPC service url.
	RefreshEndpoint string
	// RefreshTimeout for RefreshEndpoint.
	RefreshTimeout time.Duration

	// RPCEndpoint - HTTP address or GRPC service url.
	RPCEndpoint string
	// RPCTimeout for RPCEndpoint.
	RPCTimeout time.Duration

	// SubscribeEndpoint - HTTP address or GRPC service url.
	SubscribeEndpoint string
	// SubscribeTimeout for SubscribeEndpoint.
	SubscribeTimeout time.Duration

	// PublishEndpoint - HTTP address or GRPC service url.
	PublishEndpoint string
	// PublishTimeout for PublishEndpoint.
	PublishTimeout time.Duration

	// GRPCConfig is a GRPC specific configuration.
	GRPCConfig GRPCConfig

	// HTTPConfig is a HTTP specific configuration.
	HTTPConfig HTTPConfig
}

type GRPCConfig struct {
	CertFile         string
	CredentialsKey   string
	CredentialsValue string
	Codec            encoding.Codec
}

type HTTPConfig struct {
	Encoder proxyproto.RequestEncoder
	Decoder proxyproto.ResponseDecoder
}
