package configtypes

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/centrifugal/centrifugo/v5/internal/consuming"
	"github.com/centrifugal/centrifugo/v5/internal/proxy"
	"github.com/centrifugal/centrifugo/v5/internal/rule"
	"github.com/centrifugal/centrifugo/v5/internal/tools"

	"github.com/centrifugal/centrifuge"
)

type ChannelNamespaces []rule.ChannelNamespace

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

type RPCNamespaces []rule.RpcNamespace

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

type ConsumerConfigs []consuming.ConsumerConfig

// Decode to implement the envconfig.Decoder interface
func (d *ConsumerConfigs) Decode(value string) error {
	// If the source is a string and the target is a slice, try to parse it as JSON.
	var items ConsumerConfigs
	err := json.Unmarshal([]byte(value), &items)
	if err != nil {
		return fmt.Errorf("error parsing items from JSON: %v", err)
	}
	*d = items
	return nil
}

type ProxyConfigs []proxy.Config

// Decode to implement the envconfig.Decoder interface
func (d *ProxyConfigs) Decode(value string) error {
	// If the source is a string and the target is a slice, try to parse it as JSON.
	var items ProxyConfigs
	err := json.Unmarshal([]byte(value), &items)
	if err != nil {
		return fmt.Errorf("error parsing utems from JSON: %v", err)
	}
	*d = items
	return nil
}

type NameInitializer struct {
	Enabled bool     `mapstructure:"enabled" json:"enabled" envconfig:"enabled"`
	Names   []string `mapstructure:"names" json:"names" envconfig:"names"`
}

type Token struct {
	Enabled            bool   `mapstructure:"enabled" json:"enabled" envconfig:"enabled"`
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
	Enabled       bool   `mapstructure:"enabled" json:"enabled" envconfig:"enabled"`
	HandlerPrefix string `mapstructure:"handler_prefix" json:"handler_prefix" envconfig:"handler_prefix" default:"/connection/sse"`
}

type HTTPStream struct {
	Enabled       bool   `mapstructure:"enabled" json:"enabled" envconfig:"enabled"`
	HandlerPrefix string `mapstructure:"handler_prefix" json:"handler_prefix" envconfig:"handler_prefix" default:"/connection/http_stream"`
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
	Address            string          `mapstructure:"address" json:"address" envconfig:"address" default:"redis://127.0.0.1:6379"`
	Prefix             string          `mapstructure:"prefix" json:"prefix" envconfig:"prefix" default:"centrifugo"`
	ConnectTimeout     time.Duration   `mapstructure:"connect_timeout" json:"connect_timeout" envconfig:"connect_timeout" default:"1s"`
	IOTimeout          time.Duration   `mapstructure:"io_timeout" json:"io_timeout" envconfig:"io_timeout" default:"4s"`
	DB                 int             `mapstructure:"db" json:"db" envconfig:"db" default:"0"`
	User               string          `mapstructure:"user" json:"user" envconfig:"user"`
	Password           string          `mapstructure:"password" json:"password" envconfig:"password"`
	ClientName         string          `mapstructure:"client_name" json:"client_name" envconfig:"client_name"`
	ForceResp2         bool            `mapstructure:"force_resp2" json:"force_resp2" envconfig:"force_resp2"`
	ClusterAddress     []string        `mapstructure:"cluster_address" json:"cluster_address" envconfig:"cluster_address"`
	SentinelAddress    []string        `mapstructure:"sentinel_address" json:"sentinel_address" envconfig:"sentinel_address"`
	SentinelUser       string          `mapstructure:"sentinel_user" json:"sentinel_user" envconfig:"sentinel_user"`
	SentinelPassword   string          `mapstructure:"sentinel_password" json:"sentinel_password" envconfig:"sentinel_password"`
	SentinelMasterName string          `mapstructure:"sentinel_master_name" json:"sentinel_master_name" envconfig:"sentinel_master_name"`
	SentinelClientName string          `mapstructure:"sentinel_client_name" json:"sentinel_client_name" envconfig:"sentinel_client_name"`
	TLS                tools.TLSConfig `mapstructure:"tls" json:"tls" envconfig:"tls"`
	SentinelTLS        tools.TLSConfig `mapstructure:"sentinel_tls" json:"sentinel_tls" envconfig:"sentinel_tls"`
}

type RedisBroker struct {
	Redis    `mapstructure:",squash"`
	UseLists bool `mapstructure:"use_lists" json:"use_lists" envconfig:"use_lists"`
}

type RedisPresenceManager struct {
	Redis                `mapstructure:",squash"`
	PresenceTTL          time.Duration `mapstructure:"presence_ttl" json:"presence_ttl" envconfig:"presence_ttl" default:"60s"`
	PresenceHashFieldTTL bool          `mapstructure:"presence_hash_field_ttl" json:"presence_hash_field_ttl" envconfig:"presence_hash_field_ttl"`
	PresenceUserMapping  bool          `mapstructure:"presence_user_mapping" json:"presence_user_mapping" envconfig:"presence_user_mapping"`
}

type RedisEngine struct {
	RedisBroker          `mapstructure:",squash"`
	RedisPresenceManager `mapstructure:",squash"`
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

type Admin struct {
	Enabled         bool   `mapstructure:"enabled" json:"enabled" envconfig:"enabled"`
	HandlerPrefix   string `mapstructure:"handler_prefix" json:"handler_prefix" envconfig:"handler_prefix" default:""`
	Password        string `mapstructure:"password" json:"password" envconfig:"password"`
	Secret          string `mapstructure:"secret" json:"secret" envconfig:"secret"`
	Insecure        bool   `mapstructure:"insecure" json:"insecure" envconfig:"insecure"`
	WebPath         string `mapstructure:"web_path" json:"web_path" envconfig:"web_path"`
	WebProxyAddress string `mapstructure:"web_proxy_address" json:"web_proxy_address" envconfig:"web_proxy_address"`
	External        bool   `mapstructure:"external" json:"external" envconfig:"external"`
}

type UsageStats struct {
	Disabled bool `mapstructure:"disabled" json:"disabled" envconfig:"disabled"`
}

type Proxy struct {
	ConnectEndpoint         string `mapstructure:"connect_endpoint" json:"connect_endpoint" envconfig:"connect_endpoint"`
	RefreshEndpoint         string `mapstructure:"refresh_endpoint" json:"refresh_endpoint" envconfig:"refresh_endpoint"`
	SubscribeEndpoint       string `mapstructure:"subscribe_endpoint" json:"subscribe_endpoint" envconfig:"subscribe_endpoint"`
	PublishEndpoint         string `mapstructure:"publish_endpoint" json:"publish_endpoint" envconfig:"publish_endpoint"`
	SubRefreshEndpoint      string `mapstructure:"sub_refresh_endpoint" json:"sub_refresh_endpoint" envconfig:"sub_refresh_endpoint"`
	RPCEndpoint             string `mapstructure:"rpc_endpoint" json:"rpc_endpoint" envconfig:"rpc_endpoint"`
	SubscribeStreamEndpoint string `mapstructure:"subscribe_stream_endpoint" json:"subscribe_stream_endpoint" envconfig:"subscribe_stream_endpoint"`

	ConnectTimeout         time.Duration `mapstructure:"connect_timeout" json:"connect_timeout" envconfig:"connect_timeout" default:"1s"`
	RPCTimeout             time.Duration `mapstructure:"rpc_timeout" json:"rpc_timeout" envconfig:"rpc_timeout" default:"1s"`
	RefreshTimeout         time.Duration `mapstructure:"refresh_timeout" json:"refresh_timeout" envconfig:"refresh_timeout" default:"1s"`
	SubscribeTimeout       time.Duration `mapstructure:"subscribe_timeout" json:"subscribe_timeout" envconfig:"subscribe_timeout" default:"1s"`
	PublishTimeout         time.Duration `mapstructure:"publish_timeout" json:"publish_timeout" envconfig:"publish_timeout" default:"1s"`
	SubRefreshTimeout      time.Duration `mapstructure:"sub_refresh_timeout" json:"sub_refresh_timeout" envconfig:"sub_refresh_timeout" default:"1s"`
	SubscribeStreamTimeout time.Duration `mapstructure:"subscribe_stream_timeout" json:"subscribe_stream_timeout" envconfig:"subscribe_stream_timeout" default:"1s"`

	GrpcMetadata          []string          `mapstructure:"grpc_metadata" json:"grpc_metadata" envconfig:"grpc_metadata"`
	HttpHeaders           []string          `mapstructure:"http_headers" json:"http_headers" envconfig:"http_headers"`
	StaticHTTPHeaders     map[string]string `mapstructure:"static_http_headers" json:"static_http_headers" envconfig:"static_http_headers"`
	BinaryEncoding        bool              `mapstructure:"binary_encoding" json:"binary_encoding" envconfig:"binary_encoding"`
	IncludeConnectionMeta bool              `mapstructure:"include_connection_meta" json:"include_connection_meta" envconfig:"include_connection_meta"`
	GrpcTLS               TLSConfig         `mapstructure:"grpc_tls" json:"grpc_tls" envconfig:"grpc_tls"`
	GrpcCredentialsKey    string            `mapstructure:"grpc_credentials_key" json:"grpc_credentials_key" envconfig:"grpc_credentials_key"`
	GrpcCredentialsValue  string            `mapstructure:"grpc_credentials_value" json:"grpc_credentials_value" envconfig:"grpc_credentials_value"`
	GrpcCompression       bool              `mapstructure:"grpc_compression" json:"grpc_compression" envconfig:"grpc_compression"`
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
	Timeout          time.Duration `mapstructure:"timeout" json:"timeout" envconfig:"timeout" default:"30s"`
	TerminationDelay time.Duration `mapstructure:"termination_delay" json:"termination_delay" envconfig:"termination_delay"`
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
	SubscriptionToken Token `mapstructure:"subscription_token" json:"subscription_token" envconfig:"subscription_token"`
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
	ChannelPositionMaxTimeLag        int                    `mapstructure:"channel_position_max_time_lag" json:"channel_position_max_time_lag" envconfig:"channel_position_max_time_lag"`
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
	rule.ChannelOptions `mapstructure:",squash"`
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
	rule.RpcOptions `mapstructure:",squash"`
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
