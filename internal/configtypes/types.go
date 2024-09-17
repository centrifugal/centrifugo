package configtypes

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"time"

	"github.com/centrifugal/centrifuge"
)

type EnvStringStringMap map[string]string

func (s *EnvStringStringMap) Decode(value string) error {
	var m map[string]string
	err := json.Unmarshal([]byte(value), &m)
	*s = m
	return err
}

type ChannelNamespaces []ChannelNamespace

// Decode to implement the envconfig.Decoder interface
func (d *ChannelNamespaces) Decode(value string) error {
	// If the source is a string and the target is a slice, try to parse it as JSON.
	var items ChannelNamespaces
	err := json.Unmarshal([]byte(value), &items)
	if err != nil {
		return fmt.Errorf("error parsing items from JSON: %v", err)
	}
	*d = items
	return nil
}

type RPCNamespaces []RpcNamespace

// Decode to implement the envconfig.Decoder interface
func (d *RPCNamespaces) Decode(value string) error {
	// If the source is a string and the target is a slice, try to parse it as JSON.
	var items RPCNamespaces
	err := json.Unmarshal([]byte(value), &items)
	if err != nil {
		return fmt.Errorf("error parsing items from JSON: %v", err)
	}
	*d = items
	return nil
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

type Token struct {
	HMACSecretKey      string `mapstructure:"hmac_secret_key" json:"hmac_secret_key" envconfig:"hmac_secret_key"`
	RSAPublicKey       string `mapstructure:"rsa_public_key" json:"rsa_public_key" envconfig:"rsa_public_key"`
	ECDSAPublicKey     string `mapstructure:"ecdsa_public_key" json:"ecdsa_public_key" envconfig:"ecdsa_public_key"`
	JWKSPublicEndpoint string `mapstructure:"jwks_public_endpoint" json:"jwks_public_endpoint" envconfig:"jwks_public_endpoint"`
	Audience           string `mapstructure:"audience" json:"audience" envconfig:"audience"`
	AudienceRegex      string `mapstructure:"audience_regex" json:"audience_regex" envconfig:"audience_regex"`
	Issuer             string `mapstructure:"issuer" json:"issuer" envconfig:"issuer"`
	IssuerRegex        string `mapstructure:"issuer_regex" json:"issuer_regex" envconfig:"issuer_regex"`
	UserIDClaim        string `mapstructure:"user_id_claim" json:"user_id_claim" envconfig:"user_id_claim"`
}

type SubscriptionToken struct {
	Enabled bool `mapstructure:"enabled" json:"enabled" envconfig:"enabled"`
	Token   `mapstructure:",squash"`
}

type WebSocket struct {
	Disabled           bool          `mapstructure:"disabled" json:"disabled" envconfig:"disabled"`
	HandlerPrefix      string        `mapstructure:"handler_prefix" json:"handler_prefix" envconfig:"handler_prefix" default:"/connection/websocket"`
	Compression        bool          `mapstructure:"compression" json:"compression" envconfig:"compression"`
	CompressionMinSize int           `mapstructure:"compression_min_size" json:"compression_min_size" envconfig:"compression_min_size"`
	CompressionLevel   int           `mapstructure:"compression_level" json:"compression_level" envconfig:"compression_level" default:"1"`
	ReadBufferSize     int           `mapstructure:"read_buffer_size" json:"read_buffer_size" envconfig:"read_buffer_size"`
	UseWriteBufferPool bool          `mapstructure:"use_write_buffer_pool" json:"use_write_buffer_pool" envconfig:"use_write_buffer_pool"`
	WriteBufferSize    int           `mapstructure:"write_buffer_size" json:"write_buffer_size" envconfig:"write_buffer_size"`
	WriteTimeout       time.Duration `mapstructure:"write_timeout" json:"write_timeout" envconfig:"write_timeout" default:"1000ms"`
	MessageSizeLimit   int           `mapstructure:"message_size_limit" json:"message_size_limit" envconfig:"message_size_limit" default:"65536"`
}

type SSE struct {
	Enabled            bool   `mapstructure:"enabled" json:"enabled" envconfig:"enabled"`
	HandlerPrefix      string `mapstructure:"handler_prefix" json:"handler_prefix" envconfig:"handler_prefix" default:"/connection/sse"`
	MaxRequestBodySize int    `mapstructure:"max_request_body_size" json:"max_request_body_size" envconfig:"max_request_body_size" default:"65536"`
}

type HTTPStream struct {
	Enabled            bool   `mapstructure:"enabled" json:"enabled" envconfig:"enabled"`
	HandlerPrefix      string `mapstructure:"handler_prefix" json:"handler_prefix" envconfig:"handler_prefix" default:"/connection/http_stream"`
	MaxRequestBodySize int    `mapstructure:"max_request_body_size" json:"max_request_body_size" envconfig:"max_request_body_size" default:"65536"`
}

type WebTransport struct {
	Enabled       bool   `mapstructure:"enabled" json:"enabled" envconfig:"enabled"`
	HandlerPrefix string `mapstructure:"handler_prefix" json:"handler_prefix" envconfig:"handler_prefix" default:"/connection/webtransport"`
}

type UniWebSocket struct {
	Enabled            bool          `mapstructure:"enabled" json:"enabled" envconfig:"enabled"`
	HandlerPrefix      string        `mapstructure:"handler_prefix" json:"handler_prefix" envconfig:"handler_prefix" default:"/connection/uni_websocket"`
	Compression        bool          `mapstructure:"compression" json:"compression" envconfig:"compression"`
	CompressionMinSize int           `mapstructure:"compression_min_size" json:"compression_min_size" envconfig:"compression_min_size"`
	CompressionLevel   int           `mapstructure:"compression_level" json:"compression_level" envconfig:"compression_level" default:"1"`
	ReadBufferSize     int           `mapstructure:"read_buffer_size" json:"read_buffer_size" envconfig:"read_buffer_size"`
	UseWriteBufferPool bool          `mapstructure:"use_write_buffer_pool" json:"use_write_buffer_pool" envconfig:"use_write_buffer_pool"`
	WriteBufferSize    int           `mapstructure:"write_buffer_size" json:"write_buffer_size" envconfig:"write_buffer_size"`
	WriteTimeout       time.Duration `mapstructure:"write_timeout" json:"write_timeout" envconfig:"write_timeout" default:"1000ms"`
	MessageSizeLimit   int           `mapstructure:"message_size_limit" json:"message_size_limit" envconfig:"message_size_limit" default:"65536"`
}

type UniHTTPStream struct {
	Enabled            bool   `mapstructure:"enabled" json:"enabled" envconfig:"enabled"`
	HandlerPrefix      string `mapstructure:"handler_prefix" json:"handler_prefix" envconfig:"handler_prefix" default:"/connection/uni_http_stream"`
	MaxRequestBodySize int    `mapstructure:"max_request_body_size" json:"max_request_body_size" envconfig:"max_request_body_size" default:"65536"`
}

type UniSSE struct {
	Enabled bool `mapstructure:"enabled" json:"enabled" envconfig:"enabled"`

	HandlerPrefix string `mapstructure:"handler_prefix" json:"handler_prefix" envconfig:"handler_prefix" default:"/connection/uni_sse"`

	// MaxRequestBodySize for initial POST requests (when POST is used).
	MaxRequestBodySize int `mapstructure:"max_request_body_size" json:"max_request_body_size" envconfig:"max_request_body_size" default:"65536"`
}

type UniGRPC struct {
	Enabled               bool      `mapstructure:"enabled" json:"enabled" envconfig:"enabled"`
	Address               string    `mapstructure:"address" json:"address" envconfig:"address"`
	Port                  int       `mapstructure:"port" json:"port" envconfig:"port" default:"11000"`
	MaxReceiveMessageSize int       `mapstructure:"max_receive_message_size" json:"max_receive_message_size" envconfig:"max_receive_message_size"`
	TLS                   TLSConfig `mapstructure:"tls" json:"tls" envconfig:"tls"`
}

// PingPong allows configuring application level ping-pong behavior.
// Note that in current implementation PingPongConfig.PingInterval must be greater than PingPongConfig.PongTimeout.
type PingPong struct {
	// PingInterval tells how often to issue server-to-client pings.
	// To disable sending app-level pings use -1.
	PingInterval time.Duration `mapstructure:"ping_interval" json:"ping_interval" envconfig:"ping_interval" default:"25s"`
	// PongTimeout sets time for pong check after issuing a ping. To disable pong checks use -1.
	// PongTimeout must be less than PingInterval in current implementation.
	PongTimeout time.Duration `mapstructure:"pong_timeout" json:"pong_timeout" envconfig:"pong_timeout" default:"8s"`
}

type Redis struct {
	Address            []string      `mapstructure:"address" json:"address" envconfig:"address" default:"redis://127.0.0.1:6379"`
	Prefix             string        `mapstructure:"prefix" json:"prefix" envconfig:"prefix" default:"centrifugo"`
	ConnectTimeout     time.Duration `mapstructure:"connect_timeout" json:"connect_timeout" envconfig:"connect_timeout" default:"1s"`
	IOTimeout          time.Duration `mapstructure:"io_timeout" json:"io_timeout" envconfig:"io_timeout" default:"4s"`
	DB                 int           `mapstructure:"db" json:"db" envconfig:"db" default:"0"`
	User               string        `mapstructure:"user" json:"user" envconfig:"user"`
	Password           string        `mapstructure:"password" json:"password" envconfig:"password"`
	ClientName         string        `mapstructure:"client_name" json:"client_name" envconfig:"client_name"`
	ForceResp2         bool          `mapstructure:"force_resp2" json:"force_resp2" envconfig:"force_resp2"`
	ClusterAddress     []string      `mapstructure:"cluster_address" json:"cluster_address" envconfig:"cluster_address"`
	SentinelAddress    []string      `mapstructure:"sentinel_address" json:"sentinel_address" envconfig:"sentinel_address"`
	SentinelUser       string        `mapstructure:"sentinel_user" json:"sentinel_user" envconfig:"sentinel_user"`
	SentinelPassword   string        `mapstructure:"sentinel_password" json:"sentinel_password" envconfig:"sentinel_password"`
	SentinelMasterName string        `mapstructure:"sentinel_master_name" json:"sentinel_master_name" envconfig:"sentinel_master_name"`
	SentinelClientName string        `mapstructure:"sentinel_client_name" json:"sentinel_client_name" envconfig:"sentinel_client_name"`
	TLS                TLSConfig     `mapstructure:"tls" json:"tls" envconfig:"tls"`
	SentinelTLS        TLSConfig     `mapstructure:"sentinel_tls" json:"sentinel_tls" envconfig:"sentinel_tls"`
}

type RedisBrokerCommon struct {
	UseLists bool `mapstructure:"use_lists" json:"use_lists" envconfig:"use_lists"`
}

type RedisBroker struct {
	Redis             `mapstructure:",squash"`
	RedisBrokerCommon `mapstructure:",squash"`
}

type EngineRedisBroker struct {
	RedisBrokerCommon `mapstructure:",squash"`
}

type RedisPresenceManagerCommon struct {
	PresenceTTL          time.Duration `mapstructure:"presence_ttl" json:"presence_ttl" envconfig:"presence_ttl" default:"60s"`
	PresenceHashFieldTTL bool          `mapstructure:"presence_hash_field_ttl" json:"presence_hash_field_ttl" envconfig:"presence_hash_field_ttl"`
	PresenceUserMapping  bool          `mapstructure:"presence_user_mapping" json:"presence_user_mapping" envconfig:"presence_user_mapping"`
}

type EngineRedisPresenceManager struct {
	RedisPresenceManagerCommon `mapstructure:",squash"`
}

type RedisPresenceManager struct {
	Redis                      `mapstructure:",squash"`
	RedisPresenceManagerCommon `mapstructure:",squash"`
}

// RedisEngine configuration.
type RedisEngine struct {
	Redis                      `mapstructure:",squash"`
	EngineRedisBroker          `mapstructure:",squash"`
	EngineRedisPresenceManager `mapstructure:",squash"`
}

// NatsBroker configuration.
type NatsBroker struct {
	// URL is a Nats server URL.
	URL string `mapstructure:"url" json:"url" envconfig:"url"`
	// Prefix allows customizing channel prefix in Nats to work with a single Nats from different
	// unrelated Centrifugo setups.
	Prefix string `mapstructure:"prefix" json:"prefix" envconfig:"prefix"`
	// DialTimeout is a timeout for establishing connection to Nats.
	DialTimeout time.Duration `mapstructure:"dial_timeout" json:"dial_timeout" envconfig:"dial_timeout"`
	// WriteTimeout is a timeout for write operation to Nats.
	WriteTimeout time.Duration `mapstructure:"write_timeout" json:"write_timeout" envconfig:"write_timeout"`
	// TLS for the Nats connection. TLS is not used if nil.
	TLS TLSConfig `mapstructure:"tls" json:"tls" envconfig:"tls"`

	// AllowWildcards allows to enable wildcard subscriptions. By default, wildcard subscriptions
	// are not allowed. Using wildcard subscriptions can't be combined with join/leave events and presence
	// because subscriptions do not belong to a concrete channel after with wildcards, while join/leave events
	// require concrete channel to be published. And presence does not make a lot of sense for wildcard
	// subscriptions - there could be subscribers which use different mask, but still receive subset of updates.
	// It's required to use channels without wildcards to for mentioned features to work properly. When
	// using wildcard subscriptions a special care is needed regarding security - pay additional
	// attention to a proper permission management.
	AllowWildcards bool `mapstructure:"allow_wildcards" json:"allow_wildcards" envconfig:"allow_wildcards"`

	// RawMode allows enabling raw communication with Nats. When on, Centrifugo subscribes to channels
	// without adding any prefixes to channel name. Proper prefixes must be managed by the application in this
	// case. Data consumed from Nats is sent directly to subscribers without any processing. When publishing
	// to Nats Centrifugo does not add any prefixes to channel names also. Centrifugo features like Publication
	// tags, Publication ClientInfo, join/leave events are not supported in raw mode.
	RawMode RawModeConfig `mapstructure:"raw_mode" json:"raw_mode" envconfig:"raw_mode"`
}

type RawModeConfig struct {
	// Enabled enables raw mode when true.
	Enabled bool `mapstructure:"enabled" json:"enabled" envconfig:"enabled"`

	// ChannelReplacements is a map where keys are strings to replace and values are replacements.
	// For example, you have Centrifugo namespace "chat" and using channel "chat:index", but you want to
	// use channel "chat.index" in Nats. Then you can define SymbolReplacements map like this: {":": "."}.
	// In this case Centrifugo will replace all ":" symbols in channel name with "." before sending to Nats.
	// Broker keeps reverse mapping to the original channel to broadcast to proper channels when processing
	// messages received from Nats.
	ChannelReplacements EnvStringStringMap `mapstructure:"channel_replacements" json:"channel_replacements" envconfig:"channel_replacements"`

	// Prefix is a string that will be added to all channels when publishing messages to Nats, subscribing
	// to channels in Nats. It's also stripped from channel name when processing messages received from Nats.
	// By default, no prefix is used.
	Prefix string `mapstructure:"prefix" json:"prefix" envconfig:"prefix"`
}

type OpenTelemetry struct {
	Enabled   bool `mapstructure:"enabled" json:"enabled" envconfig:"enabled"`
	API       bool `mapstructure:"api" json:"api" envconfig:"api"`
	Consuming bool `mapstructure:"consuming" json:"consuming" envconfig:"consuming"`
}

type HttpAPI struct {
	Disabled      bool   `mapstructure:"disabled" json:"disabled" envconfig:"disabled"`
	HandlerPrefix string `mapstructure:"handler_prefix" json:"handler_prefix" envconfig:"handler_prefix" default:"/api"`
	Key           string `mapstructure:"key" json:"key" envconfig:"key"`
	ErrorMode     string `mapstructure:"error_mode" json:"error_mode" envconfig:"error_mode"`
	External      bool   `mapstructure:"external" json:"external" envconfig:"external"`
	Insecure      bool   `mapstructure:"insecure" json:"insecure" envconfig:"insecure"`
}

type GrpcAPI struct {
	Enabled               bool      `mapstructure:"enabled" json:"enabled" envconfig:"enabled"`
	ErrorMode             string    `mapstructure:"error_mode" json:"error_mode" envconfig:"error_mode"`
	Address               string    `mapstructure:"address" json:"address" envconfig:"address"`
	Port                  int       `mapstructure:"port" json:"port" envconfig:"port" default:"10000"`
	Key                   string    `mapstructure:"key" json:"key" envconfig:"key"`
	TLS                   TLSConfig `mapstructure:"tls" json:"tls" envconfig:"tls"`
	Reflection            bool      `mapstructure:"reflection" json:"reflection" envconfig:"reflection"`
	MaxReceiveMessageSize int       `mapstructure:"max_receive_message_size" json:"max_receive_message_size" envconfig:"max_receive_message_size"`
}

type Graphite struct {
	Enabled  bool          `mapstructure:"enabled" json:"enabled" envconfig:"enabled"`
	Host     string        `mapstructure:"host" json:"host" envconfig:"host" default:"localhost"`
	Port     int           `mapstructure:"port" json:"port" envconfig:"port" default:"2003"`
	Prefix   string        `mapstructure:"prefix" json:"prefix" envconfig:"prefix" default:"centrifugo"`
	Interval time.Duration `mapstructure:"interval" json:"interval" envconfig:"interval" default:"10s"`
	Tags     bool          `mapstructure:"tags" json:"tags" envconfig:"tags"`
}

type Emulation struct {
	HandlerPrefix      string `mapstructure:"handler_prefix" json:"handler_prefix" envconfig:"handler_prefix" default:"/emulation"`
	MaxRequestBodySize int    `mapstructure:"max_request_body_size" json:"max_request_body_size" envconfig:"max_request_body_size" default:"65536"`
}

type UsageStats struct {
	Disabled bool `mapstructure:"disabled" json:"disabled" envconfig:"disabled"`
}

type Prometheus struct {
	Enabled       bool   `mapstructure:"enabled" json:"enabled" envconfig:"enabled"`
	HandlerPrefix string `mapstructure:"handler_prefix" json:"handler_prefix" envconfig:"handler_prefix" default:"/metrics"`
}

type Health struct {
	Enabled       bool   `mapstructure:"enabled" json:"enabled" envconfig:"enabled"`
	HandlerPrefix string `mapstructure:"handler_prefix" json:"handler_prefix" envconfig:"handler_prefix" default:"/health"`
}

type Swagger struct {
	Enabled       bool   `mapstructure:"enabled" json:"enabled" envconfig:"enabled"`
	HandlerPrefix string `mapstructure:"handler_prefix" json:"handler_prefix" envconfig:"handler_prefix" default:"/swagger"`
}

type Debug struct {
	Enabled       bool   `mapstructure:"enabled" json:"enabled" envconfig:"enabled"`
	HandlerPrefix string `mapstructure:"handler_prefix" json:"handler_prefix" envconfig:"handler_prefix" default:"/debug/pprof"`
}

type Shutdown struct {
	Timeout time.Duration `mapstructure:"timeout" json:"timeout" envconfig:"timeout" default:"30s"`
}

type Client struct {
	// AllowedOrigins is a list of allowed origins for client connections.
	AllowedOrigins []string `mapstructure:"allowed_origins" json:"allowed_origins" envconfig:"allowed_origins"`

	// Token is a configuration for token generation and verification. When enabled, this configuration
	// is used for both connection and subscription tokens. See also SubscriptionToken to use a separate
	// configuration for subscription tokens.
	Token Token `mapstructure:"token" json:"token" envconfig:"token"`
	// SubscriptionToken is a configuration for subscription token generation and verification. When enabled,
	// Centrifugo will use this configuration for subscription tokens only. Configuration in Token is then only
	// used for connection tokens.
	SubscriptionToken SubscriptionToken `mapstructure:"subscription_token" json:"subscription_token" envconfig:"subscription_token"`
	// AllowAnonymousConnectWithoutToken allows to connect to Centrifugo without a token. In this case connection will
	// be accepted but client will be anonymous (i.e. will have empty user ID).
	AllowAnonymousConnectWithoutToken bool `mapstructure:"allow_anonymous_connect_without_token" json:"allow_anonymous_connect_without_token" envconfig:"allow_anonymous_connect_without_token"`
	// DisallowAnonymousConnectionTokens disallows anonymous connection tokens. When enabled, Centrifugo will not
	// accept connection tokens with empty user ID.
	DisallowAnonymousConnectionTokens bool `mapstructure:"disallow_anonymous_connection_tokens" json:"disallow_anonymous_connection_tokens" envconfig:"disallow_anonymous_connection_tokens"`

	// PingPong allows configuring application level ping-pong behavior for client connections.
	PingPong `mapstructure:",squash"`

	AllowedDeltaTypes                []centrifuge.DeltaType `mapstructure:"allowed_delta_types" json:"allowed_delta_types" envconfig:"allowed_delta_types"`
	ExpiredCloseDelay                time.Duration          `mapstructure:"expired_close_delay" json:"expired_close_delay" envconfig:"expired_close_delay" default:"25s"`
	ExpiredSubCloseDelay             time.Duration          `mapstructure:"expired_sub_close_delay" json:"expired_sub_close_delay" envconfig:"expired_sub_close_delay" default:"25s"`
	StaleCloseDelay                  time.Duration          `mapstructure:"stale_close_delay" json:"stale_close_delay" envconfig:"stale_close_delay" default:"10s"`
	ChannelLimit                     int                    `mapstructure:"channel_limit" json:"channel_limit" envconfig:"channel_limit" default:"128"`
	QueueMaxSize                     int                    `mapstructure:"queue_max_size" json:"queue_max_size" envconfig:"queue_max_size" default:"1048576"`
	PresenceUpdateInterval           time.Duration          `mapstructure:"presence_update_interval" json:"presence_update_interval" envconfig:"presence_update_interval" default:"27s"`
	Concurrency                      int                    `mapstructure:"concurrency" json:"concurrency" envconfig:"concurrency"`
	ChannelPositionCheckDelay        time.Duration          `mapstructure:"channel_position_check_delay" json:"channel_position_check_delay" envconfig:"channel_position_check_delay" default:"40s"`
	ChannelPositionMaxTimeLag        time.Duration          `mapstructure:"channel_position_max_time_lag" json:"channel_position_max_time_lag" envconfig:"channel_position_max_time_lag"`
	ConnectionLimit                  int                    `mapstructure:"connection_limit" json:"connection_limit" envconfig:"connection_limit"`
	UserConnectionLimit              int                    `mapstructure:"user_connection_limit" json:"user_connection_limit" envconfig:"user_connection_limit"`
	ConnectionRateLimit              int                    `mapstructure:"connection_rate_limit" json:"connection_rate_limit" envconfig:"connection_rate_limit"`
	ConnectIncludeServerTime         bool                   `mapstructure:"connect_include_server_time" json:"connect_include_server_time" envconfig:"connect_include_server_time"`
	HistoryMaxPublicationLimit       int                    `mapstructure:"history_max_publication_limit" json:"history_max_publication_limit" envconfig:"history_max_publication_limit" default:"300"`
	RecoveryMaxPublicationLimit      int                    `mapstructure:"recovery_max_publication_limit" json:"recovery_max_publication_limit" envconfig:"recovery_max_publication_limit" default:"300"`
	InsecureSkipTokenSignatureVerify bool                   `mapstructure:"insecure_skip_token_signature_verify" json:"insecure_skip_token_signature_verify" envconfig:"insecure_skip_token_signature_verify"`
	UserIDHTTPHeader                 string                 `mapstructure:"user_id_http_header" json:"user_id_http_header" envconfig:"user_id_http_header"`
	Insecure                         bool                   `mapstructure:"insecure" json:"insecure" envconfig:"insecure"`
}

type Channel struct {
	// ChannelOptions is a configuration for channels which do not have namespace. Generally, we recommend always
	// use channel namespaces but this option can be useful for simple setups.
	ChannelOptions `mapstructure:",squash"`
	// Namespaces is a list of channel namespaces. Each channel namespace can have its own set of rules.
	Namespaces ChannelNamespaces `mapstructure:"namespaces" json:"namespaces" envconfig:"namespaces"`

	MaxLength         int    `mapstructure:"max_length" json:"max_length" envconfig:"max_length" default:"255"`
	PrivatePrefix     string `mapstructure:"private_prefix" json:"private_prefix" envconfig:"private_prefix" default:"$"`
	NamespaceBoundary string `mapstructure:"namespace_boundary" json:"namespace_boundary" envconfig:"namespace_boundary" default:":"`
	UserBoundary      string `mapstructure:"user_boundary" json:"user_boundary" envconfig:"user_boundary" default:"#"`
	UserSeparator     string `mapstructure:"user_separator" json:"user_separator" envconfig:"user_separator" default:","`
}

type RPC struct {
	// RpcOptions is a configuration for rpc methods without rpc namespace. Generally, we recommend always use
	// rpc namespaces but this option can be useful for simple setups.
	RpcOptions `mapstructure:",squash"`
	// RPCNamespaces is a list of rpc namespaces. Each rpc namespace can have its own set of rules.
	Namespaces RPCNamespaces `mapstructure:"namespaces" json:"namespaces" envconfig:"namespaces"`

	Ping              bool   `mapstructure:"ping" json:"ping" envconfig:"ping"`
	PingMethod        string `mapstructure:"ping_method" json:"ping_method" envconfig:"ping_method" default:"ping"`
	NamespaceBoundary string `mapstructure:"namespace_boundary" json:"namespace_boundary" envconfig:"namespace_boundary" default:":"`
}

type UserSubscribeToPersonal struct {
	Enabled                  bool   `mapstructure:"enabled" json:"enabled" envconfig:"enabled"`
	PersonalChannelNamespace string `mapstructure:"personal_channel_namespace" json:"personal_channel_namespace" envconfig:"personal_channel_namespace"`
	SingleConnection         bool   `mapstructure:"single_connection" json:"single_connection"`
}

type Admin struct {
	Enabled       bool   `mapstructure:"enabled" json:"enabled" envconfig:"enabled"`
	HandlerPrefix string `mapstructure:"handler_prefix" json:"handler_prefix" envconfig:"handler_prefix" default:""`
	// Password is an admin password.
	Password string `mapstructure:"password" json:"password" envconfig:"password"`
	// Secret is a secret to generate auth token for admin requests.
	Secret string `mapstructure:"secret" json:"secret" envconfig:"secret"`
	// Insecure turns on insecure mode for admin endpoints - no auth
	// required to connect to web interface and for requests to admin API.
	// Admin resources must be protected by firewall rules in production when
	// this option enabled otherwise everyone from internet can make admin
	// actions.
	Insecure bool `mapstructure:"insecure" json:"insecure" envconfig:"insecure"`
	// WebPath is path to admin web application to serve.
	WebPath string `mapstructure:"web_path" json:"web_path" envconfig:"web_path"`
	// WebProxyAddress is an address for proxying to the running admin web application app.
	// So it's possible to run web app in dev mode and point Centrifugo to its address for
	// development purposes.
	WebProxyAddress string `mapstructure:"web_proxy_address" json:"web_proxy_address" envconfig:"web_proxy_address"`
	External        bool   `mapstructure:"external" json:"external" envconfig:"external"`
}

type ProxyCommon struct {
	// HTTPHeaders is a list of HTTP headers to proxy. No headers used by proxy by default.
	// If GRPC proxy is used then request HTTP headers set to outgoing request metadata.
	HttpHeaders []string `mapstructure:"http_headers" json:"http_headers,omitempty" envconfig:"http_headers"`
	// GRPCMetadata is a list of GRPC metadata keys to proxy. No meta keys used by proxy by
	// default. If HTTP proxy is used then these keys become outgoing request HTTP headers.
	GrpcMetadata []string `mapstructure:"grpc_metadata" json:"grpc_metadata,omitempty" envconfig:"grpc_metadata"`

	// StaticHttpHeaders is a static set of key/value pairs to attach to HTTP proxy request as
	// headers. Headers received from HTTP client request or metadata from GRPC client request
	// both have priority over values set in StaticHttpHeaders map.
	StaticHttpHeaders EnvStringStringMap `mapstructure:"static_http_headers" json:"static_http_headers,omitempty" envconfig:"static_http_headers"`

	// BinaryEncoding makes proxy send data as base64 string (assuming it contains custom
	// non-JSON payload).
	BinaryEncoding bool `mapstructure:"binary_encoding" json:"binary_encoding,omitempty" envconfig:"binary_encoding"`
	// IncludeConnectionMeta to each proxy request (except connect where it's obtained).
	IncludeConnectionMeta bool `mapstructure:"include_connection_meta" json:"include_connection_meta,omitempty" envconfig:"include_connection_meta"`

	// GrpcTLS is a common configuration for GRPC TLS.
	GrpcTLS TLSConfig `mapstructure:"grpc_tls" json:"grpc_tls,omitempty" envconfig:"grpc_tls"`
	// GrpcCertFile is a path to GRPC cert file on disk.
	GrpcCertFile string `mapstructure:"grpc_cert_file" json:"grpc_cert_file,omitempty" envconfig:"grpc_cert_file"`
	// GrpcCredentialsKey is a custom key to add into per-RPC credentials.
	GrpcCredentialsKey string `mapstructure:"grpc_credentials_key" json:"grpc_credentials_key,omitempty" envconfig:"grpc_credentials_key"`
	// GrpcCredentialsValue is a custom value for GrpcCredentialsKey.
	GrpcCredentialsValue string `mapstructure:"grpc_credentials_value" json:"grpc_credentials_value,omitempty" envconfig:"grpc_credentials_value"`
	// GrpcCompression enables compression for outgoing calls (gzip).
	GrpcCompression bool `mapstructure:"grpc_compression" json:"grpc_compression,omitempty" envconfig:"grpc_compression"`
}

type GlobalProxy struct {
	ConnectEndpoint         string `mapstructure:"connect_endpoint" json:"connect_endpoint" envconfig:"connect_endpoint"`
	RefreshEndpoint         string `mapstructure:"refresh_endpoint" json:"refresh_endpoint" envconfig:"refresh_endpoint"`
	SubscribeEndpoint       string `mapstructure:"subscribe_endpoint" json:"subscribe_endpoint" envconfig:"subscribe_endpoint"`
	PublishEndpoint         string `mapstructure:"publish_endpoint" json:"publish_endpoint" envconfig:"publish_endpoint"`
	SubRefreshEndpoint      string `mapstructure:"sub_refresh_endpoint" json:"sub_refresh_endpoint" envconfig:"sub_refresh_endpoint"`
	RPCEndpoint             string `mapstructure:"rpc_endpoint" json:"rpc_endpoint" envconfig:"rpc_endpoint"`
	SubscribeStreamEndpoint string `mapstructure:"subscribe_stream_endpoint" json:"subscribe_stream_endpoint" envconfig:"subscribe_stream_endpoint"`
	StreamSubscribeEndpoint string `mapstructure:"stream_subscribe_endpoint" json:"stream_subscribe_endpoint" envconfig:"stream_subscribe_endpoint"`

	ConnectTimeout         time.Duration `mapstructure:"connect_timeout" json:"connect_timeout" envconfig:"connect_timeout" default:"1s"`
	RPCTimeout             time.Duration `mapstructure:"rpc_timeout" json:"rpc_timeout" envconfig:"rpc_timeout" default:"1s"`
	RefreshTimeout         time.Duration `mapstructure:"refresh_timeout" json:"refresh_timeout" envconfig:"refresh_timeout" default:"1s"`
	SubscribeTimeout       time.Duration `mapstructure:"subscribe_timeout" json:"subscribe_timeout" envconfig:"subscribe_timeout" default:"1s"`
	PublishTimeout         time.Duration `mapstructure:"publish_timeout" json:"publish_timeout" envconfig:"publish_timeout" default:"1s"`
	SubRefreshTimeout      time.Duration `mapstructure:"sub_refresh_timeout" json:"sub_refresh_timeout" envconfig:"sub_refresh_timeout" default:"1s"`
	SubscribeStreamTimeout time.Duration `mapstructure:"subscribe_stream_timeout" json:"subscribe_stream_timeout" envconfig:"subscribe_stream_timeout" default:"1s"`
	StreamSubscribeTimeout time.Duration `mapstructure:"stream_subscribe_timeout" json:"stream_subscribe_timeout" envconfig:"stream_subscribe_timeout" default:"1s"`

	ProxyCommon `mapstructure:",squash"`
}

// Proxy configuration.
type Proxy struct {
	// Name is a unique name of proxy to reference.
	Name string `mapstructure:"name" json:"name" envconfig:"name"`

	// Enabled must be true to tell Centrifugo to use the configured proxy.
	Enabled bool `mapstructure:"enabled" json:"enabled" envconfig:"enabled"`

	// Endpoint - HTTP address or GRPC service endpoint.
	Endpoint string `mapstructure:"endpoint" json:"endpoint" envconfig:"endpoint"`
	// Timeout for proxy request.
	Timeout time.Duration `mapstructure:"timeout" json:"timeout,omitempty" envconfig:"timeout"`

	ProxyCommon `mapstructure:",squash"`

	TestGrpcDialer func(context.Context, string) (net.Conn, error)
}

type Consumer struct {
	// Name is a unique name required for each consumer.
	Name string `mapstructure:"name" json:"name" envconfig:"name"`

	// Enabled must be true to tell Centrifugo to run configured consumer.
	Enabled bool `mapstructure:"enabled" json:"enabled" envconfig:"enabled"`

	// Type describes the type of consumer.
	Type string `mapstructure:"type" json:"type" envconfig:"type"`

	// Postgres allows defining options for consumer of postgresql type.
	Postgres PostgresConsumerConfig `mapstructure:"postgresql" json:"postgresql,omitempty" envconfig:"postgresql"`
	// Kafka allows defining options for consumer of kafka type.
	Kafka KafkaConsumerConfig `mapstructure:"kafka" json:"kafka,omitempty" envconfig:"kafka"`
}

type PostgresConsumerConfig struct {
	DSN                          string        `mapstructure:"dsn" json:"dsn" envconfig:"dsn"`
	OutboxTableName              string        `mapstructure:"outbox_table_name" json:"outbox_table_name" envconfig:"outbox_table_name"`
	NumPartitions                int           `mapstructure:"num_partitions" json:"num_partitions" envconfig:"num_partitions" default:"1"`
	PartitionSelectLimit         int           `mapstructure:"partition_select_limit" json:"partition_select_limit" envconfig:"partition_select_limit" default:"100"`
	PartitionPollInterval        time.Duration `mapstructure:"partition_poll_interval" json:"partition_poll_interval" envconfig:"partition_poll_interval" default:"300ms"`
	PartitionNotificationChannel string        `mapstructure:"partition_notification_channel" json:"partition_notification_channel" envconfig:"partition_notification_channel"`
	TLS                          TLSConfig     `mapstructure:"tls" json:"tls" envconfig:"tls"`
}

type KafkaConsumerConfig struct {
	Brokers        []string `mapstructure:"brokers" json:"brokers" envconfig:"brokers"`
	Topics         []string `mapstructure:"topics" json:"topics" envconfig:"topics"`
	ConsumerGroup  string   `mapstructure:"consumer_group" json:"consumer_group" envconfig:"consumer_group"`
	MaxPollRecords int      `mapstructure:"max_poll_records" json:"max_poll_records" envconfig:"max_poll_records" default:"100"`

	// TLS for client connection.
	TLS TLSConfig `mapstructure:"tls" json:"tls" envconfig:"tls"`

	// SASLMechanism when not empty enables SASL auth. For now, Centrifugo only
	// supports "plain" SASL mechanism.
	SASLMechanism string `mapstructure:"sasl_mechanism" json:"sasl_mechanism" envconfig:"sasl_mechanism"`
	SASLUser      string `mapstructure:"sasl_user" json:"sasl_user" envconfig:"sasl_user"`
	SASLPassword  string `mapstructure:"sasl_password" json:"sasl_password" envconfig:"sasl_password"`

	// PartitionBufferSize is the size of the buffer for each partition consumer.
	// This is the number of records that can be buffered before the consumer
	// will pause fetching records from Kafka. By default, this is 16.
	// Set to -1 to use non-buffered channel.
	PartitionBufferSize int `mapstructure:"partition_buffer_size" json:"partition_buffer_size" envconfig:"partition_buffer_size" default:"16"`
}
