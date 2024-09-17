package configtypes

import "time"

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
