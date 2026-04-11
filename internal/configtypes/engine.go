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
	// Type of broker to use. Can be "memory", "redis", "nats", "postgres" at this point.
	Type string `mapstructure:"type" default:"memory" json:"type" envconfig:"type" yaml:"type" toml:"type"`
	// Redis is a configuration for "redis" broker.
	Redis RedisBroker `mapstructure:"redis" json:"redis" envconfig:"redis" toml:"redis" yaml:"redis"`
	// Nats is a configuration for NATS broker. It does not support history/recovery/cache.
	Nats NatsBroker `mapstructure:"nats" json:"nats" envconfig:"nats" toml:"nats" yaml:"nats"`
	// Postgres is a configuration for "postgres" stream broker (PG-backed
	// implementation of centrifuge.Broker for stream subscriptions).
	Postgres PostgresStreamBroker `mapstructure:"postgres" json:"postgres" envconfig:"postgres" toml:"postgres" yaml:"postgres"`
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

// MapBroker configures the map broker used for synchronized keyed state channels.
type MapBroker struct {
	// Type of map broker to use. Can be "memory", "redis", or "postgres".
	Type string `mapstructure:"type" default:"memory" json:"type" envconfig:"type" yaml:"type" toml:"type"`
	// Redis is a configuration for "redis" map broker.
	Redis RedisMapBroker `mapstructure:"redis" json:"redis" envconfig:"redis" toml:"redis" yaml:"redis"`
	// Postgres is a configuration for "postgres" map broker.
	Postgres PostgresMapBroker `mapstructure:"postgres" json:"postgres" envconfig:"postgres" toml:"postgres" yaml:"postgres"`
}

// RedisMapBroker is a configuration for Redis-based map broker.
type RedisMapBroker struct {
	RedisPrefixed        `mapstructure:",squash" yaml:",inline"`
	RedisMapBrokerCommon `mapstructure:",squash" yaml:",inline"`
}

// RedisMapBrokerCommon contains common Redis map broker settings shared between
// standalone and engine-level configurations.
type RedisMapBrokerCommon struct {
	// CleanupInterval defines how often to run the cleanup worker that generates
	// remove events for expired keyed state entries. Default: "1s".
	// Set to "-1" to disable cleanup.
	CleanupInterval Duration `mapstructure:"cleanup_interval" json:"cleanup_interval" envconfig:"cleanup_interval" default:"1s" yaml:"cleanup_interval" toml:"cleanup_interval"`
	// CleanupBatchSize defines max entries to process per channel per cleanup cycle.
	// Default: 100.
	CleanupBatchSize int `mapstructure:"cleanup_batch_size" json:"cleanup_batch_size" envconfig:"cleanup_batch_size" default:"100" yaml:"cleanup_batch_size" toml:"cleanup_batch_size"`
	// IdempotentResultTTL is a time-to-live for idempotent publish results.
	// Default: "5m".
	IdempotentResultTTL Duration `mapstructure:"idempotent_result_ttl" json:"idempotent_result_ttl" envconfig:"idempotent_result_ttl" default:"5m" yaml:"idempotent_result_ttl" toml:"idempotent_result_ttl"`
	// SkipPubSub disables PUB/SUB for the map broker. When true, the broker only works
	// with data structures without publishing to channels. Useful when PUB/SUB is handled
	// by another component.
	SkipPubSub bool `mapstructure:"skip_pub_sub" json:"skip_pub_sub" envconfig:"skip_pub_sub" yaml:"skip_pub_sub" toml:"skip_pub_sub"`
}

// PostgresMapBroker is a configuration for PostgreSQL-based map broker.
type PostgresMapBroker struct {
	// DSN is the primary PostgreSQL connection string.
	// Example: "postgres://user:pass@localhost:5432/dbname?sslmode=disable".
	DSN string `mapstructure:"dsn" json:"dsn" envconfig:"dsn" yaml:"dsn" toml:"dsn"`
	// PoolSize sets the maximum number of connections in the pool. Default: 32.
	PoolSize int `mapstructure:"pool_size" json:"pool_size" envconfig:"pool_size" default:"32" yaml:"pool_size" toml:"pool_size"`
	// NumShards is the total number of shards for parallel delivery workers.
	// Channels are distributed across shards using consistent hashing. Default: 16.
	NumShards int `mapstructure:"num_shards" json:"num_shards" envconfig:"num_shards" default:"16" yaml:"num_shards" toml:"num_shards"`
	// TTLCheckInterval is how often to check for expired keys. Default: "1s".
	TTLCheckInterval Duration `mapstructure:"ttl_check_interval" json:"ttl_check_interval" envconfig:"ttl_check_interval" default:"1s" yaml:"ttl_check_interval" toml:"ttl_check_interval"`
	// CleanupInterval is how often to clean up expired stream/meta/idempotency entries.
	// Default: "1m".
	CleanupInterval Duration `mapstructure:"cleanup_interval" json:"cleanup_interval" envconfig:"cleanup_interval" default:"1m" yaml:"cleanup_interval" toml:"cleanup_interval"`
	// IdempotentResultTTL is the default TTL for idempotency keys. Default: "5m".
	IdempotentResultTTL Duration `mapstructure:"idempotent_result_ttl" json:"idempotent_result_ttl" envconfig:"idempotent_result_ttl" default:"5m" yaml:"idempotent_result_ttl" toml:"idempotent_result_ttl"`
	// BinaryData uses BYTEA columns instead of JSONB for data fields.
	// Set to true if data payloads are not valid JSON (e.g. binary/protobuf).
	BinaryData bool `mapstructure:"binary_data" json:"binary_data" envconfig:"binary_data" yaml:"binary_data" toml:"binary_data"`
	// StreamRetention controls how long stream entries are kept. Default: "24h".
	StreamRetention Duration `mapstructure:"stream_retention" json:"stream_retention" envconfig:"stream_retention" default:"24h" yaml:"stream_retention" toml:"stream_retention"`
	// UseNotify enables LISTEN/NOTIFY for low-latency outbox wakeup.
	// When false (default), outbox worker uses polling only.
	UseNotify bool `mapstructure:"use_notify" json:"use_notify" envconfig:"use_notify" yaml:"use_notify" toml:"use_notify"`
	// SkipShardLock disables per-shard serialization of stream inserts.
	SkipShardLock bool `mapstructure:"skip_shard_lock" json:"skip_shard_lock" envconfig:"skip_shard_lock" yaml:"skip_shard_lock" toml:"skip_shard_lock"`
	// SkipSchemaInit disables automatic schema initialization on startup.
	// When true, the schema must be managed externally (e.g. via migrations).
	SkipSchemaInit bool `mapstructure:"skip_schema_init" json:"skip_schema_init" envconfig:"skip_schema_init" yaml:"skip_schema_init" toml:"skip_schema_init"`
	// Outbox configures the outbox-based delivery mode.
	Outbox PostgresMapBrokerOutbox `mapstructure:"outbox" json:"outbox" envconfig:"outbox" yaml:"outbox" toml:"outbox"`
	// PartitionLookaheadDays controls how many future daily partitions to
	// pre-create. Required > 0 so writes don't fail at the day rollover.
	// Default: 2 (gives a 48-hour safety window if the lookahead worker stalls).
	PartitionLookaheadDays int `mapstructure:"partition_lookahead_days" json:"partition_lookahead_days" envconfig:"partition_lookahead_days" default:"2" yaml:"partition_lookahead_days" toml:"partition_lookahead_days"`
	// PartitionRetentionDays controls how old a partition can be before it
	// gets dropped whole by the partition retention worker. Default: 7.
	// Set to a large value for longer retention; the special value 0 is
	// internally promoted to 7 (treat "not set" as default).
	PartitionRetentionDays int `mapstructure:"partition_retention_days" json:"partition_retention_days" envconfig:"partition_retention_days" default:"7" yaml:"partition_retention_days" toml:"partition_retention_days"`
}

// PostgresMapBrokerOutbox configures the outbox-based delivery for PostgreSQL map broker.
type PostgresMapBrokerOutbox struct {
	// PollInterval is how often to poll for new stream entries when idle. Default: "50ms".
	PollInterval Duration `mapstructure:"poll_interval" json:"poll_interval" envconfig:"poll_interval" default:"50ms" yaml:"poll_interval" toml:"poll_interval"`
	// BatchSize is the maximum number of rows to process per batch. Default: 1000.
	BatchSize int `mapstructure:"batch_size" json:"batch_size" envconfig:"batch_size" default:"1000" yaml:"batch_size" toml:"batch_size"`
}

// PostgresStreamBroker is a configuration for PostgreSQL-based stream broker.
// It implements centrifuge.Broker for stream subscriptions, providing
// transactional publishing alongside business writes in the same SQL transaction.
type PostgresStreamBroker struct {
	// DSN is the primary PostgreSQL connection string.
	DSN string `mapstructure:"dsn" json:"dsn" envconfig:"dsn" yaml:"dsn" toml:"dsn"`
	// PoolSize sets the maximum number of connections in the pool. Default: 32.
	PoolSize int `mapstructure:"pool_size" json:"pool_size" envconfig:"pool_size" default:"32" yaml:"pool_size" toml:"pool_size"`
	// NumShards is the total number of shards for parallel delivery workers.
	// Channels are distributed across shards using consistent hashing. Default: 16.
	NumShards int `mapstructure:"num_shards" json:"num_shards" envconfig:"num_shards" default:"16" yaml:"num_shards" toml:"num_shards"`
	// CleanupInterval is how often the cleanup and partition workers tick. Default: "1m".
	CleanupInterval Duration `mapstructure:"cleanup_interval" json:"cleanup_interval" envconfig:"cleanup_interval" default:"1m" yaml:"cleanup_interval" toml:"cleanup_interval"`
	// IdempotentResultTTL is the default TTL for idempotency cache entries. Default: "5m".
	IdempotentResultTTL Duration `mapstructure:"idempotent_result_ttl" json:"idempotent_result_ttl" envconfig:"idempotent_result_ttl" default:"5m" yaml:"idempotent_result_ttl" toml:"idempotent_result_ttl"`
	// BinaryData uses BYTEA columns instead of JSONB for data fields.
	// Set to true if data payloads are not valid JSON.
	BinaryData bool `mapstructure:"binary_data" json:"binary_data" envconfig:"binary_data" yaml:"binary_data" toml:"binary_data"`
	// StreamRetention is the safety floor for HistoryMetaTTL when neither
	// PublishOptions nor node config sets it. Default: "24h". Guarantees
	// every channel meta row eventually expires.
	StreamRetention Duration `mapstructure:"stream_retention" json:"stream_retention" envconfig:"stream_retention" default:"24h" yaml:"stream_retention" toml:"stream_retention"`
	// UseNotify enables LISTEN/NOTIFY for low-latency outbox wakeup.
	UseNotify bool `mapstructure:"use_notify" json:"use_notify" envconfig:"use_notify" yaml:"use_notify" toml:"use_notify"`
	// SkipShardLock disables per-shard serialization of history inserts.
	SkipShardLock bool `mapstructure:"skip_shard_lock" json:"skip_shard_lock" envconfig:"skip_shard_lock" yaml:"skip_shard_lock" toml:"skip_shard_lock"`
	// SkipSchemaInit disables automatic schema initialization on startup.
	SkipSchemaInit bool `mapstructure:"skip_schema_init" json:"skip_schema_init" envconfig:"skip_schema_init" yaml:"skip_schema_init" toml:"skip_schema_init"`
	// Outbox configures the outbox-based delivery mode.
	Outbox PostgresMapBrokerOutbox `mapstructure:"outbox" json:"outbox" envconfig:"outbox" yaml:"outbox" toml:"outbox"`
	// PartitionLookaheadDays controls how many future daily partitions to
	// pre-create. Default: 2 (gives a 48-hour safety window).
	PartitionLookaheadDays int `mapstructure:"partition_lookahead_days" json:"partition_lookahead_days" envconfig:"partition_lookahead_days" default:"2" yaml:"partition_lookahead_days" toml:"partition_lookahead_days"`
	// PartitionRetentionDays controls how old a partition can be before it
	// gets dropped whole by the partition retention worker. Default: 7.
	PartitionRetentionDays int `mapstructure:"partition_retention_days" json:"partition_retention_days" envconfig:"partition_retention_days" default:"7" yaml:"partition_retention_days" toml:"partition_retention_days"`
	// FineGrainedHistoryCleanup enables an opt-in chunked DELETE pass that
	// removes history rows past their channel's history_ttl, instead of
	// waiting for partition retention. Use for tight-storage deployments
	// where HistoryTTL is much smaller than PartitionRetentionDays.
	FineGrainedHistoryCleanup bool `mapstructure:"fine_grained_history_cleanup" json:"fine_grained_history_cleanup" envconfig:"fine_grained_history_cleanup" yaml:"fine_grained_history_cleanup" toml:"fine_grained_history_cleanup"`
	// CleanupBatchSize bounds each fine-grained cleanup DELETE chunk. Default: 1000.
	CleanupBatchSize int `mapstructure:"cleanup_batch_size" json:"cleanup_batch_size" envconfig:"cleanup_batch_size" default:"1000" yaml:"cleanup_batch_size" toml:"cleanup_batch_size"`
	// CleanupChunkPause is the pause between fine-grained cleanup chunks. Default: "100ms".
	CleanupChunkPause Duration `mapstructure:"cleanup_chunk_pause" json:"cleanup_chunk_pause" envconfig:"cleanup_chunk_pause" default:"100ms" yaml:"cleanup_chunk_pause" toml:"cleanup_chunk_pause"`
}
