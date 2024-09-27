package configtypes

import (
	"context"
	"encoding/json"
	"fmt"
	"net"

	"github.com/centrifugal/centrifuge"
)

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

type SubscriptionToken struct {
	Enabled bool `mapstructure:"enabled" json:"enabled" envconfig:"enabled"  yaml:"enabled" toml:"enabled"`
	Token   `mapstructure:",squash" yaml:",inline"`
}

type HTTP3 struct {
	Enabled bool `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled"`
}

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

type SSE struct {
	Enabled            bool   `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled"`
	HandlerPrefix      string `mapstructure:"handler_prefix" json:"handler_prefix" envconfig:"handler_prefix" default:"/connection/sse" yaml:"handler_prefix" toml:"handler_prefix"`
	MaxRequestBodySize int    `mapstructure:"max_request_body_size" json:"max_request_body_size" envconfig:"max_request_body_size" default:"65536" yaml:"max_request_body_size" toml:"max_request_body_size"`
}

type HTTPStream struct {
	Enabled            bool   `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled"`
	HandlerPrefix      string `mapstructure:"handler_prefix" json:"handler_prefix" envconfig:"handler_prefix" default:"/connection/http_stream" yaml:"handler_prefix" toml:"handler_prefix"`
	MaxRequestBodySize int    `mapstructure:"max_request_body_size" json:"max_request_body_size" envconfig:"max_request_body_size" default:"65536" yaml:"max_request_body_size" toml:"max_request_body_size"`
}

type WebTransport struct {
	Enabled       bool   `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled"`
	HandlerPrefix string `mapstructure:"handler_prefix" json:"handler_prefix" envconfig:"handler_prefix" default:"/connection/webtransport" yaml:"handler_prefix" toml:"handler_prefix"`
}

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
}

type UniHTTPStream struct {
	Enabled            bool   `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled"`
	HandlerPrefix      string `mapstructure:"handler_prefix" json:"handler_prefix" envconfig:"handler_prefix" default:"/connection/uni_http_stream" yaml:"handler_prefix" toml:"handler_prefix"`
	MaxRequestBodySize int    `mapstructure:"max_request_body_size" json:"max_request_body_size" envconfig:"max_request_body_size" default:"65536" yaml:"max_request_body_size" toml:"max_request_body_size"`
}

type UniSSE struct {
	Enabled bool `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled"`

	HandlerPrefix string `mapstructure:"handler_prefix" json:"handler_prefix" envconfig:"handler_prefix" default:"/connection/uni_sse" yaml:"handler_prefix" toml:"handler_prefix"`

	// MaxRequestBodySize for initial POST requests (when POST is used).
	MaxRequestBodySize int `mapstructure:"max_request_body_size" json:"max_request_body_size" envconfig:"max_request_body_size" default:"65536" yaml:"max_request_body_size" toml:"max_request_body_size"`
}

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
	URL string `mapstructure:"url" json:"url" envconfig:"url" yaml:"url" toml:"url"`
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
	Enabled       bool   `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled"`
	HandlerPrefix string `mapstructure:"handler_prefix" json:"handler_prefix" envconfig:"handler_prefix" default:"/metrics" yaml:"handler_prefix" toml:"handler_prefix"`
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

type Client struct {
	// ConnectProxyName is a name of proxy to use for connect events. When not set connect events are not proxied.
	ConnectProxyName string `mapstructure:"connect_proxy_name" json:"connect_proxy_name" envconfig:"connect_proxy_name" yaml:"connect_proxy_name" toml:"connect_proxy_name"`
	// RefreshProxyName is a name of proxy to use for refresh events. When not set refresh events are not proxied.
	RefreshProxyName string `mapstructure:"refresh_proxy_name" json:"refresh_proxy_name" envconfig:"refresh_proxy_name" yaml:"refresh_proxy_name" toml:"refresh_proxy_name"`

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

	AllowedDeltaTypes                []centrifuge.DeltaType `mapstructure:"allowed_delta_types" json:"allowed_delta_types" envconfig:"allowed_delta_types" yaml:"allowed_delta_types" toml:"allowed_delta_types"`
	ExpiredCloseDelay                Duration               `mapstructure:"expired_close_delay" json:"expired_close_delay" envconfig:"expired_close_delay" default:"25s" yaml:"expired_close_delay" toml:"expired_close_delay"`
	ExpiredSubCloseDelay             Duration               `mapstructure:"expired_sub_close_delay" json:"expired_sub_close_delay" envconfig:"expired_sub_close_delay" default:"25s" yaml:"expired_sub_close_delay" toml:"expired_sub_close_delay"`
	StaleCloseDelay                  Duration               `mapstructure:"stale_close_delay" json:"stale_close_delay" envconfig:"stale_close_delay" default:"10s" yaml:"stale_close_delay" toml:"stale_close_delay"`
	ChannelLimit                     int                    `mapstructure:"channel_limit" json:"channel_limit" envconfig:"channel_limit" default:"128" yaml:"channel_limit" toml:"channel_limit"`
	QueueMaxSize                     int                    `mapstructure:"queue_max_size" json:"queue_max_size" envconfig:"queue_max_size" default:"1048576" yaml:"queue_max_size" toml:"queue_max_size"`
	PresenceUpdateInterval           Duration               `mapstructure:"presence_update_interval" json:"presence_update_interval" envconfig:"presence_update_interval" default:"27s" yaml:"presence_update_interval" toml:"presence_update_interval"`
	Concurrency                      int                    `mapstructure:"concurrency" json:"concurrency" envconfig:"concurrency" yaml:"concurrency" toml:"concurrency"`
	ChannelPositionCheckDelay        Duration               `mapstructure:"channel_position_check_delay" json:"channel_position_check_delay" envconfig:"channel_position_check_delay" default:"40s" yaml:"channel_position_check_delay" toml:"channel_position_check_delay"`
	ChannelPositionMaxTimeLag        Duration               `mapstructure:"channel_position_max_time_lag" json:"channel_position_max_time_lag" envconfig:"channel_position_max_time_lag" yaml:"channel_position_max_time_lag" toml:"channel_position_max_time_lag"`
	ConnectionLimit                  int                    `mapstructure:"connection_limit" json:"connection_limit" envconfig:"connection_limit" yaml:"connection_limit" toml:"connection_limit"`
	UserConnectionLimit              int                    `mapstructure:"user_connection_limit" json:"user_connection_limit" envconfig:"user_connection_limit" yaml:"user_connection_limit" toml:"user_connection_limit"`
	ConnectionRateLimit              int                    `mapstructure:"connection_rate_limit" json:"connection_rate_limit" envconfig:"connection_rate_limit" yaml:"connection_rate_limit" toml:"connection_rate_limit"`
	ConnectIncludeServerTime         bool                   `mapstructure:"connect_include_server_time" json:"connect_include_server_time" envconfig:"connect_include_server_time" yaml:"connect_include_server_time" toml:"connect_include_server_time"`
	HistoryMaxPublicationLimit       int                    `mapstructure:"history_max_publication_limit" json:"history_max_publication_limit" envconfig:"history_max_publication_limit" default:"300" yaml:"history_max_publication_limit" toml:"history_max_publication_limit"`
	RecoveryMaxPublicationLimit      int                    `mapstructure:"recovery_max_publication_limit" json:"recovery_max_publication_limit" envconfig:"recovery_max_publication_limit" default:"300" yaml:"recovery_max_publication_limit" toml:"recovery_max_publication_limit"`
	InsecureSkipTokenSignatureVerify bool                   `mapstructure:"insecure_skip_token_signature_verify" json:"insecure_skip_token_signature_verify" envconfig:"insecure_skip_token_signature_verify" yaml:"insecure_skip_token_signature_verify" toml:"insecure_skip_token_signature_verify"`
	UserIDHTTPHeader                 string                 `mapstructure:"user_id_http_header" json:"user_id_http_header" envconfig:"user_id_http_header" yaml:"user_id_http_header" toml:"user_id_http_header"`
	Insecure                         bool                   `mapstructure:"insecure" json:"insecure" envconfig:"insecure" yaml:"insecure" toml:"insecure"`

	// SubscribeToUserPersonalChannel is a configuration for a feature to automatically subscribe user to a personal channel
	// using server-side subscription.
	SubscribeToUserPersonalChannel SubscribeToUserPersonalChannel `mapstructure:"subscribe_to_user_personal_channel" json:"subscribe_to_user_personal_channel" envconfig:"subscribe_to_user_personal_channel" yaml:"subscribe_to_user_personal_channel" toml:"subscribe_to_user_personal_channel"`
}

type Channel struct {
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
	// WithoutNamespace is a configuration of RpcOptions for rpc methods without rpc namespace. Generally,
	// we recommend always use rpc namespaces but this option can be useful for simple setups.
	WithoutNamespace RpcOptions `mapstructure:"without_namespace" json:"without_namespace" envconfig:"without_namespace" yaml:"without_namespace" toml:"without_namespace"`
	// RPCNamespaces is a list of rpc namespaces. Each rpc namespace can have its own set of rules.
	Namespaces RPCNamespaces `mapstructure:"namespaces" default:"[]" json:"namespaces" envconfig:"namespaces" yaml:"namespaces" toml:"namespaces"`

	Ping              bool   `mapstructure:"ping" json:"ping" envconfig:"ping" yaml:"ping" toml:"ping"`
	PingMethod        string `mapstructure:"ping_method" json:"ping_method" envconfig:"ping_method" default:"ping" yaml:"ping_method" toml:"ping_method"`
	NamespaceBoundary string `mapstructure:"namespace_boundary" json:"namespace_boundary" envconfig:"namespace_boundary" default:":" yaml:"namespace_boundary" toml:"namespace_boundary"`
}

type SubscribeToUserPersonalChannel struct {
	Enabled                  bool   `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled"`
	PersonalChannelNamespace string `mapstructure:"personal_channel_namespace" json:"personal_channel_namespace" envconfig:"personal_channel_namespace" yaml:"personal_channel_namespace" toml:"personal_channel_namespace"`
	SingleConnection         bool   `mapstructure:"single_connection" json:"single_connection" yaml:"single_connection" toml:"single_connection"`
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

type ProxyCommonHTTP struct {
	// StaticHeaders is a static set of key/value pairs to attach to HTTP proxy request as
	// headers. Headers received from HTTP client request or metadata from GRPC client request
	// both have priority over values set in StaticHttpHeaders map.
	StaticHeaders MapStringString `mapstructure:"static_headers" default:"{}" json:"static_headers" envconfig:"static_headers" yaml:"static_headers" toml:"static_headers"`
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

type UnifiedProxy struct {
	ConnectEndpoint         string `mapstructure:"connect_endpoint" json:"connect_endpoint" envconfig:"connect_endpoint" yaml:"connect_endpoint" toml:"connect_endpoint"`
	RefreshEndpoint         string `mapstructure:"refresh_endpoint" json:"refresh_endpoint" envconfig:"refresh_endpoint" yaml:"refresh_endpoint" toml:"refresh_endpoint"`
	SubscribeEndpoint       string `mapstructure:"subscribe_endpoint" json:"subscribe_endpoint" envconfig:"subscribe_endpoint" yaml:"subscribe_endpoint" toml:"subscribe_endpoint"`
	PublishEndpoint         string `mapstructure:"publish_endpoint" json:"publish_endpoint" envconfig:"publish_endpoint" yaml:"publish_endpoint" toml:"publish_endpoint"`
	SubRefreshEndpoint      string `mapstructure:"sub_refresh_endpoint" json:"sub_refresh_endpoint" envconfig:"sub_refresh_endpoint" yaml:"sub_refresh_endpoint" toml:"sub_refresh_endpoint"`
	RPCEndpoint             string `mapstructure:"rpc_endpoint" json:"rpc_endpoint" envconfig:"rpc_endpoint" yaml:"rpc_endpoint" toml:"rpc_endpoint"`
	SubscribeStreamEndpoint string `mapstructure:"subscribe_stream_endpoint" json:"subscribe_stream_endpoint" envconfig:"subscribe_stream_endpoint" yaml:"subscribe_stream_endpoint" toml:"subscribe_stream_endpoint"`

	ConnectTimeout         Duration `mapstructure:"connect_timeout" json:"connect_timeout" envconfig:"connect_timeout" default:"1s" yaml:"connect_timeout" toml:"connect_timeout"`
	RPCTimeout             Duration `mapstructure:"rpc_timeout" json:"rpc_timeout" envconfig:"rpc_timeout" default:"1s" yaml:"rpc_timeout" toml:"rpc_timeout"`
	RefreshTimeout         Duration `mapstructure:"refresh_timeout" json:"refresh_timeout" envconfig:"refresh_timeout" default:"1s" yaml:"refresh_timeout" toml:"refresh_timeout"`
	SubscribeTimeout       Duration `mapstructure:"subscribe_timeout" json:"subscribe_timeout" envconfig:"subscribe_timeout" default:"1s" yaml:"subscribe_timeout" toml:"subscribe_timeout"`
	PublishTimeout         Duration `mapstructure:"publish_timeout" json:"publish_timeout" envconfig:"publish_timeout" default:"1s" yaml:"publish_timeout" toml:"publish_timeout"`
	SubRefreshTimeout      Duration `mapstructure:"sub_refresh_timeout" json:"sub_refresh_timeout" envconfig:"sub_refresh_timeout" default:"1s" yaml:"sub_refresh_timeout" toml:"sub_refresh_timeout"`
	SubscribeStreamTimeout Duration `mapstructure:"subscribe_stream_timeout" json:"subscribe_stream_timeout" envconfig:"subscribe_stream_timeout" default:"1s" yaml:"subscribe_stream_timeout" toml:"subscribe_stream_timeout"`

	ProxyCommon `mapstructure:",squash" yaml:",inline"`
}

// Proxy configuration.
type Proxy struct {
	// Name is a unique name of proxy to reference.
	Name string `mapstructure:"name" json:"name" envconfig:"name" yaml:"name" toml:"name"`

	// Endpoint - HTTP address or GRPC service endpoint.
	Endpoint string `mapstructure:"endpoint" json:"endpoint" envconfig:"endpoint" yaml:"endpoint" toml:"endpoint"`
	// Timeout for proxy request.
	Timeout Duration `mapstructure:"timeout" default:"1s" json:"timeout" envconfig:"timeout" yaml:"timeout" toml:"timeout"`

	ProxyCommon `mapstructure:",squash" yaml:",inline"`

	TestGrpcDialer func(context.Context, string) (net.Conn, error) `json:"-" yaml:"-" toml:"-"`
}

type Proxies []Proxy

// Decode to implement the envconfig.Decoder interface
func (d *Proxies) Decode(value string) error {
	// If the source is a string and the target is a slice, try to parse it as JSON.
	var items Proxies
	err := json.Unmarshal([]byte(value), &items)
	if err != nil {
		return fmt.Errorf("error parsing utems from JSON: %v", err)
	}
	*d = items
	return nil
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

type Consumers []Consumer

// Decode to implement the envconfig.Decoder interface
func (d *Consumers) Decode(value string) error {
	// If the source is a string and the target is a slice, try to parse it as JSON.
	var items Consumers
	err := json.Unmarshal([]byte(value), &items)
	if err != nil {
		return fmt.Errorf("error parsing items from JSON: %v", err)
	}
	*d = items
	return nil
}

type PostgresConsumerConfig struct {
	DSN                          string    `mapstructure:"dsn" json:"dsn" envconfig:"dsn" yaml:"dsn" toml:"dsn"`
	OutboxTableName              string    `mapstructure:"outbox_table_name" json:"outbox_table_name" envconfig:"outbox_table_name" yaml:"outbox_table_name" toml:"outbox_table_name"`
	NumPartitions                int       `mapstructure:"num_partitions" json:"num_partitions" envconfig:"num_partitions" default:"1" yaml:"num_partitions" toml:"num_partitions"`
	PartitionSelectLimit         int       `mapstructure:"partition_select_limit" json:"partition_select_limit" envconfig:"partition_select_limit" default:"100" yaml:"partition_select_limit" toml:"partition_select_limit"`
	PartitionPollInterval        Duration  `mapstructure:"partition_poll_interval" json:"partition_poll_interval" envconfig:"partition_poll_interval" default:"300ms" yaml:"partition_poll_interval" toml:"partition_poll_interval"`
	PartitionNotificationChannel string    `mapstructure:"partition_notification_channel" json:"partition_notification_channel" envconfig:"partition_notification_channel" yaml:"partition_notification_channel" toml:"partition_notification_channel"`
	TLS                          TLSConfig `mapstructure:"tls" json:"tls" envconfig:"tls" yaml:"tls" toml:"tls"`
}

type KafkaConsumerConfig struct {
	Brokers        []string `mapstructure:"brokers" json:"brokers" envconfig:"brokers" yaml:"brokers" toml:"brokers"`
	Topics         []string `mapstructure:"topics" json:"topics" envconfig:"topics" yaml:"topics" toml:"topics"`
	ConsumerGroup  string   `mapstructure:"consumer_group" json:"consumer_group" envconfig:"consumer_group" yaml:"consumer_group" toml:"consumer_group"`
	MaxPollRecords int      `mapstructure:"max_poll_records" json:"max_poll_records" envconfig:"max_poll_records" default:"100" yaml:"max_poll_records" toml:"max_poll_records"`

	// TLS for client connection.
	TLS TLSConfig `mapstructure:"tls" json:"tls" envconfig:"tls" yaml:"tls" toml:"tls"`

	// SASLMechanism when not empty enables SASL auth. For now, Centrifugo only
	// supports "plain" SASL mechanism.
	SASLMechanism string `mapstructure:"sasl_mechanism" json:"sasl_mechanism" envconfig:"sasl_mechanism" yaml:"sasl_mechanism" toml:"sasl_mechanism"`
	SASLUser      string `mapstructure:"sasl_user" json:"sasl_user" envconfig:"sasl_user" yaml:"sasl_user" toml:"sasl_user"`
	SASLPassword  string `mapstructure:"sasl_password" json:"sasl_password" envconfig:"sasl_password" yaml:"sasl_password" toml:"sasl_password"`

	// PartitionBufferSize is the size of the buffer for each partition consumer.
	// This is the number of records that can be buffered before the consumer
	// will pause fetching records from Kafka. By default, this is 16.
	// Set to -1 to use non-buffered channel.
	PartitionBufferSize int `mapstructure:"partition_buffer_size" json:"partition_buffer_size" envconfig:"partition_buffer_size" default:"16" yaml:"partition_buffer_size" toml:"partition_buffer_size"`
}
