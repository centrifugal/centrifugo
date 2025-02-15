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
	Address string `mapstructure:"address" json:"address" envconfig:"address" toml:"address" yaml:"address"`
	// Port to bind HTTP server to.
	Port int `mapstructure:"port" json:"port" envconfig:"port" default:"8000" toml:"port" yaml:"port"`
	// InternalAddress to bind internal HTTP server to. Internal server is used to serve endpoints
	// which are normally should not be exposed to the outside world.
	InternalAddress string `mapstructure:"internal_address" json:"internal_address" envconfig:"internal_address" toml:"internal_address" yaml:"internal_address"`
	// InternalPort to bind internal HTTP server to.
	InternalPort string `mapstructure:"internal_port" json:"internal_port" envconfig:"internal_port" toml:"internal_port" yaml:"internal_port"`
	// TLS configuration for HTTP server.
	TLS TLSConfig `mapstructure:"tls" json:"tls" envconfig:"tls" toml:"tls" yaml:"tls"`
	// TLSAutocert for automatic TLS certificates from ACME provider (ex. Let's Encrypt).
	TLSAutocert TLSAutocert `mapstructure:"tls_autocert" json:"tls_autocert" envconfig:"tls_autocert" toml:"tls_autocert" yaml:"tls_autocert"`
	// TLSExternal enables TLS only for external HTTP endpoints.
	TLSExternal bool `mapstructure:"tls_external" json:"tls_external" envconfig:"tls_external" toml:"tls_external" yaml:"tls_external"`
	// InternalTLS is a custom configuration for internal HTTP endpoints. If not set InternalTLS will be the same as TLS.
	InternalTLS TLSConfig `mapstructure:"internal_tls" json:"internal_tls" envconfig:"internal_tls" toml:"internal_tls" yaml:"internal_tls"`
	// HTTP3 allows enabling HTTP/3 support. EXPERIMENTAL!
	HTTP3 HTTP3 `mapstructure:"http3" json:"http3" envconfig:"http3" toml:"http3" yaml:"http3"`
}

// Log configuration.
type Log struct {
	// Level is a log level for Centrifugo logger. Supported values: none, trace, debug, info, warn, error.
	Level string `mapstructure:"level" default:"info" json:"level" envconfig:"level" toml:"level" yaml:"level"`
	// File is a path to log file. If not set logs go to stdout.
	File string `mapstructure:"file" json:"file" envconfig:"file" toml:"file" yaml:"file"`
}

// Token common configuration.
type Token struct {
	HMACSecretKey      string `mapstructure:"hmac_secret_key" json:"hmac_secret_key" envconfig:"hmac_secret_key" yaml:"hmac_secret_key" toml:"hmac_secret_key"`
	RSAPublicKey       string `mapstructure:"rsa_public_key" json:"rsa_public_key" envconfig:"rsa_public_key" yaml:"rsa_public_key" toml:"rsa_public_key"`
	ECDSAPublicKey     string `mapstructure:"ecdsa_public_key" json:"ecdsa_public_key" envconfig:"ecdsa_public_key" yaml:"ecdsa_public_key" toml:"ecdsa_public_key"`
	JWKSPublicEndpoint string `mapstructure:"jwks_public_endpoint" json:"jwks_public_endpoint" envconfig:"jwks_public_endpoint" yaml:"jwks_public_endpoint" toml:"jwks_public_endpoint"`
	Audience           string `mapstructure:"audience" json:"audience" envconfig:"audience" yaml:"audience" toml:"audience"`
	AudienceRegex      string `mapstructure:"audience_regex" json:"audience_regex" envconfig:"audience_regex" yaml:"audience_regex" toml:"audience_regex"`
	Issuer             string `mapstructure:"issuer" json:"issuer" envconfig:"issuer" yaml:"issuer" toml:"issuer"`
	IssuerRegex        string `mapstructure:"issuer_regex" json:"issuer_regex" envconfig:"issuer_regex" yaml:"issuer_regex" toml:"issuer_regex"`
	UserIDClaim        string `mapstructure:"user_id_claim" json:"user_id_claim" envconfig:"user_id_claim" yaml:"user_id_claim" toml:"user_id_claim"`
}

// SubscriptionToken can be used to set custom configuration for subscription tokens.
type SubscriptionToken struct {
	Enabled bool `mapstructure:"enabled" json:"enabled" envconfig:"enabled"  yaml:"enabled" toml:"enabled"`
	Token   `mapstructure:",squash" yaml:",inline"`
}

// HTTP3 is EXPERIMENTAL.
type HTTP3 struct {
	Enabled bool `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled"`
}

// WebSocket client real-time transport configuration.
type WebSocket struct {
	Disabled           bool     `mapstructure:"disabled" json:"disabled" envconfig:"disabled" yaml:"disabled" toml:"disabled"`
	HandlerPrefix      string   `mapstructure:"handler_prefix" json:"handler_prefix" envconfig:"handler_prefix" default:"/connection/websocket" yaml:"handler_prefix" toml:"handler_prefix"`
	Compression        bool     `mapstructure:"compression" json:"compression" envconfig:"compression" yaml:"compression" toml:"compression"`
	CompressionMinSize int      `mapstructure:"compression_min_size" json:"compression_min_size" envconfig:"compression_min_size" yaml:"compression_min_size" toml:"compression_min_size"`
	CompressionLevel   int      `mapstructure:"compression_level" json:"compression_level" envconfig:"compression_level" default:"1" yaml:"compression_level" toml:"compression_level"`
	ReadBufferSize     int      `mapstructure:"read_buffer_size" json:"read_buffer_size" envconfig:"read_buffer_size" yaml:"read_buffer_size" toml:"read_buffer_size"`
	UseWriteBufferPool bool     `mapstructure:"use_write_buffer_pool" json:"use_write_buffer_pool" envconfig:"use_write_buffer_pool" yaml:"use_write_buffer_pool" toml:"use_write_buffer_pool"`
	WriteBufferSize    int      `mapstructure:"write_buffer_size" json:"write_buffer_size" envconfig:"write_buffer_size" yaml:"write_buffer_size" toml:"write_buffer_size"`
	WriteTimeout       Duration `mapstructure:"write_timeout" json:"write_timeout" envconfig:"write_timeout" default:"1000ms" yaml:"write_timeout" toml:"write_timeout"`
	MessageSizeLimit   int      `mapstructure:"message_size_limit" json:"message_size_limit" envconfig:"message_size_limit" default:"65536" yaml:"message_size_limit" toml:"message_size_limit"`
}

// SSE client real-time transport configuration.
type SSE struct {
	Enabled            bool   `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled"`
	HandlerPrefix      string `mapstructure:"handler_prefix" json:"handler_prefix" envconfig:"handler_prefix" default:"/connection/sse" yaml:"handler_prefix" toml:"handler_prefix"`
	MaxRequestBodySize int    `mapstructure:"max_request_body_size" json:"max_request_body_size" envconfig:"max_request_body_size" default:"65536" yaml:"max_request_body_size" toml:"max_request_body_size"`
}

// HTTPStream client real-time transport configuration.
type HTTPStream struct {
	Enabled            bool   `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled"`
	HandlerPrefix      string `mapstructure:"handler_prefix" json:"handler_prefix" envconfig:"handler_prefix" default:"/connection/http_stream" yaml:"handler_prefix" toml:"handler_prefix"`
	MaxRequestBodySize int    `mapstructure:"max_request_body_size" json:"max_request_body_size" envconfig:"max_request_body_size" default:"65536" yaml:"max_request_body_size" toml:"max_request_body_size"`
}

// WebTransport client real-time transport configuration.
type WebTransport struct {
	Enabled          bool   `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled"`
	HandlerPrefix    string `mapstructure:"handler_prefix" json:"handler_prefix" envconfig:"handler_prefix" default:"/connection/webtransport" yaml:"handler_prefix" toml:"handler_prefix"`
	MessageSizeLimit int    `mapstructure:"message_size_limit" json:"message_size_limit" envconfig:"message_size_limit" default:"65536" yaml:"message_size_limit" toml:"message_size_limit"`
}

// UniWebSocket client real-time transport configuration.
type UniWebSocket struct {
	Enabled            bool     `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled"`
	HandlerPrefix      string   `mapstructure:"handler_prefix" json:"handler_prefix" envconfig:"handler_prefix" default:"/connection/uni_websocket" yaml:"handler_prefix" toml:"handler_prefix"`
	Compression        bool     `mapstructure:"compression" json:"compression" envconfig:"compression" yaml:"compression" toml:"compression"`
	CompressionMinSize int      `mapstructure:"compression_min_size" json:"compression_min_size" envconfig:"compression_min_size" yaml:"compression_min_size" toml:"compression_min_size"`
	CompressionLevel   int      `mapstructure:"compression_level" json:"compression_level" envconfig:"compression_level" default:"1" yaml:"compression_level" toml:"compression_level"`
	ReadBufferSize     int      `mapstructure:"read_buffer_size" json:"read_buffer_size" envconfig:"read_buffer_size" yaml:"read_buffer_size" toml:"read_buffer_size"`
	UseWriteBufferPool bool     `mapstructure:"use_write_buffer_pool" json:"use_write_buffer_pool" envconfig:"use_write_buffer_pool" yaml:"use_write_buffer_pool" toml:"use_write_buffer_pool"`
	WriteBufferSize    int      `mapstructure:"write_buffer_size" json:"write_buffer_size" envconfig:"write_buffer_size" yaml:"write_buffer_size" toml:"write_buffer_size"`
	WriteTimeout       Duration `mapstructure:"write_timeout" json:"write_timeout" envconfig:"write_timeout" default:"1000ms" yaml:"write_timeout" toml:"write_timeout"`
	MessageSizeLimit   int      `mapstructure:"message_size_limit" json:"message_size_limit" envconfig:"message_size_limit" default:"65536" yaml:"message_size_limit" toml:"message_size_limit"`

	// JoinPushMessages when enabled allow uni_websocket transport to join messages together into
	// one frame using Centrifugal client protocol delimiters: new line for JSON protocol and
	// length-prefixed format for Protobuf protocol. This can be useful to reduce system call
	// overhead when sending many small messages. The client side must be ready to handle such
	// joined messages coming in one WebSocket frame.
	JoinPushMessages bool `mapstructure:"join_push_messages" json:"join_push_messages" envconfig:"join_push_messages" yaml:"join_push_messages" toml:"join_push_messages"`
}

// UniHTTPStream client real-time transport configuration.
type UniHTTPStream struct {
	Enabled                   bool                      `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled"`
	HandlerPrefix             string                    `mapstructure:"handler_prefix" json:"handler_prefix" envconfig:"handler_prefix" default:"/connection/uni_http_stream" yaml:"handler_prefix" toml:"handler_prefix"`
	MaxRequestBodySize        int                       `mapstructure:"max_request_body_size" json:"max_request_body_size" envconfig:"max_request_body_size" default:"65536" yaml:"max_request_body_size" toml:"max_request_body_size"`
	ConnectCodeToHTTPResponse ConnectCodeToHTTPResponse `mapstructure:"connect_code_to_http_response" json:"connect_code_to_http_response" envconfig:"connect_code_to_http_response" yaml:"connect_code_to_http_response" toml:"connect_code_to_http_response"`
}

// UniSSE client real-time transport configuration.
type UniSSE struct {
	Enabled                   bool                      `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled"`
	HandlerPrefix             string                    `mapstructure:"handler_prefix" json:"handler_prefix" envconfig:"handler_prefix" default:"/connection/uni_sse" yaml:"handler_prefix" toml:"handler_prefix"`
	MaxRequestBodySize        int                       `mapstructure:"max_request_body_size" json:"max_request_body_size" envconfig:"max_request_body_size" default:"65536" yaml:"max_request_body_size" toml:"max_request_body_size"`
	ConnectCodeToHTTPResponse ConnectCodeToHTTPResponse `mapstructure:"connect_code_to_http_response" json:"connect_code_to_http_response" envconfig:"connect_code_to_http_response" yaml:"connect_code_to_http_response" toml:"connect_code_to_http_response"`
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
	Enabled    bool                                `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled"`
	Transforms ConnectCodeToHTTPResponseTransforms `mapstructure:"transforms" default:"[]" json:"transforms" envconfig:"transforms" yaml:"transforms" toml:"transforms"`
}

type ConnectCodeToHTTPResponseTransform struct {
	Code uint32                              `mapstructure:"code" json:"code" envconfig:"code" yaml:"code" toml:"code"`
	To   TransformedConnectErrorHttpResponse `mapstructure:"to" json:"to" envconfig:"to" yaml:"to" toml:"to"`
}

type TransformedConnectErrorHttpResponse struct {
	StatusCode int    `mapstructure:"status_code" json:"status_code" envconfig:"status_code" yaml:"status_code" toml:"status_code"`
	Body       string `mapstructure:"body" json:"body" envconfig:"body" yaml:"body" toml:"body"`
}

// UniGRPC client real-time transport configuration.
type UniGRPC struct {
	Enabled               bool      `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled"`
	Address               string    `mapstructure:"address" json:"address" envconfig:"address" yaml:"address" toml:"address"`
	Port                  int       `mapstructure:"port" json:"port" envconfig:"port" default:"11000" yaml:"port" toml:"port"`
	MaxReceiveMessageSize int       `mapstructure:"max_receive_message_size" json:"max_receive_message_size" envconfig:"max_receive_message_size" yaml:"max_receive_message_size" toml:"max_receive_message_size"`
	TLS                   TLSConfig `mapstructure:"tls" json:"tls" envconfig:"tls" yaml:"tls" toml:"tls"`
}

// PingPong allows configuring application level ping-pong behavior.
// Note that in current implementation PingPongConfig.PingInterval must be greater than PingPongConfig.PongTimeout.
type PingPong struct {
	// PingInterval tells how often to issue server-to-client pings.
	// To disable sending app-level pings use -1.
	PingInterval Duration `mapstructure:"ping_interval" json:"ping_interval" envconfig:"ping_interval" default:"25s" yaml:"ping_interval" toml:"ping_interval"`
	// PongTimeout sets time for pong check after issuing a ping. To disable pong checks use -1.
	// PongTimeout must be less than PingInterval in current implementation.
	PongTimeout Duration `mapstructure:"pong_timeout" json:"pong_timeout" envconfig:"pong_timeout" default:"8s" yaml:"pong_timeout" toml:"pong_timeout"`
}

// NatsBroker configuration.
type NatsBroker struct {
	// URL is a Nats server URL.
	URL string `mapstructure:"url" json:"url" envconfig:"url" yaml:"url" toml:"url" default:"nats://localhost:4222"`
	// Prefix allows customizing channel prefix in Nats to work with a single Nats from different
	// unrelated Centrifugo setups.
	Prefix string `mapstructure:"prefix" default:"centrifugo" json:"prefix" envconfig:"prefix" yaml:"prefix" toml:"prefix"`
	// DialTimeout is a timeout for establishing connection to Nats.
	DialTimeout Duration `mapstructure:"dial_timeout" default:"1s" json:"dial_timeout" envconfig:"dial_timeout" yaml:"dial_timeout" toml:"dial_timeout"`
	// WriteTimeout is a timeout for write operation to Nats.
	WriteTimeout Duration `mapstructure:"write_timeout" default:"1s" json:"write_timeout" envconfig:"write_timeout" yaml:"write_timeout" toml:"write_timeout"`
	// TLS for the Nats connection. TLS is not used if nil.
	TLS TLSConfig `mapstructure:"tls" json:"tls" envconfig:"tls" yaml:"tls" toml:"tls"`

	// AllowWildcards allows to enable wildcard subscriptions. By default, wildcard subscriptions
	// are not allowed. Using wildcard subscriptions can't be combined with join/leave events and presence
	// because subscriptions do not belong to a concrete channel after with wildcards, while join/leave events
	// require concrete channel to be published. And presence does not make a lot of sense for wildcard
	// subscriptions - there could be subscribers which use different mask, but still receive subset of updates.
	// It's required to use channels without wildcards to for mentioned features to work properly. When
	// using wildcard subscriptions a special care is needed regarding security - pay additional
	// attention to a proper permission management.
	AllowWildcards bool `mapstructure:"allow_wildcards" json:"allow_wildcards" envconfig:"allow_wildcards" yaml:"allow_wildcards" toml:"allow_wildcards"`

	// RawMode allows enabling raw communication with Nats. When on, Centrifugo subscribes to channels
	// without adding any prefixes to channel name. Proper prefixes must be managed by the application in this
	// case. Data consumed from Nats is sent directly to subscribers without any processing. When publishing
	// to Nats Centrifugo does not add any prefixes to channel names also. Centrifugo features like Publication
	// tags, Publication ClientInfo, join/leave events are not supported in raw mode.
	RawMode RawModeConfig `mapstructure:"raw_mode" json:"raw_mode" envconfig:"raw_mode" yaml:"raw_mode" toml:"raw_mode"`
}

type RawModeConfig struct {
	// Enabled enables raw mode when true.
	Enabled bool `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled"`

	// ChannelReplacements is a map where keys are strings to replace and values are replacements.
	// For example, you have Centrifugo namespace "chat" and using channel "chat:index", but you want to
	// use channel "chat.index" in Nats. Then you can define SymbolReplacements map like this: {":": "."}.
	// In this case Centrifugo will replace all ":" symbols in channel name with "." before sending to Nats.
	// Broker keeps reverse mapping to the original channel to broadcast to proper channels when processing
	// messages received from Nats.
	ChannelReplacements MapStringString `mapstructure:"channel_replacements" default:"{}" json:"channel_replacements" envconfig:"channel_replacements" yaml:"channel_replacements" toml:"channel_replacements"`

	// Prefix is a string that will be added to all channels when publishing messages to Nats, subscribing
	// to channels in Nats. It's also stripped from channel name when processing messages received from Nats.
	// By default, no prefix is used.
	Prefix string `mapstructure:"prefix" json:"prefix" envconfig:"prefix" yaml:"prefix" toml:"prefix"`
}

type OpenTelemetry struct {
	Enabled   bool `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled"`
	API       bool `mapstructure:"api" json:"api" envconfig:"api" yaml:"api" toml:"api"`
	Consuming bool `mapstructure:"consuming" json:"consuming" envconfig:"consuming" yaml:"consuming" toml:"consuming"`
}

type HttpAPI struct {
	Disabled      bool   `mapstructure:"disabled" json:"disabled" envconfig:"disabled" yaml:"disabled" toml:"disabled"`
	HandlerPrefix string `mapstructure:"handler_prefix" json:"handler_prefix" envconfig:"handler_prefix" default:"/api" yaml:"handler_prefix" toml:"handler_prefix"`
	Key           string `mapstructure:"key" json:"key" envconfig:"key" yaml:"key" toml:"key"`
	ErrorMode     string `mapstructure:"error_mode" json:"error_mode" envconfig:"error_mode" yaml:"error_mode" toml:"error_mode"`
	External      bool   `mapstructure:"external" json:"external" envconfig:"external" yaml:"external" toml:"external"`
	Insecure      bool   `mapstructure:"insecure" json:"insecure" envconfig:"insecure" yaml:"insecure" toml:"insecure"`
}

type GrpcAPI struct {
	Enabled               bool      `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled"`
	ErrorMode             string    `mapstructure:"error_mode" json:"error_mode" envconfig:"error_mode" yaml:"error_mode" toml:"error_mode"`
	Address               string    `mapstructure:"address" json:"address" envconfig:"address" yaml:"address" toml:"address"`
	Port                  int       `mapstructure:"port" json:"port" envconfig:"port" default:"10000" yaml:"port" toml:"port"`
	Key                   string    `mapstructure:"key" json:"key" envconfig:"key" yaml:"key" toml:"key"`
	TLS                   TLSConfig `mapstructure:"tls" json:"tls" envconfig:"tls" yaml:"tls" toml:"tls"`
	Reflection            bool      `mapstructure:"reflection" json:"reflection" envconfig:"reflection" yaml:"reflection" toml:"reflection"`
	MaxReceiveMessageSize int       `mapstructure:"max_receive_message_size" json:"max_receive_message_size" envconfig:"max_receive_message_size" yaml:"max_receive_message_size" toml:"max_receive_message_size"`
}

type Graphite struct {
	Enabled  bool     `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled"`
	Host     string   `mapstructure:"host" json:"host" envconfig:"host" default:"localhost" yaml:"host" toml:"host"`
	Port     int      `mapstructure:"port" json:"port" envconfig:"port" default:"2003" yaml:"port" toml:"port"`
	Prefix   string   `mapstructure:"prefix" json:"prefix" envconfig:"prefix" default:"centrifugo" yaml:"prefix" toml:"prefix"`
	Interval Duration `mapstructure:"interval" json:"interval" envconfig:"interval" default:"10s" yaml:"interval" toml:"interval"`
	Tags     bool     `mapstructure:"tags" json:"tags" envconfig:"tags" yaml:"tags" toml:"tags"`
}

type Emulation struct {
	HandlerPrefix      string `mapstructure:"handler_prefix" json:"handler_prefix" envconfig:"handler_prefix" default:"/emulation" yaml:"handler_prefix" toml:"handler_prefix"`
	MaxRequestBodySize int    `mapstructure:"max_request_body_size" json:"max_request_body_size" envconfig:"max_request_body_size" default:"65536" yaml:"max_request_body_size" toml:"max_request_body_size"`
}

type UsageStats struct {
	Disabled bool `mapstructure:"disabled" json:"disabled" envconfig:"disabled" yaml:"disabled" toml:"disabled"`
}

type Prometheus struct {
	Enabled                bool   `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled"`
	HandlerPrefix          string `mapstructure:"handler_prefix" json:"handler_prefix" envconfig:"handler_prefix" default:"/metrics" yaml:"handler_prefix" toml:"handler_prefix"`
	InstrumentHTTPHandlers bool   `mapstructure:"instrument_http_handlers" json:"instrument_http_handlers" envconfig:"instrument_http_handlers" yaml:"instrument_http_handlers" toml:"instrument_http_handlers"`
}

type Health struct {
	Enabled       bool   `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled"`
	HandlerPrefix string `mapstructure:"handler_prefix" json:"handler_prefix" envconfig:"handler_prefix" default:"/health" yaml:"handler_prefix" toml:"handler_prefix"`
}

type Swagger struct {
	Enabled       bool   `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled"`
	HandlerPrefix string `mapstructure:"handler_prefix" json:"handler_prefix" envconfig:"handler_prefix" default:"/swagger" yaml:"handler_prefix" toml:"handler_prefix"`
}

type Debug struct {
	Enabled       bool   `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled"`
	HandlerPrefix string `mapstructure:"handler_prefix" json:"handler_prefix" envconfig:"handler_prefix" default:"/debug/pprof" yaml:"handler_prefix" toml:"handler_prefix"`
}

type Shutdown struct {
	Timeout Duration `mapstructure:"timeout" json:"timeout" envconfig:"timeout" default:"30s" yaml:"timeout" toml:"timeout"`
}

type ConnectProxy struct {
	// Enabled must be true to tell Centrifugo to enable the configured proxy.
	Enabled bool `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled"`
	Proxy   `mapstructure:",squash" yaml:",inline"`
}

type RefreshProxy struct {
	// Enabled must be true to tell Centrifugo to enable the configured proxy.
	Enabled bool `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled"`
	Proxy   `mapstructure:",squash" yaml:",inline"`
}

type ClientProxyContainer struct {
	// Connect proxy when enabled is used to proxy connect requests from client to the application backend.
	// Only requests without JWT token are proxied at this point.
	Connect ConnectProxy `mapstructure:"connect" json:"connect" envconfig:"connect" yaml:"connect" toml:"connect"`
	// Refresh proxy when enabled is used to proxy client connection refresh decisions to the application backend.
	Refresh RefreshProxy `mapstructure:"refresh" json:"refresh" envconfig:"refresh" yaml:"refresh" toml:"refresh"`
}

type Client struct {
	// Proxy is a configuration for client connection-wide proxies.
	Proxy ClientProxyContainer `mapstructure:"proxy" json:"proxy" envconfig:"proxy" toml:"proxy" yaml:"proxy"`
	// AllowedOrigins is a list of allowed origins for client connections.
	AllowedOrigins []string `mapstructure:"allowed_origins" json:"allowed_origins" envconfig:"allowed_origins" yaml:"allowed_origins" toml:"allowed_origins"`
	// Token is a configuration for token generation and verification. When enabled, this configuration
	// is used for both connection and subscription tokens. See also SubscriptionToken to use a separate
	// configuration for subscription tokens.
	Token Token `mapstructure:"token" json:"token" envconfig:"token" yaml:"token" toml:"token"`
	// SubscriptionToken is a configuration for subscription token generation and verification. When enabled,
	// Centrifugo will use this configuration for subscription tokens only. Configuration in Token is then only
	// used for connection tokens.
	SubscriptionToken SubscriptionToken `mapstructure:"subscription_token" json:"subscription_token" envconfig:"subscription_token" yaml:"subscription_token" toml:"subscription_token"`
	// AllowAnonymousConnectWithoutToken allows to connect to Centrifugo without a token. In this case connection will
	// be accepted but client will be anonymous (i.e. will have empty user ID).
	AllowAnonymousConnectWithoutToken bool `mapstructure:"allow_anonymous_connect_without_token" json:"allow_anonymous_connect_without_token" envconfig:"allow_anonymous_connect_without_token" yaml:"allow_anonymous_connect_without_token" toml:"allow_anonymous_connect_without_token"`
	// DisallowAnonymousConnectionTokens disallows anonymous connection tokens. When enabled, Centrifugo will not
	// accept connection tokens with empty user ID.
	DisallowAnonymousConnectionTokens bool `mapstructure:"disallow_anonymous_connection_tokens" json:"disallow_anonymous_connection_tokens" envconfig:"disallow_anonymous_connection_tokens" yaml:"disallow_anonymous_connection_tokens" toml:"disallow_anonymous_connection_tokens"`
	// PingPong allows configuring application level ping-pong behavior for client connections.
	PingPong `mapstructure:",squash" yaml:",inline"`

	ExpiredCloseDelay                Duration `mapstructure:"expired_close_delay" json:"expired_close_delay" envconfig:"expired_close_delay" default:"25s" yaml:"expired_close_delay" toml:"expired_close_delay"`
	ExpiredSubCloseDelay             Duration `mapstructure:"expired_sub_close_delay" json:"expired_sub_close_delay" envconfig:"expired_sub_close_delay" default:"25s" yaml:"expired_sub_close_delay" toml:"expired_sub_close_delay"`
	StaleCloseDelay                  Duration `mapstructure:"stale_close_delay" json:"stale_close_delay" envconfig:"stale_close_delay" default:"10s" yaml:"stale_close_delay" toml:"stale_close_delay"`
	ChannelLimit                     int      `mapstructure:"channel_limit" json:"channel_limit" envconfig:"channel_limit" default:"128" yaml:"channel_limit" toml:"channel_limit"`
	QueueMaxSize                     int      `mapstructure:"queue_max_size" json:"queue_max_size" envconfig:"queue_max_size" default:"1048576" yaml:"queue_max_size" toml:"queue_max_size"`
	PresenceUpdateInterval           Duration `mapstructure:"presence_update_interval" json:"presence_update_interval" envconfig:"presence_update_interval" default:"27s" yaml:"presence_update_interval" toml:"presence_update_interval"`
	Concurrency                      int      `mapstructure:"concurrency" json:"concurrency" envconfig:"concurrency" yaml:"concurrency" toml:"concurrency"`
	ChannelPositionCheckDelay        Duration `mapstructure:"channel_position_check_delay" json:"channel_position_check_delay" envconfig:"channel_position_check_delay" default:"40s" yaml:"channel_position_check_delay" toml:"channel_position_check_delay"`
	ChannelPositionMaxTimeLag        Duration `mapstructure:"channel_position_max_time_lag" json:"channel_position_max_time_lag" envconfig:"channel_position_max_time_lag" yaml:"channel_position_max_time_lag" toml:"channel_position_max_time_lag"`
	ConnectionLimit                  int      `mapstructure:"connection_limit" json:"connection_limit" envconfig:"connection_limit" yaml:"connection_limit" toml:"connection_limit"`
	UserConnectionLimit              int      `mapstructure:"user_connection_limit" json:"user_connection_limit" envconfig:"user_connection_limit" yaml:"user_connection_limit" toml:"user_connection_limit"`
	ConnectionRateLimit              int      `mapstructure:"connection_rate_limit" json:"connection_rate_limit" envconfig:"connection_rate_limit" yaml:"connection_rate_limit" toml:"connection_rate_limit"`
	ConnectIncludeServerTime         bool     `mapstructure:"connect_include_server_time" json:"connect_include_server_time" envconfig:"connect_include_server_time" yaml:"connect_include_server_time" toml:"connect_include_server_time"`
	HistoryMaxPublicationLimit       int      `mapstructure:"history_max_publication_limit" json:"history_max_publication_limit" envconfig:"history_max_publication_limit" default:"300" yaml:"history_max_publication_limit" toml:"history_max_publication_limit"`
	RecoveryMaxPublicationLimit      int      `mapstructure:"recovery_max_publication_limit" json:"recovery_max_publication_limit" envconfig:"recovery_max_publication_limit" default:"300" yaml:"recovery_max_publication_limit" toml:"recovery_max_publication_limit"`
	InsecureSkipTokenSignatureVerify bool     `mapstructure:"insecure_skip_token_signature_verify" json:"insecure_skip_token_signature_verify" envconfig:"insecure_skip_token_signature_verify" yaml:"insecure_skip_token_signature_verify" toml:"insecure_skip_token_signature_verify"`
	UserIDHTTPHeader                 string   `mapstructure:"user_id_http_header" json:"user_id_http_header" envconfig:"user_id_http_header" yaml:"user_id_http_header" toml:"user_id_http_header"`
	Insecure                         bool     `mapstructure:"insecure" json:"insecure" envconfig:"insecure" yaml:"insecure" toml:"insecure"`

	// SubscribeToUserPersonalChannel is a configuration for a feature to automatically subscribe user to a personal channel
	// using server-side subscription.
	SubscribeToUserPersonalChannel SubscribeToUserPersonalChannel `mapstructure:"subscribe_to_user_personal_channel" json:"subscribe_to_user_personal_channel" envconfig:"subscribe_to_user_personal_channel" yaml:"subscribe_to_user_personal_channel" toml:"subscribe_to_user_personal_channel"`

	// ConnectCodeToDisconnect is a configuration for a feature to transform connect error codes to the disconnect code
	// for unidirectional transports.
	ConnectCodeToUnidirectionalDisconnect ConnectCodeToUnidirectionalDisconnect `mapstructure:"connect_code_to_unidirectional_disconnect" json:"connect_code_to_unidirectional_disconnect" envconfig:"connect_code_to_unidirectional_disconnect" yaml:"connect_code_to_unidirectional_disconnect" toml:"connect_code_to_unidirectional_disconnect"`
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
	Enabled    bool                                 `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled"`
	Transforms UniConnectCodeToDisconnectTransforms `mapstructure:"transforms" default:"[]" json:"transforms" envconfig:"transforms" yaml:"transforms" toml:"transforms"`
}

type UniConnectCodeToDisconnectTransform struct {
	Code uint32              `mapstructure:"code" json:"code" envconfig:"code" yaml:"code" toml:"code"`
	To   TransformDisconnect `mapstructure:"to" json:"to" envconfig:"to" yaml:"to" toml:"to"`
}

type ChannelProxyContainer struct {
	Subscribe       Proxy `mapstructure:"subscribe" json:"subscribe" envconfig:"subscribe" yaml:"subscribe" toml:"subscribe"`
	Publish         Proxy `mapstructure:"publish" json:"publish" envconfig:"publish" yaml:"publish" toml:"publish"`
	SubRefresh      Proxy `mapstructure:"sub_refresh" json:"sub_refresh" envconfig:"sub_refresh" yaml:"sub_refresh" toml:"sub_refresh"`
	SubscribeStream Proxy `mapstructure:"subscribe_stream" json:"subscribe_stream" envconfig:"subscribe_stream" yaml:"subscribe_stream" toml:"subscribe_stream"`
}

type Channel struct {
	// Proxy configuration for channel-related events. All types inside can be referenced by the name "default".
	Proxy ChannelProxyContainer `mapstructure:"proxy" json:"proxy" envconfig:"proxy" toml:"proxy" yaml:"proxy"`

	// WithoutNamespace is a configuration of channels options for channels which do not have namespace.
	// Generally, we recommend always use channel namespaces but this option can be useful for simple setups.
	WithoutNamespace ChannelOptions `mapstructure:"without_namespace" json:"without_namespace" envconfig:"without_namespace" yaml:"without_namespace" toml:"without_namespace"`
	// Namespaces is a list of channel namespaces. Each channel namespace can have its own set of rules.
	Namespaces ChannelNamespaces `mapstructure:"namespaces" default:"[]" json:"namespaces" envconfig:"namespaces" yaml:"namespaces" toml:"namespaces"`

	// HistoryTTL is a time how long to keep history meta information. This is a global option for all channels,
	// but it can be overridden in channel namespace.
	HistoryMetaTTL Duration `mapstructure:"history_meta_ttl" json:"history_meta_ttl" envconfig:"history_meta_ttl" default:"720h" yaml:"history_meta_ttl" toml:"history_meta_ttl"`

	MaxLength         int    `mapstructure:"max_length" json:"max_length" envconfig:"max_length" default:"255" yaml:"max_length" toml:"max_length"`
	PrivatePrefix     string `mapstructure:"private_prefix" json:"private_prefix" envconfig:"private_prefix" default:"$" yaml:"private_prefix" toml:"private_prefix"`
	NamespaceBoundary string `mapstructure:"namespace_boundary" json:"namespace_boundary" envconfig:"namespace_boundary" default:":" yaml:"namespace_boundary" toml:"namespace_boundary"`
	UserBoundary      string `mapstructure:"user_boundary" json:"user_boundary" envconfig:"user_boundary" default:"#" yaml:"user_boundary" toml:"user_boundary"`
	UserSeparator     string `mapstructure:"user_separator" json:"user_separator" envconfig:"user_separator" default:"," yaml:"user_separator" toml:"user_separator"`
}

type RPC struct {
	// Proxy configuration for rpc-related events. Can be referenced by the name "default".
	Proxy Proxy `mapstructure:"proxy" json:"proxy" envconfig:"proxy" toml:"proxy" yaml:"proxy"`
	// WithoutNamespace is a configuration of RpcOptions for rpc methods without rpc namespace. Generally,
	// we recommend always use rpc namespaces but this option can be useful for simple setups.
	WithoutNamespace RpcOptions `mapstructure:"without_namespace" json:"without_namespace" envconfig:"without_namespace" yaml:"without_namespace" toml:"without_namespace"`
	// RPCNamespaces is a list of rpc namespaces. Each rpc namespace can have its own set of rules.
	Namespaces RPCNamespaces `mapstructure:"namespaces" default:"[]" json:"namespaces" envconfig:"namespaces" yaml:"namespaces" toml:"namespaces"`
	// Ping is a configuration for RPC ping method.
	Ping              RPCPing `mapstructure:"ping" json:"ping" envconfig:"ping" yaml:"ping" toml:"ping"`
	NamespaceBoundary string  `mapstructure:"namespace_boundary" json:"namespace_boundary" envconfig:"namespace_boundary" default:":" yaml:"namespace_boundary" toml:"namespace_boundary"`
}

type RPCPing struct {
	Enabled bool   `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled"`
	Method  string `mapstructure:"method" json:"method" envconfig:"method" default:"ping" yaml:"method" toml:"method"`
}

type NamedProxy struct {
	Name  string `mapstructure:"name" json:"name" envconfig:"name" yaml:"name" toml:"name"`
	Proxy `mapstructure:",squash" yaml:",inline"`
}

type NamedProxies []NamedProxy

// Decode to implement the envconfig.Decoder interface
func (d *NamedProxies) Decode(value string) error {
	return decodeToNamedSlice(value, d)
}

type SubscribeToUserPersonalChannel struct {
	Enabled                  bool   `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled"`
	PersonalChannelNamespace string `mapstructure:"personal_channel_namespace" json:"personal_channel_namespace" envconfig:"personal_channel_namespace" yaml:"personal_channel_namespace" toml:"personal_channel_namespace"`
	SingleConnection         bool   `mapstructure:"single_connection" json:"single_connection" yaml:"single_connection" toml:"single_connection" envconfig:"single_connection"`
}

type Node struct {
	// Name is a human-readable name of Centrifugo node in cluster. This must be unique for each running node
	// in a cluster. By default, Centrifugo constructs name from the hostname and port. Name is shown in admin web
	// interface. For communication between nodes in a cluster, Centrifugo uses another identifier – unique ID
	// generated on node start, so node name plays just a human-readable identifier role.
	Name string `mapstructure:"name" json:"name" envconfig:"name" yaml:"name" toml:"name"`
	// InfoMetricsAggregateInterval is a time interval to aggregate node info metrics.
	InfoMetricsAggregateInterval Duration `mapstructure:"info_metrics_aggregate_interval" json:"info_metrics_aggregate_interval" envconfig:"info_metrics_aggregate_interval" default:"60s" yaml:"info_metrics_aggregate_interval" toml:"info_metrics_aggregate_interval"`
}

type Admin struct {
	Enabled       bool   `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled"`
	HandlerPrefix string `mapstructure:"handler_prefix" json:"handler_prefix" envconfig:"handler_prefix" default:"" yaml:"handler_prefix" toml:"handler_prefix"`
	// Password is an admin password.
	Password string `mapstructure:"password" json:"password" envconfig:"password" yaml:"password" toml:"password"`
	// Secret is a secret to generate auth token for admin requests.
	Secret string `mapstructure:"secret" json:"secret" envconfig:"secret" yaml:"secret" toml:"secret"`
	// Insecure turns on insecure mode for admin endpoints - no auth
	// required to connect to web interface and for requests to admin API.
	// Admin resources must be protected by firewall rules in production when
	// this option enabled otherwise everyone from internet can make admin
	// actions.
	Insecure bool `mapstructure:"insecure" json:"insecure" envconfig:"insecure" yaml:"insecure" toml:"insecure"`
	// WebPath is path to admin web application to serve.
	WebPath string `mapstructure:"web_path" json:"web_path" envconfig:"web_path" yaml:"web_path" toml:"web_path"`
	// WebProxyAddress is an address for proxying to the running admin web application app.
	// So it's possible to run web app in dev mode and point Centrifugo to its address for
	// development purposes.
	WebProxyAddress string `mapstructure:"web_proxy_address" json:"web_proxy_address" envconfig:"web_proxy_address" yaml:"web_proxy_address" toml:"web_proxy_address"`
	// External is a flag to run admin interface on external port.
	External bool `mapstructure:"external" json:"external" envconfig:"external" yaml:"external" toml:"external"`
}

type TransformError struct {
	Code      uint32 `mapstructure:"code" json:"code" envconfig:"code" yaml:"code" toml:"code"`
	Message   string `mapstructure:"message" json:"message" envconfig:"message" yaml:"message" toml:"message"`
	Temporary bool   `mapstructure:"temporary" json:"temporary" envconfig:"temporary" yaml:"temporary" toml:"temporary"`
}

type TransformDisconnect struct {
	Code   uint32 `mapstructure:"code" json:"code" envconfig:"code" yaml:"code" toml:"code"`
	Reason string `mapstructure:"reason" json:"reason" envconfig:"reason" yaml:"reason" toml:"reason"`
}

type HttpStatusToCodeTransform struct {
	StatusCode   int                 `mapstructure:"status_code" json:"status_code" envconfig:"status_code" yaml:"status_code" toml:"status_code"`
	ToError      TransformError      `mapstructure:"to_error" json:"to_error" envconfig:"to_error" yaml:"to_error" toml:"to_error"`
	ToDisconnect TransformDisconnect `mapstructure:"to_disconnect" json:"to_disconnect" envconfig:"to_disconnect" yaml:"to_disconnect" toml:"to_disconnect"`
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
	TLS TLSConfig `mapstructure:"tls" json:"tls" envconfig:"tls" yaml:"tls" toml:"tls"`
	// StaticHeaders is a static set of key/value pairs to attach to HTTP proxy request as
	// headers. Headers received from HTTP client request or metadata from GRPC client request
	// both have priority over values set in StaticHttpHeaders map.
	StaticHeaders MapStringString `mapstructure:"static_headers" default:"{}" json:"static_headers" envconfig:"static_headers" yaml:"static_headers" toml:"static_headers"`
	// Status transforms allow to map HTTP status codes from proxy to Disconnect or Error messages.
	StatusToCodeTransforms HttpStatusToCodeTransforms `mapstructure:"status_to_code_transforms" default:"[]" json:"status_to_code_transforms" envconfig:"status_to_code_transforms" yaml:"status_to_code_transforms" toml:"status_to_code_transforms"`
}

type ProxyCommonGRPC struct {
	// TLS is a common configuration for GRPC TLS.
	TLS TLSConfig `mapstructure:"tls" json:"tls" envconfig:"tls" yaml:"tls" toml:"tls"`
	// CredentialsKey is a custom key to add into per-RPC credentials.
	CredentialsKey string `mapstructure:"credentials_key" json:"credentials_key" envconfig:"credentials_key" yaml:"credentials_key" toml:"credentials_key"`
	// GrpcCredentialsValue is a custom value for GrpcCredentialsKey.
	CredentialsValue string `mapstructure:"credentials_value" json:"credentials_value" envconfig:"credentials_value" yaml:"credentials_value" toml:"credentials_value"`
	// Compression enables compression for outgoing calls (gzip).
	Compression bool `mapstructure:"compression" json:"compression" envconfig:"compression" yaml:"compression" toml:"compression"`
}

type ProxyCommon struct {
	// HTTPHeaders is a list of HTTP headers to proxy. No headers used by proxy by default.
	// If GRPC proxy is used then request HTTP headers set to outgoing request metadata.
	HttpHeaders []string `mapstructure:"http_headers" json:"http_headers" envconfig:"http_headers" yaml:"http_headers" toml:"http_headers"`
	// GRPCMetadata is a list of GRPC metadata keys to proxy. No meta keys used by proxy by
	// default. If HTTP proxy is used then these keys become outgoing request HTTP headers.
	GrpcMetadata []string `mapstructure:"grpc_metadata" json:"grpc_metadata" envconfig:"grpc_metadata" yaml:"grpc_metadata" toml:"grpc_metadata"`
	// BinaryEncoding makes proxy send data as base64 string (assuming it contains custom
	// non-JSON payload).
	BinaryEncoding bool `mapstructure:"binary_encoding" json:"binary_encoding" envconfig:"binary_encoding" yaml:"binary_encoding" toml:"binary_encoding"`
	// IncludeConnectionMeta to each proxy request (except connect proxy where it's obtained).
	IncludeConnectionMeta bool `mapstructure:"include_connection_meta" json:"include_connection_meta" envconfig:"include_connection_meta" yaml:"include_connection_meta" toml:"include_connection_meta"`

	HTTP ProxyCommonHTTP `mapstructure:"http" json:"http" envconfig:"http" yaml:"http" toml:"http"`
	GRPC ProxyCommonGRPC `mapstructure:"grpc" json:"grpc" envconfig:"grpc" yaml:"grpc" toml:"grpc"`
}

// Proxy configuration.
type Proxy struct {
	// Endpoint - HTTP address or GRPC service endpoint.
	Endpoint string `mapstructure:"endpoint" json:"endpoint" envconfig:"endpoint" yaml:"endpoint" toml:"endpoint"`
	// Timeout for proxy request.
	Timeout Duration `mapstructure:"timeout" default:"1s" json:"timeout" envconfig:"timeout" yaml:"timeout" toml:"timeout"`

	ProxyCommon `mapstructure:",squash" yaml:",inline"`

	TestGrpcDialer func(context.Context, string) (net.Conn, error) `json:"-" yaml:"-" toml:"-" envconfig:"-"`
}

const (
	ConsumerTypePostgres = "postgresql"
	ConsumerTypeKafka    = "kafka"
)

var KnownConsumerTypes = []string{
	ConsumerTypePostgres,
	ConsumerTypeKafka,
}

type Consumer struct {
	// Name is a unique name required for each consumer.
	Name string `mapstructure:"name" json:"name" envconfig:"name" yaml:"name" toml:"name"`

	// Enabled must be true to tell Centrifugo to run configured consumer.
	Enabled bool `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled"`

	// Type describes the type of consumer.
	Type string `mapstructure:"type" json:"type" envconfig:"type" yaml:"type" toml:"type"`

	// Postgres allows defining options for consumer of postgresql type.
	Postgres PostgresConsumerConfig `mapstructure:"postgresql" json:"postgresql" envconfig:"postgresql" yaml:"postgresql" toml:"postgresql"`
	// Kafka allows defining options for consumer of kafka type.
	Kafka KafkaConsumerConfig `mapstructure:"kafka" json:"kafka" envconfig:"kafka" yaml:"kafka" toml:"kafka"`
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
	DSN                          string    `mapstructure:"dsn" json:"dsn" envconfig:"dsn" yaml:"dsn" toml:"dsn"`
	OutboxTableName              string    `mapstructure:"outbox_table_name" json:"outbox_table_name" envconfig:"outbox_table_name" yaml:"outbox_table_name" toml:"outbox_table_name"`
	NumPartitions                int       `mapstructure:"num_partitions" json:"num_partitions" envconfig:"num_partitions" default:"1" yaml:"num_partitions" toml:"num_partitions"`
	PartitionSelectLimit         int       `mapstructure:"partition_select_limit" json:"partition_select_limit" envconfig:"partition_select_limit" default:"100" yaml:"partition_select_limit" toml:"partition_select_limit"`
	PartitionPollInterval        Duration  `mapstructure:"partition_poll_interval" json:"partition_poll_interval" envconfig:"partition_poll_interval" default:"300ms" yaml:"partition_poll_interval" toml:"partition_poll_interval"`
	PartitionNotificationChannel string    `mapstructure:"partition_notification_channel" json:"partition_notification_channel" envconfig:"partition_notification_channel" yaml:"partition_notification_channel" toml:"partition_notification_channel"`
	TLS                          TLSConfig `mapstructure:"tls" json:"tls" envconfig:"tls" yaml:"tls" toml:"tls"`
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
	Brokers        []string `mapstructure:"brokers" json:"brokers" envconfig:"brokers" yaml:"brokers" toml:"brokers"`
	Topics         []string `mapstructure:"topics" json:"topics" envconfig:"topics" yaml:"topics" toml:"topics"`
	ConsumerGroup  string   `mapstructure:"consumer_group" json:"consumer_group" envconfig:"consumer_group" yaml:"consumer_group" toml:"consumer_group"`
	MaxPollRecords int      `mapstructure:"max_poll_records" json:"max_poll_records" envconfig:"max_poll_records" default:"100" yaml:"max_poll_records" toml:"max_poll_records"`

	// TLS for the connection to Kafka.
	TLS TLSConfig `mapstructure:"tls" json:"tls" envconfig:"tls" yaml:"tls" toml:"tls"`

	// SASLMechanism when not empty enables SASL auth.
	SASLMechanism string `mapstructure:"sasl_mechanism" json:"sasl_mechanism" envconfig:"sasl_mechanism" yaml:"sasl_mechanism" toml:"sasl_mechanism"`
	SASLUser      string `mapstructure:"sasl_user" json:"sasl_user" envconfig:"sasl_user" yaml:"sasl_user" toml:"sasl_user"`
	SASLPassword  string `mapstructure:"sasl_password" json:"sasl_password" envconfig:"sasl_password" yaml:"sasl_password" toml:"sasl_password"`

	// PartitionBufferSize is the size of the buffer for each partition consumer.
	// This is the number of records that can be buffered before the consumer
	// will pause fetching records from Kafka. By default, this is 16.
	// Set to -1 to use non-buffered channel.
	PartitionBufferSize int `mapstructure:"partition_buffer_size" json:"partition_buffer_size" envconfig:"partition_buffer_size" default:"16" yaml:"partition_buffer_size" toml:"partition_buffer_size"`

	// FetchMaxBytes is the maximum number of bytes to fetch from Kafka in a single request.
	// If not set the default 50MB is used.
	FetchMaxBytes int32 `mapstructure:"fetch_max_bytes" json:"fetch_max_bytes" envconfig:"fetch_max_bytes" yaml:"fetch_max_bytes" toml:"fetch_max_bytes"`

	// PublicationDataMode is a configuration for the mode where message payload already
	// contains data ready to publish into channels, instead of API command.
	PublicationDataMode KafkaPublicationDataModeConfig `mapstructure:"publication_data_mode" json:"publication_data_mode" envconfig:"publication_data_mode" yaml:"publication_data_mode" toml:"publication_data_mode"`
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
	return nil
}

// KafkaPublicationDataModeConfig is a configuration for Kafka publication data mode.
// In this mode we expect Kafka message payload to contain data ready to publish into
// channels, instead of API command. All other fields used to build channel Publication
// can be passed in Kafka message headers – thus it's possible to integrate existing
// topics with Centrifugo.
type KafkaPublicationDataModeConfig struct {
	// Enabled enables Kafka publication data mode for the Kafka consumer.
	Enabled bool `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled"`
	// ChannelsHeader is a header name to extract channels to publish data into
	// (channels must be comma-separated). Ex. of value: "channel1,channel2".
	ChannelsHeader string `mapstructure:"channels_header" json:"channels_header" envconfig:"channels_header" yaml:"channels_header" toml:"channels_header"`
	// IdempotencyKeyHeader is a header name to extract Publication idempotency key from
	// Kafka message. See https://centrifugal.dev/docs/server/server_api#publishrequest.
	IdempotencyKeyHeader string `mapstructure:"idempotency_key_header" json:"idempotency_key_header" envconfig:"idempotency_key_header" yaml:"idempotency_key_header" toml:"idempotency_key_header"`
	// DeltaHeader is a header name to extract Publication delta flag from Kafka message
	// which tells Centrifugo whether to use delta compression for message or not.
	// See https://centrifugal.dev/docs/server/delta_compression and
	// https://centrifugal.dev/docs/server/server_api#publishrequest.
	DeltaHeader string `mapstructure:"delta_header" json:"delta_header" envconfig:"delta_header" yaml:"delta_header" toml:"delta_header"`
}
