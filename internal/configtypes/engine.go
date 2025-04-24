package configtypes

type Engine struct {
	// Type of broker to use. Can be `memory` or `redis` at this point.
	Type string `mapstructure:"type" default:"memory" json:"type" envconfig:"type" yaml:"type" toml:"type"`
	// Redis is a configuration for `redis` broker.
	Redis RedisEngine `mapstructure:"redis" json:"redis" envconfig:"redis" toml:"redis" yaml:"redis"`
}

type RedisBrokerCommon struct {
	// UseLists enables usage of Redis Lists for history storage. Lists do not support pagination,
	// idempotent publish and reverse order history retrieval.
	UseLists bool `mapstructure:"history_use_lists" json:"history_use_lists" envconfig:"history_use_lists" yaml:"history_use_lists" toml:"history_use_lists"`
}

type RedisBroker struct {
	RedisPrefixed     `mapstructure:",squash" yaml:",inline"`
	RedisBrokerCommon `mapstructure:",squash" yaml:",inline"`
}

type EngineRedisBroker struct {
	RedisBrokerCommon `mapstructure:",squash" yaml:",inline"`
}

type RedisPresenceManagerCommon struct {
	// PresenceTTL is a period of time while presence information is considered valid.
	PresenceTTL Duration `mapstructure:"presence_ttl" json:"presence_ttl" envconfig:"presence_ttl" default:"60s" yaml:"presence_ttl" toml:"presence_ttl"`
	// PresenceHashFieldTTL enables using hash per-field expiration for presence.
	PresenceHashFieldTTL bool `mapstructure:"presence_hash_field_ttl" json:"presence_hash_field_ttl" envconfig:"presence_hash_field_ttl" yaml:"presence_hash_field_ttl" toml:"presence_hash_field_ttl"`
	// PresenceUserMapping enables optimization for presence stats keeping a separate hash of subscribed users in Redis.
	PresenceUserMapping bool `mapstructure:"presence_user_mapping" json:"presence_user_mapping" envconfig:"presence_user_mapping" yaml:"presence_user_mapping" toml:"presence_user_mapping"`
}

type EngineRedisPresenceManager struct {
	RedisPresenceManagerCommon `mapstructure:",squash" yaml:",inline"`
}

type RedisPresenceManager struct {
	RedisPrefixed              `mapstructure:",squash" yaml:",inline"`
	RedisPresenceManagerCommon `mapstructure:",squash" yaml:",inline"`
}

// RedisNatsBroker configuration.
type RedisNatsBroker struct {
	Redis RedisBroker `mapstructure:"redis" json:"redis" envconfig:"redis" toml:"redis" yaml:"redis"`
	Nats  NatsBroker  `mapstructure:"nats" json:"nats" envconfig:"nats" toml:"nats" yaml:"nats"`
}

// RedisEngine configuration.
type RedisEngine struct {
	RedisPrefixed              `mapstructure:",squash" yaml:",inline"`
	EngineRedisBroker          `mapstructure:",squash" yaml:",inline"`
	EngineRedisPresenceManager `mapstructure:",squash" yaml:",inline"`
}

type Broker struct {
	Enabled bool `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled"`
	// Type of broker to use. Can be "memory", "redis", "nats" at this point.
	Type string `mapstructure:"type" default:"memory" json:"type" envconfig:"type" yaml:"type" toml:"type"`
	// Redis is a configuration for "redis" broker.
	Redis RedisBroker `mapstructure:"redis" json:"redis" envconfig:"redis" toml:"redis" yaml:"redis"`
	// Nats is a configuration for NATS broker. It does not support history/recovery/cache.
	Nats NatsBroker `mapstructure:"nats" json:"nats" envconfig:"nats" toml:"nats" yaml:"nats"`
	// RedisNats is a configuration for Redis + NATS broker. It's highly experimental, undocumented and
	// can only be used when enable_unreleased_features option is set to true. NODOC.
	RedisNats *RedisNatsBroker `mapstructure:"redisnats" json:"redisnats,omitempty" envconfig:"redisnats" toml:"redisnats,omitempty" yaml:"redisnats,omitempty"`
}

type PresenceManager struct {
	Enabled bool `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled"`
	// Type of presence manager to use. Can be "memory" or "redis" at this point.
	Type string `mapstructure:"type" default:"memory" json:"type" envconfig:"type" yaml:"type" toml:"type"`
	// Redis is a configuration for "redis" broker.
	Redis RedisPresenceManager `mapstructure:"redis" json:"redis" envconfig:"redis" toml:"redis" yaml:"redis"`
}
