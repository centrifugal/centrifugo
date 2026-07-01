package configtypes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"reflect"
	"strings"
)

// HTTPServer configuration.
type HTTPServer struct {
	// Address to bind HTTP server to.
	Address string `mapstructure:"address" json:"address" envconfig:"address" toml:"address" yaml:"address" expose:"full" doc:"Address (host) to bind the HTTP server to. Empty string means all interfaces."`
	// Port to bind HTTP server to.
	Port int `mapstructure:"port" json:"port" envconfig:"port" default:"8000" toml:"port" yaml:"port" doc:"Port to bind the HTTP server to. Default <<8000>>."`
	// InternalAddress to bind internal HTTP server to. Internal server is used to serve endpoints
	// which are normally should not be exposed to the outside world.
	InternalAddress string `mapstructure:"internal_address" json:"internal_address" envconfig:"internal_address" toml:"internal_address" yaml:"internal_address" expose:"full" doc:"Address (host) to bind the internal HTTP server to. Internal endpoints (metrics, health, etc.) are served on this address."`
	// InternalPort to bind internal HTTP server to.
	InternalPort string `mapstructure:"internal_port" json:"internal_port" envconfig:"internal_port" toml:"internal_port" yaml:"internal_port" expose:"full" doc:"Port to bind the internal HTTP server to. When set, internal endpoints are served on this separate port."`
	// TLS configuration for HTTP server.
	TLS TLSConfig `mapstructure:"tls" json:"tls" envconfig:"tls" toml:"tls" yaml:"tls" doc:"TLS configuration for the external HTTP server."`
	// TLSAutocert for automatic TLS certificates from ACME provider (ex. Let's Encrypt).
	TLSAutocert TLSAutocert `mapstructure:"tls_autocert" json:"tls_autocert" envconfig:"tls_autocert" toml:"tls_autocert" yaml:"tls_autocert" doc:"Automatic TLS certificate configuration via ACME provider (e.g. Let's Encrypt)."`
	// TLSExternal enables TLS only for external HTTP endpoints.
	TLSExternal bool `mapstructure:"tls_external" json:"tls_external" envconfig:"tls_external" toml:"tls_external" yaml:"tls_external" doc:"When <<true>>, TLS is applied only to external HTTP endpoints, leaving internal endpoints unencrypted."`
	// InternalTLS is a custom configuration for internal HTTP endpoints. If not set InternalTLS will be the same as TLS.
	InternalTLS TLSConfig `mapstructure:"internal_tls" json:"internal_tls" envconfig:"internal_tls" toml:"internal_tls" yaml:"internal_tls" doc:"Custom TLS configuration for internal HTTP endpoints. If not set, the main TLS configuration is used for internal endpoints too."`
	// HTTP3 allows enabling HTTP/3 support. EXPERIMENTAL.
	HTTP3 HTTP3 `mapstructure:"http3" json:"http3" envconfig:"http3" toml:"http3" yaml:"http3" doc:"HTTP/3 (QUIC) configuration. Experimental."`
	// H2CExternal allows enabling HTTP/2 CLEARTEXT for external HTTP endpoints. Note, that to enable this option
	// custom separate InternalPort must be used.
	H2CExternal bool `mapstructure:"h2c_external" json:"h2c_external" envconfig:"h2c_external" toml:"h2c_external" yaml:"h2c_external" doc:"Enables HTTP/2 cleartext (h2c) for external HTTP endpoints. Requires a separate internal_port to be configured."`
	// ExternalHTTP2ExtendedConnect enables HTTP/2 Extended CONNECT support for HTTP server.
	// Commented for now because Go only has global toggle using GODEBUG=http2xconnect=1.
	// But ideally we would like per-server setting in the future. See https://github.com/golang/go/issues/53208.
	// Per endpoint is not possible because ALPN happens before routing.
	// ExternalHTTP2ExtendedConnect bool `mapstructure:"external_http2_extended_connect" json:"external_http2_extended_connect" envconfig:"external_http2_extended_connect" yaml:"external_http2_extended_connect" toml:"external_http2_extended_connect"`
}

// Log configuration.
type Log struct {
	// Level is a log level for Centrifugo logger. Supported values: `none`, `trace`, `debug`, `info`, `warn`, `error`.
	Level string `mapstructure:"level" default:"info" json:"level" envconfig:"level" toml:"level" yaml:"level" expose:"full" doc:"Log level. Possible values: <<none>>, <<trace>>, <<debug>>, <<info>>, <<warn>>, <<error>>. Default <<info>>."`
	// File is a path to log file. If not set logs go to stdout.
	File string `mapstructure:"file" json:"file" envconfig:"file" toml:"file" yaml:"file" expose:"full" doc:"Path to log file. When not set, logs are written to stdout."`
}

// Token common configuration.
type Token struct {
	HMACSecretKey                       string `mapstructure:"hmac_secret_key" json:"hmac_secret_key" envconfig:"hmac_secret_key" yaml:"hmac_secret_key" toml:"hmac_secret_key" doc:"HMAC secret key used to verify HS256/HS384/HS512 signed tokens."`
	HMACPreviousSecretKey               string `mapstructure:"hmac_previous_secret_key" json:"hmac_previous_secret_key" envconfig:"hmac_previous_secret_key" yaml:"hmac_previous_secret_key" toml:"hmac_previous_secret_key" doc:"Previous HMAC secret key, accepted during key rotation alongside the current hmac_secret_key."`
	HMACPreviousSecretKeyValidUntil     int64  `mapstructure:"hmac_previous_secret_key_valid_until" json:"hmac_previous_secret_key_valid_until" envconfig:"hmac_previous_secret_key_valid_until" yaml:"hmac_previous_secret_key_valid_until" toml:"hmac_previous_secret_key_valid_until" doc:"Unix timestamp until which the previous HMAC secret key remains valid. Tokens signed with the previous key are rejected after this time."`
	RSAPublicKey                        string `mapstructure:"rsa_public_key" json:"rsa_public_key" envconfig:"rsa_public_key" yaml:"rsa_public_key" toml:"rsa_public_key" expose:"full" doc:"RSA public key (PEM) used to verify RS256/RS384/RS512 signed tokens."`
	ECDSAPublicKey                      string `mapstructure:"ecdsa_public_key" json:"ecdsa_public_key" envconfig:"ecdsa_public_key" yaml:"ecdsa_public_key" toml:"ecdsa_public_key" expose:"full" doc:"ECDSA public key (PEM) used to verify ES256/ES384/ES512 signed tokens."`
	JWKSPublicEndpoint                  string `mapstructure:"jwks_public_endpoint" json:"jwks_public_endpoint" envconfig:"jwks_public_endpoint" yaml:"jwks_public_endpoint" toml:"jwks_public_endpoint" expose:"full" doc:"URL of the JWKS endpoint used to fetch public keys for token verification."`
	Audience                            string `mapstructure:"audience" json:"audience" envconfig:"audience" yaml:"audience" toml:"audience" expose:"full" doc:"Expected JWT audience claim (<<aud>>) value. When set, tokens without a matching audience are rejected."`
	AudienceRegex                       string `mapstructure:"audience_regex" json:"audience_regex" envconfig:"audience_regex" yaml:"audience_regex" toml:"audience_regex" expose:"full" doc:"Regular expression that the JWT audience claim (<<aud>>) must match. Alternative to audience for more flexible matching."`
	Issuer                              string `mapstructure:"issuer" json:"issuer" envconfig:"issuer" yaml:"issuer" toml:"issuer" expose:"full" doc:"Expected JWT issuer claim (<<iss>>) value. When set, tokens without a matching issuer are rejected."`
	IssuerRegex                         string `mapstructure:"issuer_regex" json:"issuer_regex" envconfig:"issuer_regex" yaml:"issuer_regex" toml:"issuer_regex" expose:"full" doc:"Regular expression that the JWT issuer claim (<<iss>>) must match. Alternative to issuer for more flexible matching."`
	UserIDClaim                         string `mapstructure:"user_id_claim" json:"user_id_claim" envconfig:"user_id_claim" yaml:"user_id_claim" toml:"user_id_claim" expose:"full" doc:"JWT claim name to extract the user ID from. When empty, the standard <<sub>> claim is used."`
	InsecureSkipJWKSEndpointSafetyCheck bool   `mapstructure:"insecure_skip_jwks_endpoint_safety_check" json:"insecure_skip_jwks_endpoint_safety_check" envconfig:"insecure_skip_jwks_endpoint_safety_check" yaml:"insecure_skip_jwks_endpoint_safety_check" toml:"insecure_skip_jwks_endpoint_safety_check" doc:"Disables the safety check that prevents using a JWKS endpoint accessible from the internet without additional protection. Use only in trusted environments."`
}

// SubscriptionToken can be used to set custom configuration for subscription tokens.
type SubscriptionToken struct {
	// Enabled allows enabling separate configuration for subscription tokens.
	Enabled bool `mapstructure:"enabled" json:"enabled" envconfig:"enabled"  yaml:"enabled" toml:"enabled" doc:"Enables a separate token configuration for subscription tokens, distinct from the connection token configuration."`
	Token   `mapstructure:",squash" yaml:",inline"`
}

// SharedPoll contains configuration for shared poll subscriptions.
type SharedPoll struct {
	HMACSecretKey                   string `mapstructure:"hmac_secret_key" json:"hmac_secret_key" envconfig:"hmac_secret_key" yaml:"hmac_secret_key" toml:"hmac_secret_key" doc:"HMAC secret key used to verify shared poll subscription tokens."`
	HMACPreviousSecretKey           string `mapstructure:"hmac_previous_secret_key" json:"hmac_previous_secret_key" envconfig:"hmac_previous_secret_key" yaml:"hmac_previous_secret_key" toml:"hmac_previous_secret_key" doc:"Previous HMAC secret key for shared poll tokens, accepted during key rotation alongside the current hmac_secret_key."`
	HMACPreviousSecretKeyValidUntil int64  `mapstructure:"hmac_previous_secret_key_valid_until" json:"hmac_previous_secret_key_valid_until" envconfig:"hmac_previous_secret_key_valid_until" yaml:"hmac_previous_secret_key_valid_until" toml:"hmac_previous_secret_key_valid_until" doc:"Unix timestamp until which the previous shared poll HMAC secret key remains valid."`
	ConcurrencyLimit                int    `mapstructure:"concurrency_limit" json:"concurrency_limit" envconfig:"concurrency_limit" yaml:"concurrency_limit" toml:"concurrency_limit" doc:"Maximum number of concurrent shared poll subscription requests processed simultaneously. Zero means no limit."`
}

// HTTP3 is EXPERIMENTAL.
type HTTP3 struct {
	Enabled bool `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled" doc:"Enables HTTP/3 (QUIC) support. Experimental."`
}

// WebSocket client real-time transport configuration.
type WebSocket struct {
	Disabled           bool     `mapstructure:"disabled" json:"disabled" envconfig:"disabled" yaml:"disabled" toml:"disabled" doc:"Disables the WebSocket transport endpoint."`
	HandlerPrefix      string   `mapstructure:"handler_prefix" json:"handler_prefix" envconfig:"handler_prefix" default:"/connection/websocket" yaml:"handler_prefix" toml:"handler_prefix" expose:"full" doc:"URL prefix for the WebSocket handler. Default <</connection/websocket>>."`
	Compression        bool     `mapstructure:"compression" json:"compression" envconfig:"compression" yaml:"compression" toml:"compression" doc:"Enables permessage-deflate WebSocket compression. Adds CPU overhead."`
	CompressionMinSize int      `mapstructure:"compression_min_size" json:"compression_min_size" envconfig:"compression_min_size" yaml:"compression_min_size" toml:"compression_min_size" doc:"Minimum message size in bytes before compression is applied. Zero compresses all messages when compression is enabled."`
	CompressionLevel   int      `mapstructure:"compression_level" json:"compression_level" envconfig:"compression_level" default:"1" yaml:"compression_level" toml:"compression_level" doc:"Compression level for permessage-deflate (1=fastest, 9=best compression). Default <<1>>."`
	ReadBufferSize     int      `mapstructure:"read_buffer_size" json:"read_buffer_size" envconfig:"read_buffer_size" yaml:"read_buffer_size" toml:"read_buffer_size" doc:"WebSocket read buffer size in bytes. Zero uses the default gorilla/websocket buffer size."`
	UseWriteBufferPool bool     `mapstructure:"use_write_buffer_pool" json:"use_write_buffer_pool" envconfig:"use_write_buffer_pool" yaml:"use_write_buffer_pool" toml:"use_write_buffer_pool" doc:"Enables a buffer pool for WebSocket write operations to reduce allocations."`
	WriteBufferSize    int      `mapstructure:"write_buffer_size" json:"write_buffer_size" envconfig:"write_buffer_size" yaml:"write_buffer_size" toml:"write_buffer_size" doc:"WebSocket write buffer size in bytes. Zero uses the default gorilla/websocket buffer size."`
	WriteTimeout       Duration `mapstructure:"write_timeout" json:"write_timeout" envconfig:"write_timeout" default:"1000ms" yaml:"write_timeout" toml:"write_timeout" doc:"Timeout for a single write operation to a WebSocket connection. Default <<1000ms>>."`
	MessageSizeLimit   int      `mapstructure:"message_size_limit" json:"message_size_limit" envconfig:"message_size_limit" default:"65536" yaml:"message_size_limit" toml:"message_size_limit" doc:"Maximum allowed WebSocket message size in bytes. Default <<65536>>."`
	// DecompressedMessageSizeLimit bounds the size of a message after permessage-deflate decompression.
	// Only effective when Compression is enabled. message_size_limit alone bounds only the compressed bytes
	// received on the wire, so without this limit a small compressed frame could be inflated into a much
	// larger amount of memory (a "decompression bomb"). When zero, the limit is derived from
	// message_size_limit multiplied by the default multiplier (10).
	DecompressedMessageSizeLimit int `mapstructure:"decompressed_message_size_limit" json:"decompressed_message_size_limit" envconfig:"decompressed_message_size_limit" yaml:"decompressed_message_size_limit" toml:"decompressed_message_size_limit" doc:"Maximum allowed WebSocket message size in bytes after permessage-deflate decompression. Only used when compression is enabled. Zero derives the limit from message_size_limit times the default multiplier (10)."`
}

// SSE client real-time transport configuration.
type SSE struct {
	Enabled            bool   `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled" doc:"Enables the SSE (Server-Sent Events) bidirectional emulation transport."`
	HandlerPrefix      string `mapstructure:"handler_prefix" json:"handler_prefix" envconfig:"handler_prefix" default:"/connection/sse" yaml:"handler_prefix" toml:"handler_prefix" expose:"full" doc:"URL prefix for the SSE handler. Default <</connection/sse>>."`
	MaxRequestBodySize int    `mapstructure:"max_request_body_size" json:"max_request_body_size" envconfig:"max_request_body_size" default:"65536" yaml:"max_request_body_size" toml:"max_request_body_size" doc:"Maximum allowed request body size in bytes for SSE connect requests. Default <<65536>>."`
}

// HTTPStream client real-time transport configuration.
type HTTPStream struct {
	Enabled            bool   `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled" doc:"Enables the HTTP streaming bidirectional emulation transport."`
	HandlerPrefix      string `mapstructure:"handler_prefix" json:"handler_prefix" envconfig:"handler_prefix" default:"/connection/http_stream" yaml:"handler_prefix" toml:"handler_prefix" expose:"full" doc:"URL prefix for the HTTP streaming handler. Default <</connection/http_stream>>."`
	MaxRequestBodySize int    `mapstructure:"max_request_body_size" json:"max_request_body_size" envconfig:"max_request_body_size" default:"65536" yaml:"max_request_body_size" toml:"max_request_body_size" doc:"Maximum allowed request body size in bytes for HTTP stream connect requests. Default <<65536>>."`
}

// WebTransport client real-time transport configuration.
type WebTransport struct {
	Enabled          bool   `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled" doc:"Enables the WebTransport unidirectional transport."`
	HandlerPrefix    string `mapstructure:"handler_prefix" json:"handler_prefix" envconfig:"handler_prefix" default:"/connection/webtransport" yaml:"handler_prefix" toml:"handler_prefix" expose:"full" doc:"URL prefix for the WebTransport handler. Default <</connection/webtransport>>."`
	MessageSizeLimit int    `mapstructure:"message_size_limit" json:"message_size_limit" envconfig:"message_size_limit" default:"65536" yaml:"message_size_limit" toml:"message_size_limit" doc:"Maximum allowed WebTransport message size in bytes. Default <<65536>>."`
}

// UniWebSocket client real-time transport configuration.
type UniWebSocket struct {
	Enabled            bool     `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled" doc:"Enables the unidirectional WebSocket transport."`
	HandlerPrefix      string   `mapstructure:"handler_prefix" json:"handler_prefix" envconfig:"handler_prefix" default:"/connection/uni_websocket" yaml:"handler_prefix" toml:"handler_prefix" expose:"full" doc:"URL prefix for the unidirectional WebSocket handler. Default <</connection/uni_websocket>>."`
	Compression        bool     `mapstructure:"compression" json:"compression" envconfig:"compression" yaml:"compression" toml:"compression" doc:"Enables permessage-deflate WebSocket compression for the unidirectional transport. Adds CPU overhead."`
	CompressionMinSize int      `mapstructure:"compression_min_size" json:"compression_min_size" envconfig:"compression_min_size" yaml:"compression_min_size" toml:"compression_min_size" doc:"Minimum message size in bytes before compression is applied on the unidirectional WebSocket transport."`
	CompressionLevel   int      `mapstructure:"compression_level" json:"compression_level" envconfig:"compression_level" default:"1" yaml:"compression_level" toml:"compression_level" doc:"Compression level for the unidirectional WebSocket transport (1=fastest, 9=best). Default <<1>>."`
	ReadBufferSize     int      `mapstructure:"read_buffer_size" json:"read_buffer_size" envconfig:"read_buffer_size" yaml:"read_buffer_size" toml:"read_buffer_size" doc:"Read buffer size in bytes for unidirectional WebSocket connections. Zero uses the default buffer size."`
	UseWriteBufferPool bool     `mapstructure:"use_write_buffer_pool" json:"use_write_buffer_pool" envconfig:"use_write_buffer_pool" yaml:"use_write_buffer_pool" toml:"use_write_buffer_pool" doc:"Enables a buffer pool for write operations on the unidirectional WebSocket transport to reduce allocations."`
	WriteBufferSize    int      `mapstructure:"write_buffer_size" json:"write_buffer_size" envconfig:"write_buffer_size" yaml:"write_buffer_size" toml:"write_buffer_size" doc:"Write buffer size in bytes for unidirectional WebSocket connections. Zero uses the default buffer size."`
	WriteTimeout       Duration `mapstructure:"write_timeout" json:"write_timeout" envconfig:"write_timeout" default:"1000ms" yaml:"write_timeout" toml:"write_timeout" doc:"Timeout for a single write operation on the unidirectional WebSocket transport. Default <<1000ms>>."`
	MessageSizeLimit   int      `mapstructure:"message_size_limit" json:"message_size_limit" envconfig:"message_size_limit" default:"65536" yaml:"message_size_limit" toml:"message_size_limit" doc:"Maximum allowed message size in bytes for the unidirectional WebSocket transport. Default <<65536>>."`
	// DecompressedMessageSizeLimit bounds the size of a message after permessage-deflate decompression.
	// Only effective when Compression is enabled. message_size_limit alone bounds only the compressed bytes
	// received on the wire, so without this limit a small compressed frame could be inflated into a much
	// larger amount of memory (a "decompression bomb"). When zero, the limit is derived from
	// message_size_limit multiplied by DefaultWebsocketDecompressedMessageSizeLimitMultiplier.
	DecompressedMessageSizeLimit int `mapstructure:"decompressed_message_size_limit" json:"decompressed_message_size_limit" envconfig:"decompressed_message_size_limit" yaml:"decompressed_message_size_limit" toml:"decompressed_message_size_limit" doc:"Maximum allowed message size in bytes after permessage-deflate decompression for the unidirectional WebSocket transport. Only used when compression is enabled. Zero derives the limit from message_size_limit times the default multiplier (10)."`
	// DisableClosingHandshake disables WebSocket closing handshake. This restores the behavior prior to
	// Centrifugo v6.5.1 where server never sent a close frame on connection close initiated by server.
	// Normally closing handshake is recommended to be performed according to WebSocket protocol RFC,
	// so this option is useful only in some specific cases when you need to restore the previous behavior.
	DisableClosingHandshake bool `mapstructure:"disable_closing_handshake" json:"disable_closing_handshake" envconfig:"disable_closing_handshake" yaml:"disable_closing_handshake" toml:"disable_closing_handshake" doc:"Disables the WebSocket closing handshake for the unidirectional transport. Restores pre-v6.5.1 behavior where the server never sent a close frame."`
	// DisableDisconnectPush disables sending disconnect push messages to clients. It's sent by default to make
	// unidirectional transports similar, but since uni_websocket transport also sends close frame to the client
	// with the same code/reason – some users may want to disable disconnect push to avoid ambiguity.
	DisableDisconnectPush bool `mapstructure:"disable_disconnect_push" json:"disable_disconnect_push" envconfig:"disable_disconnect_push" yaml:"disable_disconnect_push" toml:"disable_disconnect_push" doc:"Disables sending a disconnect push message before closing the unidirectional WebSocket connection, to avoid duplicating the close frame code and reason."`
	// JoinPushMessages when enabled allow uni_websocket transport to join messages together into
	// one frame using Centrifugal client protocol delimiters: new line for JSON protocol and
	// length-prefixed format for Protobuf protocol. This can be useful to reduce system call
	// overhead when sending many small messages. The client side must be ready to handle such
	// joined messages coming in one WebSocket frame.
	JoinPushMessages bool `mapstructure:"join_push_messages" json:"join_push_messages" envconfig:"join_push_messages" yaml:"join_push_messages" toml:"join_push_messages" doc:"Joins multiple push messages into a single WebSocket frame using protocol delimiters, reducing system call overhead when sending many small messages. The client must support joined frames."`
}

// UniHTTPStream client real-time transport configuration.
type UniHTTPStream struct {
	Enabled                   bool                      `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled" doc:"Enables the unidirectional HTTP streaming transport."`
	HandlerPrefix             string                    `mapstructure:"handler_prefix" json:"handler_prefix" envconfig:"handler_prefix" default:"/connection/uni_http_stream" yaml:"handler_prefix" toml:"handler_prefix" expose:"full" doc:"URL prefix for the unidirectional HTTP streaming handler. Default <</connection/uni_http_stream>>."`
	MaxRequestBodySize        int                       `mapstructure:"max_request_body_size" json:"max_request_body_size" envconfig:"max_request_body_size" default:"65536" yaml:"max_request_body_size" toml:"max_request_body_size" doc:"Maximum allowed request body size in bytes for unidirectional HTTP stream connect requests. Default <<65536>>."`
	ConnectCodeToHTTPResponse ConnectCodeToHTTPResponse `mapstructure:"connect_code_to_http_response" json:"connect_code_to_http_response" envconfig:"connect_code_to_http_response" yaml:"connect_code_to_http_response" toml:"connect_code_to_http_response" doc:"Configuration for mapping Centrifugo connect error codes to HTTP response status codes and body for the unidirectional HTTP stream transport."`
}

// UniSSE client real-time transport configuration.
type UniSSE struct {
	Enabled                   bool                      `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled" doc:"Enables the unidirectional SSE (Server-Sent Events) transport."`
	HandlerPrefix             string                    `mapstructure:"handler_prefix" json:"handler_prefix" envconfig:"handler_prefix" default:"/connection/uni_sse" yaml:"handler_prefix" toml:"handler_prefix" expose:"full" doc:"URL prefix for the unidirectional SSE handler. Default <</connection/uni_sse>>."`
	MaxRequestBodySize        int                       `mapstructure:"max_request_body_size" json:"max_request_body_size" envconfig:"max_request_body_size" default:"65536" yaml:"max_request_body_size" toml:"max_request_body_size" doc:"Maximum allowed request body size in bytes for unidirectional SSE connect requests. Default <<65536>>."`
	ConnectCodeToHTTPResponse ConnectCodeToHTTPResponse `mapstructure:"connect_code_to_http_response" json:"connect_code_to_http_response" envconfig:"connect_code_to_http_response" yaml:"connect_code_to_http_response" toml:"connect_code_to_http_response" doc:"Configuration for mapping Centrifugo connect error codes to HTTP response status codes and body for the unidirectional SSE transport."`
}

type ConnInit struct {
	Enabled       bool   `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled" doc:"Enables the connection init endpoint used for connection initialization without transport establishment."`
	HandlerPrefix string `mapstructure:"handler_prefix" json:"handler_prefix" envconfig:"handler_prefix" default:"/connection/init" yaml:"handler_prefix" toml:"handler_prefix" expose:"full" doc:"URL prefix for the connection init handler. Default <</connection/init>>."`
}

type ConnectCodeToHTTPResponseTransforms []ConnectCodeToHTTPResponseTransform

// Decode to implement the envconfig.Decoder interface
func (d *ConnectCodeToHTTPResponseTransforms) Decode(value string) error {
	// If the source is a string and the target is a slice, try to parse it as JSON.
	var items ConnectCodeToHTTPResponseTransforms
	err := json.Unmarshal([]byte(value), &items)
	if err != nil {
		return fmt.Errorf("error parsing items from JSON: %v", err)
	}
	*d = items
	return nil
}

type ConnectCodeToHTTPResponse struct {
	Enabled    bool                                `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled" doc:"Enables mapping of Centrifugo connect error codes to custom HTTP response status codes and body."`
	Transforms ConnectCodeToHTTPResponseTransforms `mapstructure:"transforms" default:"[]" json:"transforms" envconfig:"transforms" yaml:"transforms" toml:"transforms" doc:"List of connect error code to HTTP response transformations. Default <<[]>>."`
}

type ConnectCodeToHTTPResponseTransform struct {
	Code uint32                              `mapstructure:"code" json:"code" envconfig:"code" yaml:"code" toml:"code" doc:"Centrifugo connect error code to match for this transformation."`
	To   TransformedConnectErrorHttpResponse `mapstructure:"to" json:"to" envconfig:"to" yaml:"to" toml:"to" doc:"Target HTTP response (status code and body) to return when the connect error code matches."`
}

type TransformedConnectErrorHttpResponse struct {
	StatusCode int    `mapstructure:"status_code" json:"status_code" envconfig:"status_code" yaml:"status_code" toml:"status_code" doc:"HTTP status code to use in the response when the connect error is matched."`
	Body       string `mapstructure:"body" json:"body" envconfig:"body" yaml:"body" toml:"body" expose:"full" doc:"HTTP response body to use when the connect error is matched."`
}

// UniGRPC client real-time transport configuration.
type UniGRPC struct {
	Enabled               bool      `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled" doc:"Enables the unidirectional gRPC transport."`
	Address               string    `mapstructure:"address" json:"address" envconfig:"address" yaml:"address" toml:"address" expose:"full" doc:"Address (host) to bind the unidirectional gRPC server to."`
	Port                  int       `mapstructure:"port" json:"port" envconfig:"port" default:"11000" yaml:"port" toml:"port" doc:"Port to bind the unidirectional gRPC server to. Default <<11000>>."`
	MaxReceiveMessageSize int       `mapstructure:"max_receive_message_size" json:"max_receive_message_size" envconfig:"max_receive_message_size" yaml:"max_receive_message_size" toml:"max_receive_message_size" doc:"Maximum allowed gRPC message size in bytes for the unidirectional gRPC transport. Zero uses the gRPC default."`
	TLS                   TLSConfig `mapstructure:"tls" json:"tls" envconfig:"tls" yaml:"tls" toml:"tls" doc:"TLS configuration for the unidirectional gRPC server."`
}

// PingPong allows configuring application level ping-pong behavior.
// Note that in current implementation PingPongConfig.PingInterval must be greater than PingPongConfig.PongTimeout.
type PingPong struct {
	// PingInterval tells how often to issue server-to-client pings.
	// To disable sending app-level pings use -1.
	PingInterval Duration `mapstructure:"ping_interval" json:"ping_interval" envconfig:"ping_interval" default:"25s" yaml:"ping_interval" toml:"ping_interval" doc:"Interval between server-to-client application-level pings. Use <<-1>> to disable. Default <<25s>>."`
	// PongTimeout sets time for pong check after issuing a ping. To disable pong checks use -1.
	// PongTimeout must be less than PingInterval in current implementation.
	PongTimeout Duration `mapstructure:"pong_timeout" json:"pong_timeout" envconfig:"pong_timeout" default:"8s" yaml:"pong_timeout" toml:"pong_timeout" doc:"Time to wait for a pong response after sending a ping before disconnecting. Use <<-1>> to disable pong checks. Default <<8s>>."`
}

type NatsPrefixed struct {
	// Prefix allows customizing channel prefix in Nats to work with a single Nats from different
	// unrelated Centrifugo setups.
	Prefix string `mapstructure:"prefix" default:"centrifugo" json:"prefix" envconfig:"prefix" yaml:"prefix" toml:"prefix" expose:"full" doc:"Prefix added to all NATS channel names to namespace them and avoid collisions between unrelated Centrifugo setups. Default <<centrifugo>>."`
	// NatsCommon contains common Nats configuration.
	NatsCommon `mapstructure:",squash" yaml:",inline"`
}

// NatsEmptyPrefixed is like NatsPrefixed but defaults to an empty prefix.
// Useful for standalone pub/sub channels where a prefix is not needed by default.
type NatsEmptyPrefixed struct {
	Prefix     string `mapstructure:"prefix" json:"prefix" envconfig:"prefix" yaml:"prefix" toml:"prefix" expose:"full" doc:"Optional prefix added to all NATS channel names. Defaults to empty string (no prefix)."`
	NatsCommon `mapstructure:",squash" yaml:",inline"`
}

type NatsCommon struct {
	// URL is a Nats server URL.
	URL string `mapstructure:"url" json:"url" envconfig:"url" yaml:"url" toml:"url" default:"nats://localhost:4222" expose:"url" doc:"NATS server URL. Default <<nats://localhost:4222>>."`
	// DialTimeout is a timeout for establishing connection to Nats.
	DialTimeout Duration `mapstructure:"dial_timeout" default:"1s" json:"dial_timeout" envconfig:"dial_timeout" yaml:"dial_timeout" toml:"dial_timeout" doc:"Timeout for establishing a connection to the NATS server. Default <<1s>>."`
	// WriteTimeout is a timeout for write operation to Nats.
	WriteTimeout Duration `mapstructure:"write_timeout" default:"1s" json:"write_timeout" envconfig:"write_timeout" yaml:"write_timeout" toml:"write_timeout" doc:"Timeout for write operations to NATS. Default <<1s>>."`
	// TLS for the Nats connection. TLS is not used if nil.
	TLS TLSConfig `mapstructure:"tls" json:"tls" envconfig:"tls" yaml:"tls" toml:"tls" doc:"TLS configuration for the NATS connection."`
}

// NatsBroker configuration.
type NatsBroker struct {
	NatsPrefixed `mapstructure:",squash" yaml:",inline"`

	// AllowWildcards allows to enable wildcard subscriptions. By default, wildcard subscriptions
	// are not allowed. Using wildcard subscriptions can't be combined with join/leave events and presence
	// because subscriptions do not belong to a concrete channel after with wildcards, while join/leave events
	// require concrete channel to be published. And presence does not make a lot of sense for wildcard
	// subscriptions - there could be subscribers which use different mask, but still receive subset of updates.
	// It's required to use channels without wildcards to for mentioned features to work properly. When
	// using wildcard subscriptions a special care is needed regarding security - pay additional
	// attention to a proper permission management.
	AllowWildcards bool `mapstructure:"allow_wildcards" json:"allow_wildcards" envconfig:"allow_wildcards" yaml:"allow_wildcards" toml:"allow_wildcards" doc:"Enables wildcard channel subscriptions in NATS. Cannot be combined with join/leave events or presence features."`

	// RawMode allows enabling raw communication with Nats. When on, Centrifugo subscribes to channels
	// without adding any prefixes to channel name. Proper prefixes must be managed by the application in this
	// case. Data consumed from Nats is sent directly to subscribers without any processing. When publishing
	// to Nats Centrifugo does not add any prefixes to channel names also. Centrifugo features like Publication
	// tags, Publication ClientInfo, join/leave events are not supported in raw mode.
	RawMode RawModeConfig `mapstructure:"raw_mode" json:"raw_mode" envconfig:"raw_mode" yaml:"raw_mode" toml:"raw_mode" doc:"Raw NATS mode configuration — when enabled, Centrifugo communicates with NATS without any channel prefixes or message processing."`
}

type RawModeConfig struct {
	// Enabled enables raw mode when true.
	Enabled bool `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled" doc:"Enables raw NATS mode where Centrifugo does not add any prefixes to channel names and forwards data directly to subscribers."`

	// ChannelReplacements is a map where keys are strings to replace and values are replacements.
	// For example, you have Centrifugo namespace "chat" and using channel "chat:index", but you want to
	// use channel "chat.index" in Nats. Then you can define SymbolReplacements map like this: `{":": "."}`.
	// In this case Centrifugo will replace all ":" symbols in channel name with "." before sending to Nats.
	// Broker keeps reverse mapping to the original channel to broadcast to proper channels when processing
	// messages received from Nats.
	ChannelReplacements MapStringString `mapstructure:"channel_replacements" default:"{}" json:"channel_replacements" envconfig:"channel_replacements" yaml:"channel_replacements" toml:"channel_replacements" doc:"Map of symbol replacements applied to channel names before passing them to NATS (e.g. replacing <<:>> with <<.>>). Default <<{}>>."`

	// Prefix is a string that will be added to all channels when publishing messages to Nats, subscribing
	// to channels in Nats. It's also stripped from channel name when processing messages received from Nats.
	// By default, no prefix is used.
	Prefix string `mapstructure:"prefix" json:"prefix" envconfig:"prefix" yaml:"prefix" toml:"prefix" expose:"full" doc:"Prefix added to all NATS channel names in raw mode. Stripped from channel names when processing incoming NATS messages."`
}

type OpenTelemetry struct {
	Enabled   bool `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled" doc:"Enables OpenTelemetry tracing and metrics export."`
	API       bool `mapstructure:"api" json:"api" envconfig:"api" yaml:"api" toml:"api" doc:"Enables OpenTelemetry instrumentation for HTTP/gRPC API requests."`
	Consuming bool `mapstructure:"consuming" json:"consuming" envconfig:"consuming" yaml:"consuming" toml:"consuming" doc:"Enables OpenTelemetry instrumentation for consumer processing."`
	// GoogleCloudADCAuth, when true, authenticates the OTLP exporter with
	// Google Cloud Application Default Credentials (ADC). This allows exporting
	// directly to Google Cloud's OTLP endpoint (telemetry.googleapis.com)
	// without a sidecar collector. Works with both exporter protocols: over
	// grpc the ADC token is attached as a per-RPC credential, over http/protobuf
	// via an OAuth2 http.Client transport; in both cases the token refreshes
	// automatically. The endpoint and target project are still configured via
	// the standard OTEL_EXPORTER_OTLP_* environment variables (e.g.
	// OTEL_EXPORTER_OTLP_ENDPOINT=https://telemetry.googleapis.com and
	// OTEL_RESOURCE_ATTRIBUTES=gcp.project_id=PROJECT_ID).
	GoogleCloudADCAuth bool `mapstructure:"google_cloud_adc_auth" json:"google_cloud_adc_auth" envconfig:"google_cloud_adc_auth" yaml:"google_cloud_adc_auth" toml:"google_cloud_adc_auth" doc:"Enables Google Cloud Application Default Credentials (ADC) authentication for the OTLP exporter, allowing direct export to Google Cloud without a sidecar collector."`
}

type HttpAPI struct {
	Disabled      bool   `mapstructure:"disabled" json:"disabled" envconfig:"disabled" yaml:"disabled" toml:"disabled" doc:"Disables the HTTP API endpoint."`
	HandlerPrefix string `mapstructure:"handler_prefix" json:"handler_prefix" envconfig:"handler_prefix" default:"/api" yaml:"handler_prefix" toml:"handler_prefix" expose:"full" doc:"URL prefix for the HTTP API handler. Default <</api>>."`
	Key           string `mapstructure:"key" json:"key" envconfig:"key" yaml:"key" toml:"key" doc:"API key for HTTP API authentication. When set, requests must include this key."`
	ErrorMode     string `mapstructure:"error_mode" json:"error_mode" envconfig:"error_mode" yaml:"error_mode" toml:"error_mode" expose:"full" doc:"Controls error response format for the HTTP API. Possible values: <<transport>> (default), <<custom>>."`
	External      bool   `mapstructure:"external" json:"external" envconfig:"external" yaml:"external" toml:"external" doc:"Runs the HTTP API on the external port instead of the internal port."`
	Insecure      bool   `mapstructure:"insecure" json:"insecure" envconfig:"insecure" yaml:"insecure" toml:"insecure" doc:"Disables API key authentication for the HTTP API. Use only in trusted network environments."`
}

type GrpcAPI struct {
	Enabled               bool      `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled" doc:"Enables the gRPC API server."`
	ErrorMode             string    `mapstructure:"error_mode" json:"error_mode" envconfig:"error_mode" yaml:"error_mode" toml:"error_mode" expose:"full" doc:"Controls error response format for the gRPC API. Possible values: <<transport>> (default), <<custom>>."`
	Address               string    `mapstructure:"address" json:"address" envconfig:"address" yaml:"address" toml:"address" expose:"full" doc:"Address (host) to bind the gRPC API server to."`
	Port                  int       `mapstructure:"port" json:"port" envconfig:"port" default:"10000" yaml:"port" toml:"port" doc:"Port to bind the gRPC API server to. Default <<10000>>."`
	Key                   string    `mapstructure:"key" json:"key" envconfig:"key" yaml:"key" toml:"key" doc:"API key required in gRPC metadata for authentication."`
	TLS                   TLSConfig `mapstructure:"tls" json:"tls" envconfig:"tls" yaml:"tls" toml:"tls" doc:"TLS configuration for the gRPC API server."`
	Reflection            bool      `mapstructure:"reflection" json:"reflection" envconfig:"reflection" yaml:"reflection" toml:"reflection" doc:"Enables gRPC server reflection, allowing tools like grpcurl to discover available methods."`
	MaxReceiveMessageSize int       `mapstructure:"max_receive_message_size" json:"max_receive_message_size" envconfig:"max_receive_message_size" yaml:"max_receive_message_size" toml:"max_receive_message_size" doc:"Maximum size of a message received by the gRPC API server, in bytes. Zero uses the gRPC default."`
}

type Graphite struct {
	Enabled  bool     `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled" doc:"Enables Graphite metrics export."`
	Host     string   `mapstructure:"host" json:"host" envconfig:"host" default:"localhost" yaml:"host" toml:"host" expose:"full" doc:"Graphite server host. Default <<localhost>>."`
	Port     int      `mapstructure:"port" json:"port" envconfig:"port" default:"2003" yaml:"port" toml:"port" doc:"Graphite server port. Default <<2003>>."`
	Prefix   string   `mapstructure:"prefix" json:"prefix" envconfig:"prefix" default:"centrifugo" yaml:"prefix" toml:"prefix" expose:"full" doc:"Prefix for all Graphite metric names. Default <<centrifugo>>."`
	Interval Duration `mapstructure:"interval" json:"interval" envconfig:"interval" default:"10s" yaml:"interval" toml:"interval" doc:"Interval between metric flushes to Graphite. Default <<10s>>."`
	Tags     bool     `mapstructure:"tags" json:"tags" envconfig:"tags" yaml:"tags" toml:"tags" doc:"Enables tagged Graphite metrics format."`
}

type Emulation struct {
	HandlerPrefix      string `mapstructure:"handler_prefix" json:"handler_prefix" envconfig:"handler_prefix" default:"/emulation" yaml:"handler_prefix" toml:"handler_prefix" expose:"full" doc:"URL prefix for the client emulation handler. Default <</emulation>>."`
	MaxRequestBodySize int    `mapstructure:"max_request_body_size" json:"max_request_body_size" envconfig:"max_request_body_size" default:"65536" yaml:"max_request_body_size" toml:"max_request_body_size" doc:"Maximum request body size in bytes for emulation requests. Default <<65536>>."`
}

type UsageStats struct {
	Disabled bool `mapstructure:"disabled" json:"disabled" envconfig:"disabled" yaml:"disabled" toml:"disabled" doc:"Disables sending anonymous usage statistics to Centrifugo developers."`
}

type Prometheus struct {
	Enabled       bool   `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled" doc:"Enables the Prometheus metrics endpoint."`
	HandlerPrefix string `mapstructure:"handler_prefix" json:"handler_prefix" envconfig:"handler_prefix" default:"/metrics" yaml:"handler_prefix" toml:"handler_prefix" expose:"full" doc:"URL prefix for the Prometheus metrics handler. Default <</metrics>>."`
	// InstrumentHTTPHandlers enables additional instrumentation of HTTP handlers
	// (extra middleware to track status codes). Optional since adds some overhead.
	InstrumentHTTPHandlers bool `mapstructure:"instrument_http_handlers" json:"instrument_http_handlers" envconfig:"instrument_http_handlers" yaml:"instrument_http_handlers" toml:"instrument_http_handlers" doc:"Enables extra Prometheus instrumentation of HTTP handlers to track response status codes. Adds some overhead."`
	// RecoveredPublicationsHistogram enables a histogram to track the distribution of recovered publications number.
	RecoveredPublicationsHistogram bool `mapstructure:"recovered_publications_histogram" json:"recovered_publications_histogram" envconfig:"recovered_publications_histogram" yaml:"recovered_publications_histogram" toml:"recovered_publications_histogram" doc:"Enables a Prometheus histogram tracking the number of publications recovered per client recovery request."`
	// NativeHistograms switches Centrifugo's Histogram instruments and the
	// underlying centrifuge library's distribution metrics to Prometheus
	// native (sparse, exponential) histogram schema with no explicit buckets
	// exposed. Designed for OpenTelemetry export via the client_golang
	// Prometheus bridge: native histograms map to OTel ExponentialHistogram.
	// Text-format scrapes lose _bucket series; only _count and _sum remain
	// visible — use protobuf scrape format (Prom 2.40+) to get full data.
	// Experimental in client_golang.
	NativeHistograms bool `mapstructure:"native_histograms" json:"native_histograms" envconfig:"native_histograms" yaml:"native_histograms" toml:"native_histograms" doc:"Switches histogram metrics to Prometheus native (sparse exponential) histogram schema. Requires protobuf scrape format (Prometheus 2.40+) to expose full bucket data. Experimental."`
}

type Health struct {
	Enabled       bool   `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled" doc:"Enables the health check endpoint."`
	HandlerPrefix string `mapstructure:"handler_prefix" json:"handler_prefix" envconfig:"handler_prefix" default:"/health" yaml:"handler_prefix" toml:"handler_prefix" expose:"full" doc:"URL prefix for the health check handler. Default <</health>>."`
}

type Swagger struct {
	Enabled       bool   `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled" doc:"Enables the Swagger UI endpoint for the HTTP API."`
	HandlerPrefix string `mapstructure:"handler_prefix" json:"handler_prefix" envconfig:"handler_prefix" default:"/swagger" yaml:"handler_prefix" toml:"handler_prefix" expose:"full" doc:"URL prefix for the Swagger UI handler. Default <</swagger>>."`
}

type Debug struct {
	Enabled       bool   `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled" doc:"Enables the pprof debug profiling endpoint."`
	HandlerPrefix string `mapstructure:"handler_prefix" json:"handler_prefix" envconfig:"handler_prefix" default:"/debug/pprof" yaml:"handler_prefix" toml:"handler_prefix" expose:"full" doc:"URL prefix for the pprof debug handler. Default <</debug/pprof>>."`
}

type Dev struct {
	Enabled       bool   `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled" expose:"-" doc:"Enables the development mode endpoint (e.g. real-time event log UI). This is an insecure endpoint which must be used only in development."`
	HandlerPrefix string `mapstructure:"handler_prefix" json:"handler_prefix" envconfig:"handler_prefix" default:"/dev" yaml:"handler_prefix" toml:"handler_prefix" expose:"full" doc:"URL prefix for the dev mode handler. Default <</dev>>."`
}

type Shutdown struct {
	Timeout Duration `mapstructure:"timeout" json:"timeout" envconfig:"timeout" default:"30s" yaml:"timeout" toml:"timeout" doc:"Maximum time to wait for graceful shutdown before forcefully stopping. Default <<30s>>."`
}

type ConnectProxy struct {
	// Enabled must be true to tell Centrifugo to enable the configured proxy.
	Enabled bool `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled" doc:"Enables the connect proxy. When enabled, connect requests without a JWT token are proxied to the backend."`
	Proxy   `mapstructure:",squash" yaml:",inline"`
}

type RefreshProxy struct {
	// Enabled must be true to tell Centrifugo to enable the configured proxy.
	Enabled bool `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled" doc:"Enables the refresh proxy. When enabled, connection refresh decisions are proxied to the backend."`
	Proxy   `mapstructure:",squash" yaml:",inline"`
}

type ClientProxyContainer struct {
	// Connect proxy when enabled is used to proxy connect requests from client to the application backend.
	// Only requests without JWT token are proxied at this point.
	Connect ConnectProxy `mapstructure:"connect" json:"connect" envconfig:"connect" yaml:"connect" toml:"connect" doc:"Connect proxy configuration. When enabled, unauthenticated connect requests are forwarded to the backend."`
	// Refresh proxy when enabled is used to proxy client connection refresh decisions to the application backend.
	Refresh RefreshProxy `mapstructure:"refresh" json:"refresh" envconfig:"refresh" yaml:"refresh" toml:"refresh" doc:"Refresh proxy configuration. When enabled, connection token refresh requests are forwarded to the backend."`
}

type Client struct {
	// Proxy is a configuration for client connection-wide proxies.
	Proxy ClientProxyContainer `mapstructure:"proxy" json:"proxy" envconfig:"proxy" toml:"proxy" yaml:"proxy" doc:"Configuration for client connection-level proxies (connect and refresh)."`
	// AllowedOrigins is a list of allowed origins for client connections.
	AllowedOrigins []string `mapstructure:"allowed_origins" json:"allowed_origins" envconfig:"allowed_origins" yaml:"allowed_origins" toml:"allowed_origins" expose:"full" doc:"List of allowed WebSocket origins (CORS). Use <<*>> to allow all origins."`
	// Token is a configuration for token generation and verification. When enabled, this configuration
	// is used for both connection and subscription tokens. See also SubscriptionToken to use a separate
	// configuration for subscription tokens.
	Token Token `mapstructure:"token" json:"token" envconfig:"token" yaml:"token" toml:"token" doc:"JWT token verification configuration used for both connection and subscription tokens (unless subscription_token is separately enabled)."`
	// SubscriptionToken is a configuration for subscription token generation and verification. When enabled,
	// Centrifugo will use this configuration for subscription tokens only. Configuration in Token is then only
	// used for connection tokens.
	SubscriptionToken SubscriptionToken `mapstructure:"subscription_token" json:"subscription_token" envconfig:"subscription_token" yaml:"subscription_token" toml:"subscription_token" doc:"Separate JWT token verification configuration for subscription tokens. When enabled, connection tokens still use the main token configuration."`
	// AllowAnonymousConnectWithoutToken allows to connect to Centrifugo without a token. In this case connection will
	// be accepted but client will be anonymous (i.e. will have empty user ID).
	AllowAnonymousConnectWithoutToken bool `mapstructure:"allow_anonymous_connect_without_token" json:"allow_anonymous_connect_without_token" envconfig:"allow_anonymous_connect_without_token" yaml:"allow_anonymous_connect_without_token" toml:"allow_anonymous_connect_without_token" doc:"Allows clients to connect without a token. Such connections will be anonymous (empty user ID)."`
	// DisallowAnonymousConnectionTokens disallows anonymous connection tokens. When enabled, Centrifugo will not
	// accept connection tokens with empty user ID.
	DisallowAnonymousConnectionTokens bool `mapstructure:"disallow_anonymous_connection_tokens" json:"disallow_anonymous_connection_tokens" envconfig:"disallow_anonymous_connection_tokens" yaml:"disallow_anonymous_connection_tokens" toml:"disallow_anonymous_connection_tokens" doc:"Rejects JWT connection tokens with an empty user ID claim. Useful to enforce authenticated connections only when tokens are in use."`
	// PingPong allows configuring application level ping-pong behavior for client connections.
	PingPong `mapstructure:",squash" yaml:",inline"`
	// ExpiredConnectionCloseDelay is a delay before closing connection after it becomes expired.
	ExpiredCloseDelay Duration `mapstructure:"expired_close_delay" json:"expired_close_delay" envconfig:"expired_close_delay" default:"25s" yaml:"expired_close_delay" toml:"expired_close_delay" doc:"Delay before closing an expired client connection. Default <<25s>>."`
	// ExpiredSubCloseDelay is a delay before closing subscription after it becomes expired.
	ExpiredSubCloseDelay Duration `mapstructure:"expired_sub_close_delay" json:"expired_sub_close_delay" envconfig:"expired_sub_close_delay" default:"25s" yaml:"expired_sub_close_delay" toml:"expired_sub_close_delay" doc:"Delay before closing an expired channel subscription. Default <<25s>>."`
	// StaleConnectionCloseDelay is a delay before closing stale connection (which does not authenticate after connecting).
	StaleCloseDelay Duration `mapstructure:"stale_close_delay" json:"stale_close_delay" envconfig:"stale_close_delay" default:"10s" yaml:"stale_close_delay" toml:"stale_close_delay" doc:"Delay before closing a stale connection that has not authenticated after connecting. Default <<10s>>."`
	// ChannelLimit is a maximum number of channels client can subscribe to.
	ChannelLimit int `mapstructure:"channel_limit" json:"channel_limit" envconfig:"channel_limit" default:"128" yaml:"channel_limit" toml:"channel_limit" doc:"Maximum number of channels a single client can subscribe to simultaneously. Default <<128>>."`
	// QueueMaxSize is a maximum size of message queue for client connection in bytes.
	QueueMaxSize int `mapstructure:"queue_max_size" json:"queue_max_size" envconfig:"queue_max_size" default:"1048576" yaml:"queue_max_size" toml:"queue_max_size" doc:"Maximum size of the outgoing message queue per client connection in bytes. Default <<1048576>> (1 MB)."`
	// PresenceUpdateInterval is a period of time how often to update presence info for subscriptions of connected client.
	PresenceUpdateInterval Duration `mapstructure:"presence_update_interval" json:"presence_update_interval" envconfig:"presence_update_interval" default:"27s" yaml:"presence_update_interval" toml:"presence_update_interval" doc:"How often a client updates its presence information in subscribed channels. Default <<27s>>."`
	// Concurrency is a maximum number of concurrent operations for client connection. If not set only one operation can be processed at a time.
	Concurrency int `mapstructure:"concurrency" json:"concurrency" envconfig:"concurrency" yaml:"concurrency" toml:"concurrency" doc:"Maximum number of commands processed concurrently per client connection. Zero means one command at a time (sequential)."`
	// ChannelPositionCheckDelay is a delay between channel position checks for client subscriptions.
	ChannelPositionCheckDelay Duration `mapstructure:"channel_position_check_delay" json:"channel_position_check_delay" envconfig:"channel_position_check_delay" default:"40s" yaml:"channel_position_check_delay" toml:"channel_position_check_delay" doc:"Interval between stream position checks for subscriptions with positioning enabled. Default <<40s>>."`
	// ChannelPositionMaxTimeLag is a maximum allowed time lag for publications for subscribers with positioning on. When
	// exceeded we mark connection with insufficient state. By default, not used - i.e. Centrifugo does not take lag into
	// account for positioning. See pub_sub_time_lag_seconds as a helpful Prometheus metric.
	ChannelPositionMaxTimeLag Duration `mapstructure:"channel_position_max_time_lag" json:"channel_position_max_time_lag" envconfig:"channel_position_max_time_lag" yaml:"channel_position_max_time_lag" toml:"channel_position_max_time_lag" doc:"Maximum allowed PUB/SUB time lag for subscribers with positioning enabled. When exceeded, the subscriber is disconnected with insufficient state. Zero disables lag-based checks."`
	// ConnectionLimit is a maximum number of connections Centrifugo node can accept.
	ConnectionLimit int `mapstructure:"connection_limit" json:"connection_limit" envconfig:"connection_limit" yaml:"connection_limit" toml:"connection_limit" doc:"Maximum total number of simultaneous client connections this node will accept. Zero means no limit."`
	// UserConnectionLimit is a maximum number of connections Centrifugo node can accept from a single user.
	UserConnectionLimit int `mapstructure:"user_connection_limit" json:"user_connection_limit" envconfig:"user_connection_limit" yaml:"user_connection_limit" toml:"user_connection_limit" doc:"Maximum number of simultaneous connections allowed per user ID on this node. Zero means no limit."`
	// ConnectionRateLimit is a maximum number of connections per second Centrifugo node can accept.
	ConnectionRateLimit int `mapstructure:"connection_rate_limit" json:"connection_rate_limit" envconfig:"connection_rate_limit" yaml:"connection_rate_limit" toml:"connection_rate_limit" doc:"Maximum number of new client connections per second this node will accept. Zero means no rate limit."`
	// ConnectIncludeServerTime allows to include server time in connect reply of client protocol.
	ConnectIncludeServerTime bool `mapstructure:"connect_include_server_time" json:"connect_include_server_time" envconfig:"connect_include_server_time" yaml:"connect_include_server_time" toml:"connect_include_server_time" doc:"Includes the current server Unix time in the connect reply sent to clients."`
	// HistoryMaxPublicationLimit is a maximum number of publications Centrifugo returns in client history requests.
	HistoryMaxPublicationLimit int `mapstructure:"history_max_publication_limit" json:"history_max_publication_limit" envconfig:"history_max_publication_limit" default:"300" yaml:"history_max_publication_limit" toml:"history_max_publication_limit" doc:"Maximum number of publications returned per client history request. Default <<300>>."`
	// RecoveryMaxPublicationLimit is a maximum number of publications Centrifugo returns during client recovery.
	RecoveryMaxPublicationLimit int `mapstructure:"recovery_max_publication_limit" json:"recovery_max_publication_limit" envconfig:"recovery_max_publication_limit" default:"300" yaml:"recovery_max_publication_limit" toml:"recovery_max_publication_limit" doc:"Maximum number of publications returned during client message recovery. Default <<300>>."`
	// InsecureSkipTokenSignatureVerify allows to skip token signature verification. This can be useful for testing purposes.
	InsecureSkipTokenSignatureVerify bool `mapstructure:"insecure_skip_token_signature_verify" json:"insecure_skip_token_signature_verify" envconfig:"insecure_skip_token_signature_verify" yaml:"insecure_skip_token_signature_verify" toml:"insecure_skip_token_signature_verify" doc:"Skips JWT token signature verification. For testing only — never use in production."`
	// UserIDHTTPHeader is a name of HTTP header to extract user ID from. If set Centrifugo will try to extract user ID from this header.
	UserIDHTTPHeader string `mapstructure:"user_id_http_header" json:"user_id_http_header" envconfig:"user_id_http_header" yaml:"user_id_http_header" toml:"user_id_http_header" expose:"full" doc:"HTTP header name to extract the user ID from, as an alternative to JWT-based authentication."`
	// Insecure allows to disable auth features in client protocol. Obviously - must not be used in production until you know what you do.
	Insecure bool `mapstructure:"insecure" json:"insecure" envconfig:"insecure" yaml:"insecure" toml:"insecure" doc:"Disables authentication for client connections. All connections are accepted as anonymous. Do not use in production."`
	// SubscribeToUserPersonalChannel is a configuration for a feature to automatically subscribe user to a personal channel
	// using server-side subscription.
	SubscribeToUserPersonalChannel SubscribeToUserPersonalChannel `mapstructure:"subscribe_to_user_personal_channel" json:"subscribe_to_user_personal_channel" envconfig:"subscribe_to_user_personal_channel" yaml:"subscribe_to_user_personal_channel" toml:"subscribe_to_user_personal_channel" doc:"Configuration for automatically subscribing authenticated users to a personal channel on connect."`
	// ConnectCodeToUnidirectionalDisconnect is a configuration for a feature to transform connect error codes to the disconnect code
	// for unidirectional transports.
	ConnectCodeToUnidirectionalDisconnect ConnectCodeToUnidirectionalDisconnect `mapstructure:"connect_code_to_unidirectional_disconnect" json:"connect_code_to_unidirectional_disconnect" envconfig:"connect_code_to_unidirectional_disconnect" yaml:"connect_code_to_unidirectional_disconnect" toml:"connect_code_to_unidirectional_disconnect" doc:"Configuration for mapping connect error codes to disconnect codes for unidirectional transports."`
}

type UniConnectCodeToDisconnectTransforms []UniConnectCodeToDisconnectTransform

// Decode to implement the envconfig.Decoder interface
func (d *UniConnectCodeToDisconnectTransforms) Decode(value string) error {
	// If the source is a string and the target is a slice, try to parse it as JSON.
	var items UniConnectCodeToDisconnectTransforms
	err := json.Unmarshal([]byte(value), &items)
	if err != nil {
		return fmt.Errorf("error parsing items from JSON: %v", err)
	}
	*d = items
	return nil
}

type ConnectCodeToUnidirectionalDisconnect struct {
	// Enabled allows to enable the feature.
	Enabled bool `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled" doc:"Enables mapping of connect error codes to disconnect codes for unidirectional transports."`
	// Transforms is a list of connect error code to disconnect code transforms.
	Transforms UniConnectCodeToDisconnectTransforms `mapstructure:"transforms" default:"[]" json:"transforms" envconfig:"transforms" yaml:"transforms" toml:"transforms" doc:"List of connect error code to disconnect code mappings for unidirectional transports."`
}

type UniConnectCodeToDisconnectTransform struct {
	// Code is a connect error code.
	Code uint32 `mapstructure:"code" json:"code" envconfig:"code" yaml:"code" toml:"code" doc:"Connect error code to match."`
	// To is a disconnect to transform the code to.
	To TransformDisconnect `mapstructure:"to" json:"to" envconfig:"to" yaml:"to" toml:"to" doc:"Disconnect code and reason to use when the connect error code matches."`
}

type ChannelProxyContainer struct {
	// Subscribe proxy configuration.
	Subscribe Proxy `mapstructure:"subscribe" json:"subscribe" envconfig:"subscribe" yaml:"subscribe" toml:"subscribe" doc:"Default subscribe proxy configuration used for channels that reference the proxy by name <<default>>."`
	// Publish proxy configuration.
	Publish Proxy `mapstructure:"publish" json:"publish" envconfig:"publish" yaml:"publish" toml:"publish" doc:"Default publish proxy configuration used for channels that reference the proxy by name <<default>>."`
	// SubRefresh proxy configuration.
	SubRefresh Proxy `mapstructure:"sub_refresh" json:"sub_refresh" envconfig:"sub_refresh" yaml:"sub_refresh" toml:"sub_refresh" doc:"Default subscription refresh proxy configuration used for channels that reference the proxy by name <<default>>."`
	// SubscribeStream proxy configuration.
	SubscribeStream Proxy `mapstructure:"subscribe_stream" json:"subscribe_stream" envconfig:"subscribe_stream" yaml:"subscribe_stream" toml:"subscribe_stream" doc:"Default subscribe stream proxy configuration used for channels that reference the proxy by name <<default>>."`
	// MapPublish proxy configuration.
	MapPublish Proxy `mapstructure:"map_publish" json:"map_publish" envconfig:"map_publish" yaml:"map_publish" toml:"map_publish" doc:"Default map publish proxy configuration used for channels that reference the proxy by name <<default>>."`
	// MapRemove proxy configuration.
	MapRemove Proxy `mapstructure:"map_remove" json:"map_remove" envconfig:"map_remove" yaml:"map_remove" toml:"map_remove" doc:"Default map remove proxy configuration used for channels that reference the proxy by name <<default>>."`
	// SharedPollRefresh proxy configuration.
	SharedPollRefresh Proxy `mapstructure:"shared_poll_refresh" json:"shared_poll_refresh" envconfig:"shared_poll_refresh" yaml:"shared_poll_refresh" toml:"shared_poll_refresh" doc:"Default shared poll refresh proxy configuration used for channels that reference the proxy by name <<default>>."`
}

type Channel struct {
	// Proxy configuration for channel-related events. All types inside can be referenced by the name "default".
	Proxy ChannelProxyContainer `mapstructure:"proxy" json:"proxy" envconfig:"proxy" toml:"proxy" yaml:"proxy" doc:"Default proxy configurations for channel-level events (subscribe, publish, sub_refresh, etc.). These are referenced by the name <<default>> in namespace configuration."`

	// WithoutNamespace is a configuration of channels options for channels which do not have namespace.
	// Generally, we recommend always use channel namespaces but this option can be useful for simple setups.
	WithoutNamespace ChannelOptions `mapstructure:"without_namespace" json:"without_namespace" envconfig:"without_namespace" yaml:"without_namespace" toml:"without_namespace" doc:"Channel options applied to channels that do not belong to any namespace. For most setups using explicit namespaces is recommended."`
	// Namespaces is a list of channel namespaces. Each channel namespace can have its own set of rules.
	Namespaces ChannelNamespaces `mapstructure:"namespaces" default:"[]" json:"namespaces" envconfig:"namespaces" yaml:"namespaces" toml:"namespaces" doc:"List of channel namespaces. Each namespace defines its own channel options and is matched by the channel name prefix."`

	// HistoryMetaTTL is a time how long to keep history meta information. This is a global option for all channels,
	// but it can be overridden in channel namespace.
	HistoryMetaTTL Duration `mapstructure:"history_meta_ttl" json:"history_meta_ttl" envconfig:"history_meta_ttl" default:"720h" yaml:"history_meta_ttl" toml:"history_meta_ttl" doc:"Global default for how long history stream meta information (offset and epoch) is retained. Default <<720h>> (30 days). Can be overridden per namespace."`

	// PublicationDataFormat is a global publication data format for all channels. Can be overridden in channel namespace.
	// Empty string means default behavior (reject empty data). Possible values: "", "json", "json_object", "binary".
	PublicationDataFormat string `mapstructure:"publication_data_format" json:"publication_data_format" envconfig:"publication_data_format" default:"" yaml:"publication_data_format" toml:"publication_data_format" expose:"full" doc:"Global publication data format for all channels. Possible values: <<json>>, <<json_object>>, <<binary>>, or empty (default — reject empty data). Can be overridden per namespace."`

	// MaxLength is a maximum length of a channel name. This is a global option for all channels.
	MaxLength int `mapstructure:"max_length" json:"max_length" envconfig:"max_length" default:"255" yaml:"max_length" toml:"max_length" doc:"Maximum allowed channel name length in characters. Default <<255>>."`
	// PrivatePrefix is a prefix for private channels. Private channels can't be subscribed without
	// token even if namespace options allows it. This is mostly kept for historic reasons.
	PrivatePrefix string `mapstructure:"private_prefix" json:"private_prefix" envconfig:"private_prefix" default:"$" yaml:"private_prefix" toml:"private_prefix" expose:"full" doc:"Prefix that marks a channel as private. Private channels require a subscription token regardless of namespace permissions. Default <<$>>."`
	// NamespaceBoundary defines a boundary for channel namespaces.
	NamespaceBoundary string `mapstructure:"namespace_boundary" json:"namespace_boundary" envconfig:"namespace_boundary" default:":" yaml:"namespace_boundary" toml:"namespace_boundary" expose:"full" doc:"Separator between the namespace name and the channel name. Default <<:>>."`
	// UserBoundary defines a boundary for user part in channel name.
	UserBoundary string `mapstructure:"user_boundary" json:"user_boundary" envconfig:"user_boundary" default:"#" yaml:"user_boundary" toml:"user_boundary" expose:"full" doc:"Separator between the channel name and the user-limited part. Default <<#>>."`
	// UserSeparator is used for passing several users in user part of channel name.
	UserSeparator string `mapstructure:"user_separator" json:"user_separator" envconfig:"user_separator" default:"," yaml:"user_separator" toml:"user_separator" expose:"full" doc:"Separator between user IDs in user-limited channel names. Default <<,>>."`
}

type RPC struct {
	// Proxy configuration for rpc-related events. Can be referenced by the name "default".
	Proxy Proxy `mapstructure:"proxy" json:"proxy" envconfig:"proxy" toml:"proxy" yaml:"proxy" doc:"Default RPC proxy configuration, referenced by the name <<default>> in RPC namespace configuration."`
	// WithoutNamespace is a configuration of RpcOptions for rpc methods without rpc namespace. Generally,
	// we recommend always use rpc namespaces but this option can be useful for simple setups.
	WithoutNamespace RpcOptions `mapstructure:"without_namespace" json:"without_namespace" envconfig:"without_namespace" yaml:"without_namespace" toml:"without_namespace" doc:"RPC options for methods that do not belong to any RPC namespace."`
	// RPCNamespaces is a list of rpc namespaces. Each rpc namespace can have its own set of rules.
	Namespaces RPCNamespaces `mapstructure:"namespaces" default:"[]" json:"namespaces" envconfig:"namespaces" yaml:"namespaces" toml:"namespaces" doc:"List of RPC namespaces, each with its own options and proxy configuration."`
	// Ping is a configuration for RPC ping method.
	Ping RPCPing `mapstructure:"ping" json:"ping" envconfig:"ping" yaml:"ping" toml:"ping" doc:"Built-in RPC ping method configuration."`
	// NamespaceBoundary allows to set a custom boundary for rpc namespaces.
	NamespaceBoundary string `mapstructure:"namespace_boundary" json:"namespace_boundary" envconfig:"namespace_boundary" default:":" yaml:"namespace_boundary" toml:"namespace_boundary" expose:"full" doc:"Separator between the RPC namespace name and the method name. Default <<:>>."`
}

type RPCPing struct {
	// Enabled allows to enable ping method.
	Enabled bool `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled" doc:"Enables the built-in RPC ping method that clients can call to check connectivity."`
	// Method can be used to override the name of ping method to use.
	Method string `mapstructure:"method" json:"method" envconfig:"method" default:"ping" yaml:"method" toml:"method" expose:"full" doc:"Name of the built-in RPC ping method. Default <<ping>>."`
}

type NamedProxy struct {
	// Name of proxy.
	Name  string `mapstructure:"name" json:"name" envconfig:"name" yaml:"name" toml:"name" expose:"full" doc:"Unique name of the proxy. Referenced by this name in channel or RPC namespace configuration."`
	Proxy `mapstructure:",squash" yaml:",inline"`
}

type NamedProxies []NamedProxy

// Decode to implement the envconfig.Decoder interface
func (d *NamedProxies) Decode(value string) error {
	return decodeToNamedSlice(value, d)
}

type SubscribeToUserPersonalChannel struct {
	// Enabled allows to enable the feature.
	Enabled bool `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled" doc:"Enables automatic server-side subscription to a personal channel for each authenticated user upon connect."`
	// PersonalChannelNamespace is a namespace for personal channels.
	PersonalChannelNamespace string `mapstructure:"personal_channel_namespace" json:"personal_channel_namespace" envconfig:"personal_channel_namespace" yaml:"personal_channel_namespace" toml:"personal_channel_namespace" expose:"full" doc:"Channel namespace used for personal channels. The personal channel is formed as <<namespace:userID>>."`
	// SingleConnection allows enabling a mode when Centrifugo will try to maintain a single connection from user.
	SingleConnection bool `mapstructure:"single_connection" json:"single_connection" yaml:"single_connection" toml:"single_connection" envconfig:"single_connection" doc:"When enabled, Centrifugo ensures only one connection per user by disconnecting previous connections when a new one arrives."`
}

type Node struct {
	// Name is a human-readable name of Centrifugo node in cluster. This must be unique for each running node
	// in a cluster. By default, Centrifugo constructs name from the hostname and port. Name is shown in admin web
	// interface. For communication between nodes in a cluster, Centrifugo uses another identifier – unique ID
	// generated on node start, so node name plays just a human-readable identifier role.
	Name string `mapstructure:"name" json:"name" envconfig:"name" yaml:"name" toml:"name" expose:"full" doc:"Human-readable node name shown in the admin UI. Must be unique across the cluster. Defaults to hostname:port."`
	// InfoMetricsAggregateInterval is a time interval to aggregate node info metrics.
	InfoMetricsAggregateInterval Duration `mapstructure:"info_metrics_aggregate_interval" json:"info_metrics_aggregate_interval" envconfig:"info_metrics_aggregate_interval" default:"60s" yaml:"info_metrics_aggregate_interval" toml:"info_metrics_aggregate_interval" doc:"Interval at which node info metrics are aggregated and published. Default <<60s>>."`
}

type Admin struct {
	Enabled       bool   `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled" doc:"Enables the admin web UI and admin API endpoints."`
	HandlerPrefix string `mapstructure:"handler_prefix" json:"handler_prefix" envconfig:"handler_prefix" default:"" yaml:"handler_prefix" toml:"handler_prefix" expose:"full" doc:"URL prefix for the admin web UI. Empty string serves the UI at the root path."`
	// Password is an admin password.
	Password string `mapstructure:"password" json:"password" envconfig:"password" yaml:"password" toml:"password" doc:"Password for accessing the admin web UI."`
	// Secret is a secret to generate auth token for admin requests.
	Secret string `mapstructure:"secret" json:"secret" envconfig:"secret" yaml:"secret" toml:"secret" doc:"Secret used to sign admin auth tokens. Must be set alongside password."`
	// Insecure turns on insecure mode for admin endpoints - no auth
	// required to connect to web interface and for requests to admin API.
	// Admin resources must be protected by firewall rules in production when
	// this option enabled otherwise everyone from internet can make admin
	// actions.
	Insecure bool `mapstructure:"insecure" json:"insecure" envconfig:"insecure" yaml:"insecure" toml:"insecure" doc:"Disables authentication for admin endpoints. Must be protected by firewall rules in production."`
	// WebPath is path to admin web application to serve.
	WebPath string `mapstructure:"web_path" json:"web_path" envconfig:"web_path" yaml:"web_path" toml:"web_path" expose:"full" doc:"Filesystem path to the admin web application files to serve."`
	// WebProxyAddress is an address for proxying to the running admin web application app.
	// So it's possible to run web app in dev mode and point Centrifugo to its address for
	// development purposes.
	WebProxyAddress string `mapstructure:"web_proxy_address" json:"web_proxy_address" envconfig:"web_proxy_address" yaml:"web_proxy_address" toml:"web_proxy_address" expose:"full" doc:"Address of a running admin web app to proxy requests to. Used for local development of the admin UI."`
	// External is a flag to run admin interface on external port.
	External bool `mapstructure:"external" json:"external" envconfig:"external" yaml:"external" toml:"external" doc:"Serves the admin UI on the external HTTP port instead of the internal port."`
}

type TransformError struct {
	// Code of error.
	Code uint32 `mapstructure:"code" json:"code" envconfig:"code" yaml:"code" toml:"code" doc:"Centrifugo protocol error code to use."`
	// Message is a human-readable message of error.
	Message string `mapstructure:"message" json:"message" envconfig:"message" yaml:"message" toml:"message" expose:"full" doc:"Human-readable error message sent to the client."`
	// Temporary is a flag to mark error as temporary.
	Temporary bool `mapstructure:"temporary" json:"temporary" envconfig:"temporary" yaml:"temporary" toml:"temporary" doc:"Marks the error as temporary, suggesting the client may retry."`
}

type TransformDisconnect struct {
	// Code of disconnect.
	Code uint32 `mapstructure:"code" json:"code" envconfig:"code" yaml:"code" toml:"code" doc:"Centrifugo protocol disconnect code to use."`
	// Reason is a human-readable reason of disconnect.
	Reason string `mapstructure:"reason" json:"reason" envconfig:"reason" yaml:"reason" toml:"reason" expose:"full" doc:"Human-readable disconnect reason sent to the client."`
}

type HttpStatusToCodeTransform struct {
	// StatusCode is an HTTP status code to transform.
	StatusCode int `mapstructure:"status_code" json:"status_code" envconfig:"status_code" yaml:"status_code" toml:"status_code" doc:"HTTP status code returned by the proxy to match."`
	// ToError is a transform to protocol error.
	ToError TransformError `mapstructure:"to_error" json:"to_error" envconfig:"to_error" yaml:"to_error" toml:"to_error" doc:"Protocol error to return to the client when the HTTP status code matches."`
	// ToDisconnect is a transform to protocol disconnect.
	ToDisconnect TransformDisconnect `mapstructure:"to_disconnect" json:"to_disconnect" envconfig:"to_disconnect" yaml:"to_disconnect" toml:"to_disconnect" doc:"Disconnect to send to the client when the HTTP status code matches."`
}

type HttpStatusToCodeTransforms []HttpStatusToCodeTransform

// Decode to implement the envconfig.Decoder interface
func (d *HttpStatusToCodeTransforms) Decode(value string) error {
	// If the source is a string and the target is a slice, try to parse it as JSON.
	var items HttpStatusToCodeTransforms
	err := json.Unmarshal([]byte(value), &items)
	if err != nil {
		return fmt.Errorf("error parsing items from JSON: %v", err)
	}
	*d = items
	return nil
}

type ProxyCommonHTTP struct {
	// TLS for HTTP client.
	TLS TLSConfig `mapstructure:"tls" json:"tls" envconfig:"tls" yaml:"tls" toml:"tls" doc:"TLS configuration for the HTTP proxy client."`
	// StaticHeaders is a static set of key/value pairs to attach to HTTP proxy request as
	// headers. Headers received from HTTP client request or metadata from GRPC client request
	// both have priority over values set in StaticHttpHeaders map.
	StaticHeaders MapStringString `mapstructure:"static_headers" default:"{}" json:"static_headers" envconfig:"static_headers" yaml:"static_headers" toml:"static_headers" doc:"Static key-value headers added to every HTTP proxy request. Client request headers take priority over these values."`
	// StatusToCodeTransforms allow to map HTTP status codes from proxy to Disconnect or Error messages.
	StatusToCodeTransforms HttpStatusToCodeTransforms `mapstructure:"status_to_code_transforms" default:"[]" json:"status_to_code_transforms" envconfig:"status_to_code_transforms" yaml:"status_to_code_transforms" toml:"status_to_code_transforms" doc:"Rules for mapping HTTP status codes from the proxy backend to Centrifugo protocol errors or disconnects."`
}

type ProxyCommonGRPC struct {
	// TLS is a common configuration for GRPC client TLS.
	TLS TLSConfig `mapstructure:"tls" json:"tls" envconfig:"tls" yaml:"tls" toml:"tls" doc:"TLS configuration for the gRPC proxy client."`
	// CredentialsKey is a custom key to add into per-RPC credentials.
	CredentialsKey string `mapstructure:"credentials_key" json:"credentials_key" envconfig:"credentials_key" yaml:"credentials_key" toml:"credentials_key" expose:"full" doc:"Custom metadata key added to per-RPC credentials for gRPC proxy calls."`
	// CredentialsValue is a custom value for GrpcCredentialsKey.
	CredentialsValue string `mapstructure:"credentials_value" json:"credentials_value" envconfig:"credentials_value" yaml:"credentials_value" toml:"credentials_value" doc:"Value for the custom per-RPC credentials key."`
	// Compression enables compression for outgoing calls (gzip).
	Compression bool `mapstructure:"compression" json:"compression" envconfig:"compression" yaml:"compression" toml:"compression" doc:"Enables gzip compression for outgoing gRPC proxy calls."`
	// StaticMetadata is a static set of key/value pairs to attach to GRPC proxy request as
	// metadata. Headers received from HTTP client request or metadata from GRPC client request
	// both have priority over values set in StaticMetadata map (but only if explicitly allowed).
	StaticMetadata MapStringString `mapstructure:"static_metadata" default:"{}" json:"static_metadata" envconfig:"static_metadata" yaml:"static_metadata" toml:"static_metadata" doc:"Static key-value metadata added to every gRPC proxy request. Client request metadata takes priority over these values."`
}

type ProxyCommon struct {
	// HttpHeaders is a list of transport-level HTTP headers to proxy. No headers used by proxy by default.
	// If GRPC proxy is used then request HTTP headers set to outgoing request metadata.
	HttpHeaders []string `mapstructure:"http_headers" json:"http_headers" envconfig:"http_headers" yaml:"http_headers" toml:"http_headers" expose:"full" doc:"List of transport-level HTTP header names to forward to the proxy backend - i.e. headers set on the connection request itself (by the client transport or a reverse proxy in front of Centrifugo), not client-supplied emulated headers. These are trustworthy only when set by a reverse proxy or gateway that clients can't bypass - a directly-connecting client (e.g. a non-browser SDK) can set transport headers itself. When a gRPC proxy is used, these are forwarded as gRPC metadata. To forward headers from the client headers emulation feature, list them in emulated_headers instead."`
	// GrpcMetadata is a list of transport-level GRPC metadata keys to proxy. No meta keys used by proxy by
	// default. If HTTP proxy is used then these keys become outgoing request HTTP headers.
	GrpcMetadata []string `mapstructure:"grpc_metadata" json:"grpc_metadata" envconfig:"grpc_metadata" yaml:"grpc_metadata" toml:"grpc_metadata" expose:"full" doc:"List of transport-level gRPC metadata keys to forward to the proxy backend. When an HTTP proxy is used, these keys become outgoing HTTP headers. To forward client-supplied values from the client headers emulation feature, list them in emulated_headers instead."`
	// EmulatedHeaders is a list of client-supplied emulated header names to proxy. None by default.
	EmulatedHeaders []string `mapstructure:"emulated_headers" json:"emulated_headers" envconfig:"emulated_headers" yaml:"emulated_headers" toml:"emulated_headers" expose:"full" doc:"List of header names taken from the client headers emulation map (sent inside the client's connect frame) to forward to the proxy backend. These values are controlled by the client, so only list names your backend independently validates as untrusted client input (e.g. a signed token) - never names you trust by origin, those belong in http_headers only. A name may also appear in http_headers (e.g. an Authorization token browsers send via emulation and native clients send as a real header): the transport value is preferred and the emulated value used as a fallback."`
	// BinaryEncoding makes proxy send data as base64 string (assuming it contains custom
	// non-JSON payload).
	BinaryEncoding bool `mapstructure:"binary_encoding" json:"binary_encoding" envconfig:"binary_encoding" yaml:"binary_encoding" toml:"binary_encoding" doc:"Encodes publication data as base64 in proxy requests. Use when the payload is binary and not valid JSON."`
	// IncludeConnectionMeta to each proxy request (except connect proxy where it's obtained).
	IncludeConnectionMeta bool `mapstructure:"include_connection_meta" json:"include_connection_meta" envconfig:"include_connection_meta" yaml:"include_connection_meta" toml:"include_connection_meta" doc:"Includes the connection meta object in proxy requests (not applicable to the connect proxy where it is set)."`

	HTTP ProxyCommonHTTP `mapstructure:"http" json:"http" envconfig:"http" yaml:"http" toml:"http" doc:"HTTP-specific proxy configuration (TLS, static headers, status code transforms)."`
	GRPC ProxyCommonGRPC `mapstructure:"grpc" json:"grpc" envconfig:"grpc" yaml:"grpc" toml:"grpc" doc:"gRPC-specific proxy configuration (TLS, credentials, static metadata)."`
}

// Proxy configuration.
type Proxy struct {
	// Endpoint - HTTP address or GRPC service endpoint.
	Endpoint string `mapstructure:"endpoint" json:"endpoint" envconfig:"endpoint" yaml:"endpoint" toml:"endpoint" expose:"url" doc:"HTTP or gRPC endpoint URL of the proxy backend, e.g. <<http://localhost:3000/centrifugo/connect>>."`
	// Timeout for proxy request.
	Timeout Duration `mapstructure:"timeout" default:"1s" json:"timeout" envconfig:"timeout" yaml:"timeout" toml:"timeout" doc:"Timeout for proxy requests to the backend. Default <<1s>>."`

	ProxyCommon `mapstructure:",squash" yaml:",inline"`

	TestGrpcDialer func(context.Context, string) (net.Conn, error) `json:"-" yaml:"-" toml:"-" envconfig:"-"`
}

const (
	ConsumerTypePostgres        = "postgresql"
	ConsumerTypeKafka           = "kafka"
	ConsumerTypeNatsJetStream   = "nats_jetstream"
	ConsumerTypeGooglePubSub    = "google_pub_sub"
	ConsumerTypeAwsSqs          = "aws_sqs"
	ConsumerTypeAzureServiceBus = "azure_service_bus"
	ConsumerTypeRedisStream     = "redis_stream"
	//ConsumerTypeRabbitMQ        = "rabbitmq"
)

var KnownConsumerTypes = []string{
	ConsumerTypePostgres,
	ConsumerTypeKafka,
	ConsumerTypeNatsJetStream,
	ConsumerTypeGooglePubSub,
	ConsumerTypeAwsSqs,
	ConsumerTypeAzureServiceBus,
	ConsumerTypeRedisStream,
	//ConsumerTypeRabbitMQ,
}

type Consumer struct {
	// Name is a unique name required for each consumer.
	Name string `mapstructure:"name" json:"name" envconfig:"name" yaml:"name" toml:"name" expose:"full" doc:"Unique name of the consumer. Required."`

	// Enabled must be true to tell Centrifugo to run configured consumer.
	Enabled bool `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled" doc:"Enables this consumer. Set to <<true>> to start processing messages."`

	// Type describes the type of consumer. Supported types are: `postgresql`, `kafka`, `nats_jetstream`,
	// `redis_stream`, `google_pub_sub`, `aws_sqs`, `azure_service_bus`.
	Type string `mapstructure:"type" json:"type" envconfig:"type" yaml:"type" toml:"type" expose:"full" doc:"Consumer type. Supported values: <<postgresql>>, <<kafka>>, <<nats_jetstream>>, <<redis_stream>>, <<google_pub_sub>>, <<aws_sqs>>, <<azure_service_bus>>."`

	// Postgres allows defining options for consumer of postgresql type.
	Postgres PostgresConsumerConfig `mapstructure:"postgresql" json:"postgresql" envconfig:"postgresql" yaml:"postgresql" toml:"postgresql" doc:"PostgreSQL outbox table consumer configuration. Used when type is <<postgresql>>."`
	// Kafka allows defining options for consumer of kafka type.
	Kafka KafkaConsumerConfig `mapstructure:"kafka" json:"kafka" envconfig:"kafka" yaml:"kafka" toml:"kafka" doc:"Kafka consumer configuration. Used when type is <<kafka>>."`
	// NatsJetStream allows defining options for consumer of nats_jetstream type.
	NatsJetStream NatsJetStreamConsumerConfig `mapstructure:"nats_jetstream" json:"nats_jetstream" envconfig:"nats_jetstream" yaml:"nats_jetstream" toml:"nats_jetstream" doc:"NATS JetStream consumer configuration. Used when type is <<nats_jetstream>>."`
	// RedisStream allows defining options for consumer of redis_stream type.
	RedisStream RedisStreamConsumerConfig `mapstructure:"redis_stream" json:"redis_stream" envconfig:"redis_stream" yaml:"redis_stream" toml:"redis_stream" doc:"Redis Streams consumer configuration. Used when type is <<redis_stream>>."`
	// GooglePubSub allows defining options for consumer of google_pub_sub type.
	GooglePubSub GooglePubSubConsumerConfig `mapstructure:"google_pub_sub" json:"google_pub_sub" envconfig:"google_pub_sub" yaml:"google_pub_sub" toml:"google_pub_sub" doc:"Google Cloud Pub/Sub consumer configuration. Used when type is <<google_pub_sub>>."`
	// AwsSqs allows defining options for consumer of aws_sqs type.
	AwsSqs AwsSqsConsumerConfig `mapstructure:"aws_sqs" json:"aws_sqs" envconfig:"aws_sqs" yaml:"aws_sqs" toml:"aws_sqs" doc:"AWS SQS consumer configuration. Used when type is <<aws_sqs>>."`
	// AzureServiceBus allows defining options for consumer of azure_service_bus type.
	AzureServiceBus AzureServiceBusConsumerConfig `mapstructure:"azure_service_bus" json:"azure_service_bus" envconfig:"azure_service_bus" yaml:"azure_service_bus" toml:"azure_service_bus" doc:"Azure Service Bus consumer configuration. Used when type is <<azure_service_bus>>."`
}

func decodeToNamedSlice(value string, target interface{}) error {
	targetVal := reflect.ValueOf(target)
	if targetVal.Kind() != reflect.Ptr || targetVal.Elem().Kind() != reflect.Slice {
		return fmt.Errorf("target must be a pointer to a slice")
	}
	targetSlice := targetVal.Elem()
	elemType := targetSlice.Type().Elem()

	if value == "" {
		// If the value is empty, leave the slice as it is.
		return nil
	}
	if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
		// Unmarshal JSON array into a temporary slice
		if err := json.Unmarshal([]byte(value), target); err != nil {
			return fmt.Errorf("failed to unmarshal JSON array: %w", err)
		}
	} else {
		// Parse space-separated values
		for _, item := range strings.Split(value, " ") {
			elem := reflect.New(elemType).Elem()
			if elem.Kind() == reflect.Struct {
				field := elem.FieldByName("Name")
				if field.IsValid() && field.Kind() == reflect.String {
					field.SetString(item)
				}
			}
			targetSlice.Set(reflect.Append(targetSlice, elem))
		}
	}
	return nil
}

type Consumers []Consumer

// Decode to implement the envconfig.Decoder interface
func (d *Consumers) Decode(value string) error {
	return decodeToNamedSlice(value, d)
}

// PostgresConsumerConfig is a configuration for Postgres async outbox table consumer.
type PostgresConsumerConfig struct {
	DSN                          string   `mapstructure:"dsn" json:"dsn" envconfig:"dsn" yaml:"dsn" toml:"dsn" expose:"url" doc:"PostgreSQL connection string, e.g. <<postgres://user:pass@localhost:5432/dbname>>."`
	OutboxTableName              string   `mapstructure:"outbox_table_name" json:"outbox_table_name" envconfig:"outbox_table_name" yaml:"outbox_table_name" toml:"outbox_table_name" expose:"full" doc:"Name of the outbox table to consume messages from."`
	NumPartitions                int      `mapstructure:"num_partitions" json:"num_partitions" envconfig:"num_partitions" default:"1" yaml:"num_partitions" toml:"num_partitions" doc:"Number of outbox table partitions to consume in parallel. Default <<1>>."`
	PartitionSelectLimit         int      `mapstructure:"partition_select_limit" json:"partition_select_limit" envconfig:"partition_select_limit" default:"100" yaml:"partition_select_limit" toml:"partition_select_limit" doc:"Maximum number of rows to fetch per partition per poll iteration. Default <<100>>."`
	PartitionPollInterval        Duration `mapstructure:"partition_poll_interval" json:"partition_poll_interval" envconfig:"partition_poll_interval" default:"300ms" yaml:"partition_poll_interval" toml:"partition_poll_interval" doc:"Interval between polling each partition for new outbox messages. Default <<300ms>>."`
	PartitionNotificationChannel string   `mapstructure:"partition_notification_channel" json:"partition_notification_channel" envconfig:"partition_notification_channel" yaml:"partition_notification_channel" toml:"partition_notification_channel" expose:"full" doc:"PostgreSQL LISTEN channel name for instant notification when new outbox rows are inserted, reducing polling latency."`
	// PartitionNotificationDSN is an optional separate DSN used exclusively for
	// the LISTEN connection. Set this to a direct PostgreSQL URL (bypassing
	// PGBouncer) when DSN points at a PGBouncer endpoint — PGBouncer transaction
	// pooling mode is incompatible with LISTEN/NOTIFY. If empty, the primary
	// DSN pool is used (fine for direct PostgreSQL connections).
	PartitionNotificationDSN string    `mapstructure:"partition_notification_dsn" json:"partition_notification_dsn" envconfig:"partition_notification_dsn" yaml:"partition_notification_dsn" toml:"partition_notification_dsn" expose:"url" doc:"Optional separate DSN for the LISTEN connection. Use a direct PostgreSQL URL here when the main DSN points to PGBouncer, since PGBouncer transaction pooling is incompatible with LISTEN/NOTIFY."`
	TLS                      TLSConfig `mapstructure:"tls" json:"tls" envconfig:"tls" yaml:"tls" toml:"tls" doc:"TLS configuration for PostgreSQL connections."`
	// UseTryLock when enabled tells Centrifugo to use pg_try_advisory_xact_lock instead of pg_advisory_xact_lock.
	UseTryLock bool `mapstructure:"use_try_lock" json:"use_try_lock" envconfig:"use_try_lock" default:"false" yaml:"use_try_lock" toml:"use_try_lock" doc:"Uses <<pg_try_advisory_xact_lock>> instead of <<pg_advisory_xact_lock>> for partition locking. Default <<false>>."`
}

func (c PostgresConsumerConfig) Validate() error {
	if c.DSN == "" {
		return errors.New("no Postgres DSN provided")
	}
	if c.OutboxTableName == "" {
		return errors.New("no Postgres outbox table name provided")
	}
	return nil
}

// KafkaConsumerConfig is a configuration for Kafka async consumer.
type KafkaConsumerConfig struct {
	Brokers        []string `mapstructure:"brokers" json:"brokers" envconfig:"brokers" yaml:"brokers" toml:"brokers" expose:"full" doc:"List of Kafka broker addresses, e.g. <<[\"localhost:9092\"]>>."`
	Topics         []string `mapstructure:"topics" json:"topics" envconfig:"topics" yaml:"topics" toml:"topics" expose:"full" doc:"List of Kafka topics to consume from."`
	ConsumerGroup  string   `mapstructure:"consumer_group" json:"consumer_group" envconfig:"consumer_group" yaml:"consumer_group" toml:"consumer_group" expose:"full" doc:"Kafka consumer group ID for coordinated consumption."`
	MaxPollRecords int      `mapstructure:"max_poll_records" json:"max_poll_records" envconfig:"max_poll_records" default:"100" yaml:"max_poll_records" toml:"max_poll_records" doc:"Maximum number of records to fetch per poll. Default <<100>>."`
	// FetchMaxBytes is the maximum number of bytes to fetch from Kafka in a single request.
	// If not set the default 50MB is used.
	FetchMaxBytes int32 `mapstructure:"fetch_max_bytes" json:"fetch_max_bytes" envconfig:"fetch_max_bytes" yaml:"fetch_max_bytes" toml:"fetch_max_bytes" doc:"Maximum bytes to fetch per Kafka poll request. Zero uses the franz-go default (50 MB)."`
	// FetchMaxWait is the maximum time to wait for records when polling.
	// If not set, defaults to 500ms.
	FetchMaxWait Duration `mapstructure:"fetch_max_wait" json:"fetch_max_wait" envconfig:"fetch_max_wait" default:"500ms" yaml:"fetch_max_wait" toml:"fetch_max_wait" doc:"Maximum time to wait for records in a single Kafka poll. Default <<500ms>>."`
	// FetchReadUncommitted is a flag to enable reading uncommitted messages from Kafka. By default, this is false and Centrifugo uses ReadCommitted mode.
	FetchReadUncommitted bool `mapstructure:"fetch_read_uncommitted" json:"fetch_read_uncommitted" envconfig:"fetch_read_uncommitted" default:"false" yaml:"fetch_read_uncommitted" toml:"fetch_read_uncommitted" doc:"Enables reading uncommitted Kafka messages. By default, only committed messages are read. Default <<false>>."`
	// PartitionQueueMaxSize is the maximum number of items in partition queue before pausing consuming from a partition.
	// The actual queue size may exceed this value on `max_poll_records`, so this acts more like a threshold.
	// If -1, pausing is done on every poll. If set, pausing only happens when queue size exceeds this threshold.
	PartitionQueueMaxSize int `mapstructure:"partition_queue_max_size" json:"partition_queue_max_size" envconfig:"partition_queue_max_size" default:"1000" yaml:"partition_queue_max_size" toml:"partition_queue_max_size" doc:"Maximum in-flight items per partition queue before pausing consumption from that partition. Set to <<-1>> to pause on every poll. Default <<1000>>."`

	// DialTimeout is the timeout for establishing a TCP connection to a single broker.
	// With many seed brokers, a lower value speeds up initial discovery when some brokers
	// are unreachable, because franz-go tries them sequentially. If not set, defaults to 3s.
	DialTimeout Duration `mapstructure:"dial_timeout" json:"dial_timeout" envconfig:"dial_timeout" default:"3s" yaml:"dial_timeout" toml:"dial_timeout" doc:"TCP dial timeout per Kafka broker. Lower values speed up seed broker discovery when some are unreachable. Default <<3s>>."`

	// TLS for the connection to Kafka.
	TLS TLSConfig `mapstructure:"tls" json:"tls" envconfig:"tls" yaml:"tls" toml:"tls" doc:"TLS configuration for connections to Kafka."`

	// SASLMechanism when not empty enables SASL auth.
	SASLMechanism string `mapstructure:"sasl_mechanism" json:"sasl_mechanism" envconfig:"sasl_mechanism" yaml:"sasl_mechanism" toml:"sasl_mechanism" expose:"full" doc:"SASL authentication mechanism. Supported values: <<plain>>, <<scram-sha-256>>, <<scram-sha-512>>, <<aws-msk-iam>>. Empty disables SASL."`
	SASLUser      string `mapstructure:"sasl_user" json:"sasl_user" envconfig:"sasl_user" yaml:"sasl_user" toml:"sasl_user" expose:"full" doc:"SASL username for authentication."`
	SASLPassword  string `mapstructure:"sasl_password" json:"sasl_password" envconfig:"sasl_password" yaml:"sasl_password" toml:"sasl_password" doc:"SASL password for authentication."`
	// AssumeRoleARN, when set with sasl_mechanism aws-msk-iam, uses AWS STS AssumeRole to obtain
	// temporary credentials for MSK IAM authentication (e.g. cross-account MSK). Base credentials
	// and region come from the default AWS SDK chain (AWS_REGION, instance metadata, etc.).
	// When set, sasl_user and sasl_password are ignored. When empty, static keys from sasl_user/sasl_password are used.
	AssumeRoleARN string `mapstructure:"assume_role_arn" json:"assume_role_arn" envconfig:"assume_role_arn" yaml:"assume_role_arn" toml:"assume_role_arn" expose:"full" doc:"AWS IAM role ARN to assume for MSK IAM authentication (requires sasl_mechanism <<aws-msk-iam>>). When set, sasl_user and sasl_password are ignored."`

	// InstanceID sets a stable consumer group instance ID for Kafka static membership.
	// When set, enables the static membership protocol: during rolling restarts, a replacement
	// consumer with the same instance ID takes over partitions seamlessly without triggering
	// a group rebalance. This should be a stable identifier across restarts (e.g. Kubernetes pod name).
	// When empty (default), static membership is not used.
	InstanceID string `mapstructure:"instance_id" json:"instance_id" envconfig:"instance_id" yaml:"instance_id" toml:"instance_id" expose:"full" doc:"Stable Kafka consumer group instance ID for static membership. Prevents group rebalances during rolling restarts (e.g. use the Kubernetes pod name). Empty disables static membership."`

	// MethodHeader is a header name to extract method name from Kafka message.
	// If provided in message, then payload must be just a serialized API request object.
	MethodHeader string `mapstructure:"method_header" default:"centrifugo-method" json:"method_header" envconfig:"method_header" yaml:"method_header" toml:"method_header" expose:"full" doc:"Kafka message header name that contains the API method name. When present, the message payload is treated as a serialized API request. Default <<centrifugo-method>>."`

	// PublicationDataMode is a configuration for the mode where message payload already
	// contains data ready to publish into channels, instead of API command.
	PublicationDataMode KafkaPublicationDataModeConfig `mapstructure:"publication_data_mode" json:"publication_data_mode" envconfig:"publication_data_mode" yaml:"publication_data_mode" toml:"publication_data_mode" doc:"Configuration for publication data mode, where the Kafka message payload is data to publish directly into channels."`
}

func (c KafkaConsumerConfig) Validate() error {
	if len(c.Brokers) == 0 {
		return errors.New("no Kafka brokers provided")
	}
	if len(c.Topics) == 0 {
		return errors.New("no Kafka topics provided")
	}
	if c.ConsumerGroup == "" {
		return errors.New("no Kafka consumer group provided")
	}
	if c.PublicationDataMode.Enabled && c.PublicationDataMode.ChannelsHeader == "" {
		return errors.New("no Kafka channels_header_name provided for publication data mode")
	}
	if c.AssumeRoleARN != "" && c.SASLMechanism != "aws-msk-iam" {
		return errors.New("assume_role_arn requires sasl_mechanism aws-msk-iam")
	}
	return nil
}

// KafkaPublicationDataModeConfig is a configuration for Kafka publication data mode.
// In this mode we expect Kafka message payload to contain data ready to publish into
// channels, instead of API command. All other fields used to build channel Publication
// can be passed in Kafka message headers – thus it's possible to integrate existing
// topics with Centrifugo.
type KafkaPublicationDataModeConfig struct {
	// Enabled enables Kafka publication data mode for the Kafka consumer.
	Enabled bool `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled" doc:"Enables publication data mode for the Kafka consumer. The message payload is published directly to channels instead of being treated as an API command."`
	// ChannelsHeader is a header name to extract channels to publish data into
	// (channels must be comma-separated). Ex. of value: "channel1,channel2".
	ChannelsHeader string `mapstructure:"channels_header" default:"centrifugo-channels" json:"channels_header" envconfig:"channels_header" yaml:"channels_header" toml:"channels_header" expose:"full" doc:"Kafka header name containing comma-separated target channel names. Default <<centrifugo-channels>>."`
	// IdempotencyKeyHeader is a header name to extract Publication idempotency key from
	// Kafka message. See https://centrifugal.dev/docs/server/server_api#publishrequest.
	IdempotencyKeyHeader string `mapstructure:"idempotency_key_header" default:"centrifugo-idempotency-key" json:"idempotency_key_header" envconfig:"idempotency_key_header" yaml:"idempotency_key_header" toml:"idempotency_key_header" expose:"full" doc:"Kafka header name for the publication idempotency key. Default <<centrifugo-idempotency-key>>."`
	// DeltaHeader is a header name to extract Publication delta flag from Kafka message
	// which tells Centrifugo whether to use delta compression for message or not.
	// See https://centrifugal.dev/docs/server/delta_compression and
	// https://centrifugal.dev/docs/server/server_api#publishrequest.
	DeltaHeader string `mapstructure:"delta_header" default:"centrifugo-delta" json:"delta_header" envconfig:"delta_header" yaml:"delta_header" toml:"delta_header" expose:"full" doc:"Kafka header name indicating whether the publication should use delta compression. Default <<centrifugo-delta>>."`
	// VersionHeader is a header name to extract Publication version from Kafka message.
	VersionHeader string `mapstructure:"version_header" default:"centrifugo-version" json:"version_header" envconfig:"version_header" yaml:"version_header" toml:"version_header" expose:"full" doc:"Kafka header name for the publication version. Default <<centrifugo-version>>."`
	// VersionEpochHeader is a header name to extract Publication version epoch from Kafka message.
	VersionEpochHeader string `mapstructure:"version_epoch_header" default:"centrifugo-version-epoch" json:"version_epoch_header" envconfig:"version_epoch_header" yaml:"version_epoch_header" toml:"version_epoch_header" expose:"full" doc:"Kafka header name for the publication version epoch. Default <<centrifugo-version-epoch>>."`
	// TagsHeaderPrefix is a prefix for headers that contain tags to attach to Publication.
	TagsHeaderPrefix string `mapstructure:"tags_header_prefix" default:"centrifugo-tag-" json:"tags_header_prefix" envconfig:"tags_header_prefix" yaml:"tags_header_prefix" toml:"tags_header_prefix" expose:"full" doc:"Prefix for Kafka header names used to attach tags to publications. Default <<centrifugo-tag->>."`
}

// RedisStreamPublicationDataModeConfig holds configuration for publication data mode.
type RedisStreamPublicationDataModeConfig struct {
	// Enabled toggles publication data mode.
	Enabled bool `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled" doc:"Enables publication data mode for the Redis Stream consumer. The message field is published directly to channels instead of being treated as an API command."`
	// ChannelsValue is used to extract channels to publish data into (channels must be comma-separated).
	ChannelsValue string `mapstructure:"channels_value" default:"centrifugo-channels" json:"channels_value" envconfig:"channels_value" yaml:"channels_value" toml:"channels_value" expose:"full" doc:"Redis Stream field name containing comma-separated target channel names. Default <<centrifugo-channels>>."`
	// IdempotencyKeyValue is used to extract Publication idempotency key from Redis Stream message.
	IdempotencyKeyValue string `mapstructure:"idempotency_key_value" default:"centrifugo-idempotency-key" json:"idempotency_key_value" envconfig:"idempotency_key_value" yaml:"idempotency_key_value" toml:"idempotency_key_value" expose:"full" doc:"Redis Stream field name for the publication idempotency key. Default <<centrifugo-idempotency-key>>."`
	// DeltaValue is used to extract Publication delta flag from Redis Stream message.
	DeltaValue string `mapstructure:"delta_value" json:"delta_value" default:"centrifugo-delta" envconfig:"delta_value" yaml:"delta_value" toml:"delta_value" expose:"full" doc:"Redis Stream field name for the delta compression flag. Default <<centrifugo-delta>>."`
	// VersionValue is used to extract Publication version from Redis Stream message.
	VersionValue string `mapstructure:"version_value" default:"centrifugo-version" json:"version_value" envconfig:"version_value" yaml:"version_value" toml:"version_value" expose:"full" doc:"Redis Stream field name for the publication version. Default <<centrifugo-version>>."`
	// VersionEpochValue is used to extract Publication version epoch from Redis Stream message.
	VersionEpochValue string `mapstructure:"version_epoch_value" default:"centrifugo-version-epoch" json:"version_epoch_value" envconfig:"version_epoch_value" yaml:"version_epoch_value" toml:"version_epoch_value" expose:"full" doc:"Redis Stream field name for the publication version epoch. Default <<centrifugo-version-epoch>>."`
	// TagsValuePrefix is used to extract Publication tags from Redis Stream message.
	TagsValuePrefix string `mapstructure:"tags_value_prefix" default:"centrifugo-tag-" json:"tags_value_prefix" envconfig:"tags_value_prefix" yaml:"tags_value_prefix" toml:"tags_value_prefix" expose:"full" doc:"Prefix for Redis Stream field names used to attach tags to publications. Default <<centrifugo-tag->>."`
}

// RedisStreamConsumerConfig holds configuration for the Redis Streams consumer.
type RedisStreamConsumerConfig struct {
	Redis `mapstructure:",squash" yaml:",inline"`
	// Streams to consume.
	Streams []string `mapstructure:"streams" json:"streams" envconfig:"streams" yaml:"streams" toml:"streams" expose:"full" doc:"List of Redis stream keys to consume messages from."`
	// ConsumerGroup name to use.
	ConsumerGroup string `mapstructure:"consumer_group" json:"consumer_group" envconfig:"consumer_group" yaml:"consumer_group" toml:"consumer_group" expose:"full" doc:"Redis consumer group name for coordinated stream consumption."`
	// VisibilityTimeout is the time to wait for a message to be processed before it is re-queued.
	VisibilityTimeout Duration `mapstructure:"visibility_timeout" default:"30s" json:"visibility_timeout" envconfig:"visibility_timeout" yaml:"visibility_timeout" toml:"visibility_timeout" doc:"Time after which an unacknowledged message is re-claimed and reprocessed. Default <<30s>>."`
	// NumWorkers is the number of message workers to use for processing for each stream.
	NumWorkers int `mapstructure:"num_workers" default:"1" json:"num_workers" envconfig:"num_workers" yaml:"num_workers" toml:"num_workers" doc:"Number of concurrent worker goroutines processing messages per stream. Default <<1>>."`
	// PayloadValue is used to extract data from Redis Stream message.
	PayloadValue string `mapstructure:"payload_value" default:"payload" json:"payload_value" envconfig:"payload_value" yaml:"payload_value" toml:"payload_value" expose:"full" doc:"Redis Stream field name containing the message payload. Default <<payload>>."`
	// MethodValue is used to extract a method for command messages.
	// If provided in message, then payload must be just a serialized API request object.
	MethodValue string `mapstructure:"method_value" default:"method" json:"method_value" envconfig:"method_value" yaml:"method_value" toml:"method_value" expose:"full" doc:"Redis Stream field name that holds the API method name. When present, the payload field is treated as a serialized API request. Default <<method>>."`
	// PublicationDataMode configures publication data mode.
	PublicationDataMode RedisStreamPublicationDataModeConfig `mapstructure:"publication_data_mode" json:"publication_data_mode" envconfig:"publication_data_mode" yaml:"publication_data_mode" toml:"publication_data_mode" doc:"Configuration for publication data mode where the Redis Stream message payload is data to publish directly to channels."`
}

// Validate validates required fields in the config.
func (c RedisStreamConsumerConfig) Validate() error {
	if len(c.Redis.Address) == 0 {
		return errors.New("redis address is required")
	}
	if len(c.Streams) == 0 {
		return errors.New("streams can't be empty")
	}
	if c.ConsumerGroup == "" {
		return errors.New("consumer_group is required")
	}
	if c.PublicationDataMode.Enabled && c.PublicationDataMode.ChannelsValue == "" {
		return errors.New("channels_value is required when publication data mode is enabled")
	}
	return nil
}

// NatsJetStreamConsumerConfig holds configuration for the NATS JetStream consumer.
type NatsJetStreamConsumerConfig struct {
	// URL is the address of the NATS server.
	URL string `mapstructure:"url" default:"nats://127.0.0.1:4222" json:"url" envconfig:"url" toml:"url" yaml:"url" expose:"url" doc:"NATS server URL. Default <<nats://127.0.0.1:4222>>."`
	// CredentialsFile is the path to a NATS credentials file used for authentication (nats.UserCredentials).
	// If provided, it overrides username/password and token.
	CredentialsFile string `mapstructure:"credentials_file" json:"credentials_file" envconfig:"credentials_file" toml:"credentials_file" yaml:"credentials_file" expose:"full" doc:"Path to a NATS credentials file (nats.UserCredentials). Takes precedence over username/password and token."`
	// Username is used for basic authentication (along with Password) if CredentialsFile is not provided.
	Username string `mapstructure:"username" json:"username" envconfig:"username" toml:"username" yaml:"username" expose:"full" doc:"NATS username for basic authentication (used when credentials_file is not set)."`
	// Password is used with Username for basic authentication.
	Password string `mapstructure:"password" json:"password" envconfig:"password" toml:"password" yaml:"password" doc:"NATS password for basic authentication."`
	// Token is an alternative authentication mechanism if CredentialsFile and Username are not provided.
	Token string `mapstructure:"token" json:"token" envconfig:"token" toml:"token" yaml:"token" doc:"NATS authentication token (used when credentials_file and username are not set)."`
	// StreamName is the name of the NATS JetStream stream to use.
	StreamName string `mapstructure:"stream_name" json:"stream_name" envconfig:"stream_name" toml:"stream_name" yaml:"stream_name" expose:"full" doc:"Name of the NATS JetStream stream to consume from."`
	// UseExistingConsumer when enabled tells Centrifugo to use an existing consumer with
	// durable_consumer_name instead of creating a new one. When on, these fields are ignored:
	// deliver_policy, subjects, max_ack_pending, and all other consumer-creation-related options
	// which may be added later (like ack wait, etc.). The existing consumer's configuration defines
	// all behavior, and Centrifugo will fail to start if the consumer does not already exist.
	UseExistingConsumer bool `mapstructure:"use_existing_consumer" default:"false" json:"use_existing_consumer" envconfig:"use_existing_consumer" toml:"use_existing_consumer" yaml:"use_existing_consumer" doc:"Uses an existing durable consumer instead of creating a new one. When enabled, deliver_policy, subjects and max_ack_pending are ignored. Centrifugo fails to start if the consumer does not exist. Default <<false>>."`
	// Subjects is the list of NATS subjects (topics) to filter.
	Subjects []string `mapstructure:"subjects" json:"subjects" envconfig:"subjects" toml:"subjects" yaml:"subjects" expose:"full" doc:"List of NATS subjects to filter messages from the stream."`
	// DurableConsumerName sets the name of the durable JetStream consumer to use.
	DurableConsumerName string `mapstructure:"durable_consumer_name" json:"durable_consumer_name" envconfig:"durable_consumer_name" toml:"durable_consumer_name" yaml:"durable_consumer_name" expose:"full" doc:"Name of the durable JetStream consumer to create or attach to. Required."`
	// DeliverPolicy is the NATS JetStream delivery policy for the consumer. By default, it is set to "new". Possible values: `new`, `all`.
	DeliverPolicy string `mapstructure:"deliver_policy" default:"new" json:"deliver_policy" envconfig:"deliver_policy" toml:"deliver_policy" yaml:"deliver_policy" expose:"full" doc:"JetStream delivery policy. Possible values: <<new>> (only new messages), <<all>> (replay from beginning). Default <<new>>."`
	// MaxAckPending is the maximum number of unacknowledged messages that can be pending for the consumer.
	MaxAckPending int `mapstructure:"max_ack_pending" default:"100" json:"max_ack_pending" envconfig:"max_ack_pending" toml:"max_ack_pending" yaml:"max_ack_pending" doc:"Maximum number of unacknowledged messages outstanding for the JetStream consumer. Default <<100>>."`
	// MethodHeader is the NATS message header used to extract the method name for dispatching commands.
	// If provided in message, then payload must be just a serialized API request object.
	MethodHeader string `mapstructure:"method_header" default:"centrifugo-method" json:"method_header" envconfig:"method_header" toml:"method_header" yaml:"method_header" expose:"full" doc:"NATS message header name that contains the API method name. When present, the payload is treated as a serialized API request. Default <<centrifugo-method>>."`
	// PublicationDataMode configures extraction of pre-formatted publication data from message headers.
	PublicationDataMode NatsJetStreamPublicationDataModeConfig `mapstructure:"publication_data_mode" json:"publication_data_mode" envconfig:"publication_data_mode" toml:"publication_data_mode" yaml:"publication_data_mode" doc:"Configuration for publication data mode where the NATS message payload is data to publish directly to channels."`
	// TLS is the configuration for TLS.
	TLS TLSConfig `mapstructure:"tls" json:"tls" envconfig:"tls" toml:"tls" yaml:"tls" doc:"TLS configuration for connections to NATS JetStream."`
}

// NatsJetStreamPublicationDataModeConfig holds settings for publication data mode.
type NatsJetStreamPublicationDataModeConfig struct {
	// Enabled toggles publication data mode.
	Enabled bool `mapstructure:"enabled" json:"enabled" envconfig:"enabled" toml:"enabled" yaml:"enabled" doc:"Enables publication data mode for the NATS JetStream consumer. The message payload is published directly to channels instead of being treated as an API command."`
	// ChannelsHeader is the name of the header that contains comma-separated channel names.
	ChannelsHeader string `mapstructure:"channels_header" default:"centrifugo-channels" json:"channels_header" envconfig:"channels_header" toml:"channels_header" yaml:"channels_header" expose:"full" doc:"NATS message header name containing comma-separated target channel names. Default <<centrifugo-channels>>."`
	// IdempotencyKeyHeader is the name of the header that contains an idempotency key for deduplication.
	IdempotencyKeyHeader string `mapstructure:"idempotency_key_header" default:"centrifugo-idempotency-key" json:"idempotency_key_header" envconfig:"idempotency_key_header" toml:"idempotency_key_header" yaml:"idempotency_key_header" expose:"full" doc:"NATS message header name for the publication idempotency key. Default <<centrifugo-idempotency-key>>."`
	// DeltaHeader is the name of the header indicating whether the message represents a delta (partial update).
	DeltaHeader string `mapstructure:"delta_header" default:"centrifugo-delta" json:"delta_header" envconfig:"delta_header" toml:"delta_header" yaml:"delta_header" expose:"full" doc:"NATS message header name for the delta compression flag. Default <<centrifugo-delta>>."`
	// VersionHeader is the name of the header that contains the version of the message.
	VersionHeader string `mapstructure:"version_header" default:"centrifugo-version" json:"version_header" envconfig:"version_header" toml:"version_header" yaml:"version_header" expose:"full" doc:"NATS message header name for the publication version. Default <<centrifugo-version>>."`
	// VersionEpochHeader is the name of the header that contains the version epoch of the message.
	VersionEpochHeader string `mapstructure:"version_epoch_header" default:"centrifugo-version-epoch" json:"version_epoch_header" envconfig:"version_epoch_header" toml:"version_epoch_header" yaml:"version_epoch_header" expose:"full" doc:"NATS message header name for the publication version epoch. Default <<centrifugo-version-epoch>>."`
	// TagsHeaderPrefix is the prefix used to extract dynamic tags from message headers.
	TagsHeaderPrefix string `mapstructure:"tags_header_prefix" default:"centrifugo-tag-" json:"tags_header_prefix" envconfig:"tags_header_prefix" toml:"tags_header_prefix" yaml:"tags_header_prefix" expose:"full" doc:"Prefix for NATS message header names used to attach tags to publications. Default <<centrifugo-tag->>."`
}

// Validate validates the required fields.
func (cfg NatsJetStreamConsumerConfig) Validate() error {
	if cfg.URL == "" {
		return errors.New("url is required")
	}
	if cfg.StreamName == "" {
		return errors.New("stream_name is required")
	}
	if cfg.DurableConsumerName == "" {
		return errors.New("durable_consumer_name is required for consumer")
	}
	if !cfg.UseExistingConsumer {
		if cfg.DeliverPolicy != "new" && cfg.DeliverPolicy != "all" {
			return errors.New("deliver_policy must be either 'new' or 'all'")
		}
	}
	if cfg.PublicationDataMode.Enabled && cfg.PublicationDataMode.ChannelsHeader == "" {
		return errors.New("channels_header is required for publication data mode")
	}
	return nil
}

// GooglePubSubConsumerConfig is a configuration for the Google Pub/Sub consumer.
type GooglePubSubConsumerConfig struct {
	// Google Cloud project ID.
	ProjectID string `mapstructure:"project_id" json:"project_id" envconfig:"project_id" yaml:"project_id" toml:"project_id" expose:"full" doc:"Google Cloud project ID that owns the Pub/Sub subscriptions."`
	// Subscriptions is the list of Pub/Sub subscription ids to consume from.
	Subscriptions []string `mapstructure:"subscriptions" json:"subscriptions" envconfig:"subscriptions" yaml:"subscriptions" toml:"subscriptions" expose:"full" doc:"List of Google Cloud Pub/Sub subscription IDs to consume messages from."`
	// MaxOutstandingMessages controls the maximum number of unprocessed messages.
	MaxOutstandingMessages int `mapstructure:"max_outstanding_messages" default:"100" json:"max_outstanding_messages" envconfig:"max_outstanding_messages" yaml:"max_outstanding_messages" toml:"max_outstanding_messages" doc:"Maximum number of unacknowledged Pub/Sub messages outstanding at a time. Default <<100>>."`
	// MaxOutstandingBytes controls the maximum number of unprocessed bytes.
	MaxOutstandingBytes int `mapstructure:"max_outstanding_bytes" default:"1000000" json:"max_outstanding_bytes" envconfig:"max_outstanding_bytes" yaml:"max_outstanding_bytes" toml:"max_outstanding_bytes" doc:"Maximum total size of unacknowledged Pub/Sub message payloads in bytes. Default <<1000000>>."`
	// AuthMechanism specifies which authentication mechanism to use:
	// "default", "service_account".
	AuthMechanism string `mapstructure:"auth_mechanism" json:"auth_mechanism" envconfig:"auth_mechanism" yaml:"auth_mechanism" toml:"auth_mechanism" expose:"full" doc:"Authentication mechanism for Google Cloud. Possible values: <<default>> (Application Default Credentials), <<service_account>> (explicit service account JSON)."`
	// CredentialsFile is the path to the service account JSON file if required.
	CredentialsFile string `mapstructure:"credentials_file" json:"credentials_file" envconfig:"credentials_file" yaml:"credentials_file" toml:"credentials_file" expose:"full" doc:"Path to the service account JSON credentials file. Required when auth_mechanism is <<service_account>>."`
	// MethodAttribute is an attribute name to extract a method name from the message.
	// If provided in message, then payload must be just a serialized API request object.
	MethodAttribute string `mapstructure:"method_attribute" default:"centrifugo-method" json:"method_attribute" envconfig:"method_attribute" yaml:"method_attribute" toml:"method_attribute" expose:"full" doc:"Pub/Sub message attribute name that contains the API method name. When present, the payload is treated as a serialized API request. Default <<centrifugo-method>>."`
	// PublicationDataMode holds settings for the mode where message payload already contains data
	// ready to publish into channels.
	PublicationDataMode GooglePubSubPublicationDataModeConfig `mapstructure:"publication_data_mode" json:"publication_data_mode" envconfig:"publication_data_mode" yaml:"publication_data_mode" toml:"publication_data_mode" doc:"Configuration for publication data mode where the Pub/Sub message payload is data to publish directly to channels."`
}

// GooglePubSubPublicationDataModeConfig is the configuration for the publication data mode.
type GooglePubSubPublicationDataModeConfig struct {
	// Enabled enables publication data mode.
	Enabled bool `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled" doc:"Enables publication data mode for the Google Pub/Sub consumer. The message payload is published directly to channels instead of being treated as an API command."`
	// ChannelsAttribute is the attribute name containing comma-separated channel names.
	ChannelsAttribute string `mapstructure:"channels_attribute" default:"centrifugo-channels" json:"channels_attribute" envconfig:"channels_attribute" yaml:"channels_attribute" toml:"channels_attribute" expose:"full" doc:"Pub/Sub message attribute name containing comma-separated target channel names. Default <<centrifugo-channels>>."`
	// IdempotencyKeyAttribute is the attribute name for an idempotency key.
	IdempotencyKeyAttribute string `mapstructure:"idempotency_key_attribute" default:"centrifugo-idempotency-key" json:"idempotency_key_attribute" envconfig:"idempotency_key_attribute" yaml:"idempotency_key_attribute" toml:"idempotency_key_attribute" expose:"full" doc:"Pub/Sub message attribute name for the publication idempotency key. Default <<centrifugo-idempotency-key>>."`
	// DeltaAttribute is the attribute name for a delta flag.
	DeltaAttribute string `mapstructure:"delta_attribute" default:"centrifugo-delta" json:"delta_attribute" envconfig:"delta_attribute" yaml:"delta_attribute" toml:"delta_attribute" expose:"full" doc:"Pub/Sub message attribute name for the delta compression flag. Default <<centrifugo-delta>>."`
	// VersionAttribute is the attribute name for a version.
	VersionAttribute string `mapstructure:"version_attribute" default:"centrifugo-version" json:"version_attribute" envconfig:"version_attribute" yaml:"version_attribute" toml:"version_attribute" expose:"full" doc:"Pub/Sub message attribute name for the publication version. Default <<centrifugo-version>>."`
	// VersionEpochAttribute is the attribute name for a version epoch.
	VersionEpochAttribute string `mapstructure:"version_epoch_attribute" default:"centrifugo-version-epoch" json:"version_epoch_attribute" envconfig:"version_epoch_attribute" yaml:"version_epoch_attribute" toml:"version_epoch_attribute" expose:"full" doc:"Pub/Sub message attribute name for the publication version epoch. Default <<centrifugo-version-epoch>>."`
	// TagsAttributePrefix is the prefix for attributes containing tags.
	TagsAttributePrefix string `mapstructure:"tags_attribute_prefix" default:"centrifugo-tag-" json:"tags_attribute_prefix" envconfig:"tags_attribute_prefix" yaml:"tags_attribute_prefix" toml:"tags_attribute_prefix" expose:"full" doc:"Prefix for Pub/Sub message attribute names used to attach tags to publications. Default <<centrifugo-tag->>."`
}

// Validate ensures required fields are set.
func (c GooglePubSubConsumerConfig) Validate() error {
	if c.ProjectID == "" {
		return errors.New("project_id is required")
	}
	if len(c.Subscriptions) == 0 {
		return errors.New("at least one subscription ID is required")
	}
	if c.PublicationDataMode.Enabled && c.PublicationDataMode.ChannelsAttribute == "" {
		return errors.New("channels_attribute is required for publication data mode")
	}
	return nil
}

// AzureServiceBusPublicationDataModeConfig holds configuration for publication data mode,
// where the incoming message payload is already structured for downstream publication.
type AzureServiceBusPublicationDataModeConfig struct {
	// Enabled toggles the publication data mode.
	Enabled bool `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled" doc:"Enables publication data mode for the Azure Service Bus consumer. The message payload is published directly to channels instead of being treated as an API command."`
	// ChannelsProperty is the name of the message property that contains the list of target channels.
	ChannelsProperty string `mapstructure:"channels_property" default:"centrifugo-channels" json:"channels_property" envconfig:"channels_property" yaml:"channels_property" toml:"channels_property" expose:"full" doc:"Azure Service Bus message property name containing comma-separated target channel names. Default <<centrifugo-channels>>."`
	// IdempotencyKeyProperty is the property that holds an idempotency key for deduplication.
	IdempotencyKeyProperty string `mapstructure:"idempotency_key_property" default:"centrifugo-idempotency-key" json:"idempotency_key_property" envconfig:"idempotency_key_property" yaml:"idempotency_key_property" toml:"idempotency_key_property" expose:"full" doc:"Azure Service Bus message property name for the publication idempotency key. Default <<centrifugo-idempotency-key>>."`
	// DeltaProperty is the property that represents changes or deltas in the payload.
	DeltaProperty string `mapstructure:"delta_property" default:"centrifugo-delta" json:"delta_property" envconfig:"delta_property" yaml:"delta_property" toml:"delta_property" expose:"full" doc:"Azure Service Bus message property name for the delta compression flag. Default <<centrifugo-delta>>."`
	// VersionProperty is the property that holds the version of the message.
	VersionProperty string `mapstructure:"version_property" default:"centrifugo-version" json:"version_property" envconfig:"version_property" yaml:"version_property" toml:"version_property" expose:"full" doc:"Azure Service Bus message property name for the publication version. Default <<centrifugo-version>>."`
	// VersionEpochProperty is the property that holds the version epoch of the message.
	VersionEpochProperty string `mapstructure:"version_epoch_property" default:"centrifugo-version-epoch" json:"version_epoch_property" envconfig:"version_epoch_property" yaml:"version_epoch_property" toml:"version_epoch_property" expose:"full" doc:"Azure Service Bus message property name for the publication version epoch. Default <<centrifugo-version-epoch>>."`
	// TagsPropertyPrefix defines the prefix used to extract dynamic tags from message properties.
	TagsPropertyPrefix string `mapstructure:"tags_property_prefix" default:"centrifugo-tag-" json:"tags_property_prefix" envconfig:"tags_property_prefix" yaml:"tags_property_prefix" toml:"tags_property_prefix" expose:"full" doc:"Prefix for Azure Service Bus message property names used to attach tags to publications. Default <<centrifugo-tag->>."`
}

// AzureServiceBusConsumerConfig holds configuration for the Azure Service Bus consumer.
type AzureServiceBusConsumerConfig struct {
	// ConnectionString is the full connection string used for connection-string–based authentication.
	ConnectionString string `mapstructure:"connection_string" json:"connection_string" envconfig:"connection_string" yaml:"connection_string" toml:"connection_string" doc:"Azure Service Bus connection string for authentication. Used when use_azure_identity is <<false>>."`
	// UseAzureIdentity toggles Azure Identity (AAD) authentication instead of connection strings.
	UseAzureIdentity bool `mapstructure:"use_azure_identity" json:"use_azure_identity" envconfig:"use_azure_identity" yaml:"use_azure_identity" toml:"use_azure_identity" doc:"Enables Azure Active Directory (Managed Identity / Service Principal) authentication instead of a connection string."`
	// FullyQualifiedNamespace is the Service Bus namespace, e.g. "your-namespace.servicebus.windows.net".
	FullyQualifiedNamespace string `mapstructure:"fully_qualified_namespace" json:"fully_qualified_namespace" envconfig:"fully_qualified_namespace" yaml:"fully_qualified_namespace" toml:"fully_qualified_namespace" expose:"full" doc:"Fully qualified Azure Service Bus namespace hostname, e.g. <<your-namespace.servicebus.windows.net>>. Required when use_azure_identity is <<true>>."`
	// TenantID is the Azure Active Directory tenant ID used with Azure Identity.
	TenantID string `mapstructure:"tenant_id" json:"tenant_id" envconfig:"tenant_id" yaml:"tenant_id" toml:"tenant_id" expose:"full" doc:"Azure Active Directory tenant ID. Required when use_azure_identity is <<true>>."`
	// ClientID is the Azure AD application (client) ID used for authentication.
	ClientID string `mapstructure:"client_id" json:"client_id" envconfig:"client_id" yaml:"client_id" toml:"client_id" expose:"full" doc:"Azure AD application (client) ID. Required when use_azure_identity is <<true>>."`
	// ClientSecret is the secret associated with the Azure AD application.
	ClientSecret string `mapstructure:"client_secret" json:"client_secret" envconfig:"client_secret" yaml:"client_secret" toml:"client_secret" doc:"Azure AD application client secret. Required when use_azure_identity is <<true>>."`
	// Queues is the list of the Azure Service Bus queues to consume from.
	Queues []string `mapstructure:"queues" json:"queues" envconfig:"queues" yaml:"queues" toml:"queues" expose:"full" doc:"List of Azure Service Bus queue names to consume messages from."`
	// UseSessions enables session-aware message handling.
	// All messages must include a SessionID; messages within the same session will be processed in order.
	UseSessions bool `mapstructure:"use_sessions" json:"use_sessions" envconfig:"use_sessions" yaml:"use_sessions" toml:"use_sessions" doc:"Enables session-aware message processing. All messages must include a SessionID; messages within a session are processed in order."`
	// MaxConcurrentCalls controls the maximum number of messages processed concurrently.
	MaxConcurrentCalls int `mapstructure:"max_concurrent_calls" default:"1" json:"max_concurrent_calls" envconfig:"max_concurrent_calls" yaml:"max_concurrent_calls" toml:"max_concurrent_calls" doc:"Maximum number of messages processed concurrently per queue. Default <<1>>."`
	// MaxReceiveMessages sets the batch size when receiving messages from the queue.
	MaxReceiveMessages int `mapstructure:"max_receive_messages" default:"1" json:"max_receive_messages" envconfig:"max_receive_messages" yaml:"max_receive_messages" toml:"max_receive_messages" doc:"Maximum number of messages to receive in one batch from the queue. Default <<1>>."`
	// MethodProperty is the name of the message property used to extract the method (for API command).
	// If provided in message, then payload must be just a serialized API request object.
	MethodProperty string `mapstructure:"method_property" default:"centrifugo-method" json:"method_property" envconfig:"method_property" yaml:"method_property" toml:"method_property" expose:"full" doc:"Azure Service Bus message property name that contains the API method name. When present, the payload is treated as a serialized API request. Default <<centrifugo-method>>."`
	// PublicationDataMode configures how structured publication-ready data is extracted from the message.
	PublicationDataMode AzureServiceBusPublicationDataModeConfig `mapstructure:"publication_data_mode" json:"publication_data_mode" envconfig:"publication_data_mode" yaml:"publication_data_mode" toml:"publication_data_mode" doc:"Configuration for publication data mode where the Azure Service Bus message payload is data to publish directly to channels."`
}

// Validate checks that required fields are set.
func (c AzureServiceBusConsumerConfig) Validate() error {
	if c.UseAzureIdentity {
		if c.FullyQualifiedNamespace == "" || c.TenantID == "" || c.ClientID == "" || c.ClientSecret == "" {
			return errors.New("when using Azure Identity, fully_qualified_namespace, tenant_id, client_id and client_secret are required")
		}
	} else {
		if c.ConnectionString == "" {
			return errors.New("connection_string is required when not using Azure Identity")
		}
	}
	if len(c.Queues) == 0 {
		return errors.New("at least one queue path is required")
	}
	if c.PublicationDataMode.Enabled && c.PublicationDataMode.ChannelsProperty == "" {
		return errors.New("channels_property is required for publication data mode")
	}
	return nil
}

// AwsSqsConsumerConfig holds configuration for the AWS consumer.
type AwsSqsConsumerConfig struct {
	// Queues is a list of SQS queue URLs to consume.
	Queues []string `mapstructure:"queues" json:"queues" envconfig:"queues" yaml:"queues" toml:"queues" expose:"url" doc:"List of SQS queue URLs to consume messages from."`
	// SNSEnvelope, when true, expects messages to be wrapped in an SNS envelope – this is required when
	// consuming from SNS topics with SQS subscriptions.
	SNSEnvelope bool `mapstructure:"sns_envelope" json:"sns_envelope" envconfig:"sns_envelope" yaml:"sns_envelope" toml:"sns_envelope" doc:"Expects messages to be wrapped in an SNS envelope. Required when consuming from SNS topics via SQS subscriptions."`
	// Region is the AWS region.
	Region string `mapstructure:"region" json:"region" envconfig:"region" yaml:"region" toml:"region" expose:"full" doc:"AWS region where the SQS queues reside."`
	// MaxNumberOfMessages is the maximum number of messages to receive per poll.
	MaxNumberOfMessages int32 `mapstructure:"max_number_of_messages" default:"10" json:"max_number_of_messages" envconfig:"max_number_of_messages" yaml:"max_number_of_messages" toml:"max_number_of_messages" doc:"Maximum number of SQS messages to receive per poll request. Default <<10>>."`
	// PollWaitTime is the long-poll wait time. Rounded to seconds internally.
	PollWaitTime Duration `mapstructure:"wait_time_time" json:"wait_time_time" envconfig:"wait_time_time" default:"20s" yaml:"wait_time_time" toml:"wait_time_time" doc:"SQS long-poll wait time per receive request. Rounded to seconds. Default <<20s>>."`
	// VisibilityTimeout is the time a message is hidden from other consumers. Rounded to seconds internally.
	VisibilityTimeout Duration `mapstructure:"visibility_timeout" json:"visibility_timeout" envconfig:"visibility_timeout" default:"30s" yaml:"visibility_timeout" toml:"visibility_timeout" doc:"Time a received SQS message is hidden from other consumers while being processed. Rounded to seconds. Default <<30s>>."`
	// MaxConcurrency defines max concurrency during message batch processing.
	MaxConcurrency int `mapstructure:"max_concurrency" json:"max_concurrency" envconfig:"max_concurrency" default:"1" yaml:"max_concurrency" toml:"max_concurrency" doc:"Maximum number of messages processed concurrently within a batch. Default <<1>>."`
	// CredentialsProfile is a shared credentials profile to use.
	CredentialsProfile string `mapstructure:"credentials_profile" json:"credentials_profile" envconfig:"credentials_profile" yaml:"credentials_profile" toml:"credentials_profile" expose:"full" doc:"AWS shared credentials profile name to use for authentication."`
	// AssumeRoleARN, if provided, will cause the consumer to assume the given IAM role.
	AssumeRoleARN string `mapstructure:"assume_role_arn" json:"assume_role_arn" envconfig:"assume_role_arn" yaml:"assume_role_arn" toml:"assume_role_arn" expose:"full" doc:"IAM role ARN to assume for SQS access. Useful for cross-account queue access."`
	// MethodAttribute is the attribute name to extract a method for command messages.
	// If provided in message, then payload must be just a serialized API request object.
	MethodAttribute string `mapstructure:"method_attribute" default:"centrifugo-method" json:"method_attribute" envconfig:"method_attribute" yaml:"method_attribute" toml:"method_attribute" expose:"full" doc:"SQS message attribute name that contains the API method name. When present, the payload is treated as a serialized API request. Default <<centrifugo-method>>."`
	// LocalStackEndpoint if set enables using localstack with provided URL.
	LocalStackEndpoint string `mapstructure:"localstack_endpoint" json:"localstack_endpoint" envconfig:"localstack_endpoint" yaml:"localstack_endpoint" toml:"localstack_endpoint" expose:"url" doc:"LocalStack endpoint URL. When set, enables using LocalStack for local AWS development and testing."`
	// PublicationDataMode holds settings for the mode where message payload already contains data
	// ready to publish into channels.
	PublicationDataMode AWSPublicationDataModeConfig `mapstructure:"publication_data_mode" json:"publication_data_mode" envconfig:"publication_data_mode" yaml:"publication_data_mode" toml:"publication_data_mode" doc:"Configuration for publication data mode where the SQS message body is data to publish directly to channels."`
}

// AWSPublicationDataModeConfig holds configuration for the publication data mode.
type AWSPublicationDataModeConfig struct {
	// Enabled enables publication data mode.
	Enabled bool `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled" doc:"Enables publication data mode for the AWS SQS consumer. The message body is published directly to channels instead of being treated as an API command."`
	// ChannelsAttribute is the attribute name containing comma-separated channel names.
	ChannelsAttribute string `mapstructure:"channels_attribute" default:"centrifugo-channels" json:"channels_attribute" envconfig:"channels_attribute" yaml:"channels_attribute" toml:"channels_attribute" expose:"full" doc:"SQS message attribute name containing comma-separated target channel names. Default <<centrifugo-channels>>."`
	// IdempotencyKeyAttribute is the attribute name for an idempotency key.
	IdempotencyKeyAttribute string `mapstructure:"idempotency_key_attribute" default:"centrifugo-idempotency-key" json:"idempotency_key_attribute" envconfig:"idempotency_key_attribute" yaml:"idempotency_key_attribute" toml:"idempotency_key_attribute" expose:"full" doc:"SQS message attribute name for the publication idempotency key. Default <<centrifugo-idempotency-key>>."`
	// DeltaAttribute is the attribute name for a delta flag.
	DeltaAttribute string `mapstructure:"delta_attribute" default:"centrifugo-delta" json:"delta_attribute" envconfig:"delta_attribute" yaml:"delta_attribute" toml:"delta_attribute" expose:"full" doc:"SQS message attribute name for the delta compression flag. Default <<centrifugo-delta>>."`
	// VersionAttribute is the attribute name for a version of publication.
	VersionAttribute string `mapstructure:"version_attribute" default:"centrifugo-version" json:"version_attribute" envconfig:"version_attribute" yaml:"version_attribute" toml:"version_attribute" expose:"full" doc:"SQS message attribute name for the publication version. Default <<centrifugo-version>>."`
	// VersionEpochAttribute is the attribute name for a version epoch of publication.
	VersionEpochAttribute string `mapstructure:"version_epoch_attribute" default:"centrifugo-version-epoch" json:"version_epoch_attribute" envconfig:"version_epoch_attribute" yaml:"version_epoch_attribute" toml:"version_epoch_attribute" expose:"full" doc:"SQS message attribute name for the publication version epoch. Default <<centrifugo-version-epoch>>."`
	// TagsAttributePrefix is the prefix for attributes containing tags.
	TagsAttributePrefix string `mapstructure:"tags_attribute_prefix" default:"centrifugo-tag-" json:"tags_attribute_prefix" envconfig:"tags_attribute_prefix" yaml:"tags_attribute_prefix" toml:"tags_attribute_prefix" expose:"full" doc:"Prefix for SQS message attribute names used to attach tags to publications. Default <<centrifugo-tag->>."`
}

// Validate ensures required fields are set.
func (c AwsSqsConsumerConfig) Validate() error {
	if len(c.Queues) == 0 {
		return errors.New("at least one queue url is required")
	}
	if c.PublicationDataMode.Enabled && c.PublicationDataMode.ChannelsAttribute == "" {
		return errors.New("channels_attribute is required for publication data mode")
	}
	return nil
}
