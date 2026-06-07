package configtypes

type Engine struct {
	// Type of broker to use. Can be `memory` or `redis` at this point.
	Type string `mapstructure:"type" default:"memory" json:"type" envconfig:"type" yaml:"type" toml:"type" expose:"full" doc:"Engine type used for broker and presence manager. Can be <<memory>> or <<redis>>."`
	// Redis is a configuration for `redis` broker.
	Redis RedisEngine `mapstructure:"redis" json:"redis" envconfig:"redis" toml:"redis" yaml:"redis" doc:"Redis engine configuration, used when type is <<redis>>."`
}

type RedisBrokerCommon struct {
	// UseLists enables usage of Redis Lists for history storage. Lists do not support pagination,
	// idempotent publish and reverse order history retrieval.
	UseLists bool `mapstructure:"history_use_lists" json:"history_use_lists" envconfig:"history_use_lists" yaml:"history_use_lists" toml:"history_use_lists" doc:"Use Redis Lists for history storage. Lists do not support history pagination, idempotent publishing or reverse-order history retrieval."`
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
	PresenceTTL Duration `mapstructure:"presence_ttl" json:"presence_ttl" envconfig:"presence_ttl" default:"60s" yaml:"presence_ttl" toml:"presence_ttl" doc:"How long presence information stays valid before it expires. Default <<60s>>."`
	// PresenceHashFieldTTL enables using hash per-field expiration for presence.
	PresenceHashFieldTTL bool `mapstructure:"presence_hash_field_ttl" json:"presence_hash_field_ttl" envconfig:"presence_hash_field_ttl" yaml:"presence_hash_field_ttl" toml:"presence_hash_field_ttl" doc:"Use Redis hash per-field expiration for presence (requires Redis 7.4+)."`
	// PresenceUserMapping enables optimization for presence stats keeping a separate hash of subscribed users in Redis.
	PresenceUserMapping bool `mapstructure:"presence_user_mapping" json:"presence_user_mapping" envconfig:"presence_user_mapping" yaml:"presence_user_mapping" toml:"presence_user_mapping" doc:"Optimize presence stats by keeping a separate Redis hash of subscribed users."`
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
	Redis RedisBroker `mapstructure:"redis" json:"redis" envconfig:"redis" toml:"redis" yaml:"redis" doc:"Redis part of the Redis + NATS broker configuration."`
	Nats  NatsBroker  `mapstructure:"nats" json:"nats" envconfig:"nats" toml:"nats" yaml:"nats" doc:"NATS part of the Redis + NATS broker configuration."`
}

// RedisEngine configuration.
type RedisEngine struct {
	RedisPrefixed              `mapstructure:",squash" yaml:",inline"`
	EngineRedisBroker          `mapstructure:",squash" yaml:",inline"`
	EngineRedisPresenceManager `mapstructure:",squash" yaml:",inline"`
}

type Broker struct {
	Enabled bool `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled" doc:"Enables a standalone broker, configured separately from the engine."`
	// Type of broker to use. Can be "memory", "redis", "nats", "postgres" at this point.
	Type string `mapstructure:"type" default:"memory" json:"type" envconfig:"type" yaml:"type" toml:"type" expose:"full" doc:"Broker type. Can be <<memory>>, <<redis>>, <<nats>> or <<postgres>>."`
	// Redis is a configuration for "redis" broker.
	Redis RedisBroker `mapstructure:"redis" json:"redis" envconfig:"redis" toml:"redis" yaml:"redis" doc:"Redis broker configuration, used when type is <<redis>>."`
	// Nats is a configuration for NATS broker. It does not support history/recovery/cache.
	Nats NatsBroker `mapstructure:"nats" json:"nats" envconfig:"nats" toml:"nats" yaml:"nats" doc:"NATS broker configuration, used when type is <<nats>>. Note: NATS broker does not support history, recovery or cache."`
	// Postgres is a configuration for "postgres" stream broker (PG-backed
	// implementation of centrifuge.Broker for stream subscriptions).
	Postgres PostgresStreamBroker `mapstructure:"postgres" json:"postgres" envconfig:"postgres" toml:"postgres" yaml:"postgres" doc:"PostgreSQL stream broker configuration, used when type is <<postgres>>."`
	// RedisNats is a configuration for Redis + NATS broker. It's highly experimental, undocumented and
	// can only be used when enable_unreleased_features option is set to true.
	RedisNats *RedisNatsBroker `mapstructure:"redisnats" json:"redisnats,omitempty" envconfig:"redisnats" toml:"redisnats,omitempty" yaml:"redisnats,omitempty" expose:"-" doc:"Highly experimental Redis + NATS broker configuration. Only usable when enable_unreleased_features is true; do not use in production."`
}

type PresenceManager struct {
	Enabled bool `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled" doc:"Enables a standalone presence manager, configured separately from the engine."`
	// Type of presence manager to use. Can be "memory" or "redis" at this point.
	Type string `mapstructure:"type" default:"memory" json:"type" envconfig:"type" yaml:"type" toml:"type" expose:"full" doc:"Presence manager type. Can be <<memory>> or <<redis>>."`
	// Redis is a configuration for "redis" broker.
	Redis RedisPresenceManager `mapstructure:"redis" json:"redis" envconfig:"redis" toml:"redis" yaml:"redis" doc:"Redis presence manager configuration, used when type is <<redis>>."`
}

// MapBroker configures the map broker used for synchronized keyed state channels.
type MapBroker struct {
	// Type of map broker to use. Can be "memory", "redis", or "postgres".
	Type string `mapstructure:"type" default:"memory" json:"type" envconfig:"type" yaml:"type" toml:"type" expose:"full" doc:"Map broker type for synchronized keyed state channels. Can be <<memory>>, <<redis>> or <<postgres>>."`
	// Redis is a configuration for "redis" map broker.
	Redis RedisMapBroker `mapstructure:"redis" json:"redis" envconfig:"redis" toml:"redis" yaml:"redis" doc:"Redis map broker configuration, used when type is <<redis>>."`
	// Postgres is a configuration for "postgres" map broker.
	Postgres PostgresMapBroker `mapstructure:"postgres" json:"postgres" envconfig:"postgres" toml:"postgres" yaml:"postgres" doc:"PostgreSQL map broker configuration, used when type is <<postgres>>."`
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
	CleanupInterval Duration `mapstructure:"cleanup_interval" json:"cleanup_interval" envconfig:"cleanup_interval" default:"1s" yaml:"cleanup_interval" toml:"cleanup_interval" doc:"How often the cleanup worker emits remove events for expired keyed state entries. Default <<1s>>. Set to <<-1>> to disable cleanup."`
	// CleanupBatchSize defines max entries to process per channel per cleanup cycle.
	// Default: 100.
	CleanupBatchSize int `mapstructure:"cleanup_batch_size" json:"cleanup_batch_size" envconfig:"cleanup_batch_size" default:"100" yaml:"cleanup_batch_size" toml:"cleanup_batch_size" doc:"Maximum number of entries to process per channel per cleanup cycle. Default <<100>>."`
	// IdempotentResultTTL is a time-to-live for idempotent publish results.
	// Default: "5m".
	IdempotentResultTTL Duration `mapstructure:"idempotent_result_ttl" json:"idempotent_result_ttl" envconfig:"idempotent_result_ttl" default:"5m" yaml:"idempotent_result_ttl" toml:"idempotent_result_ttl" doc:"How long idempotent publish results are cached. Default <<5m>>."`
	// SkipPubSub disables PUB/SUB for the map broker. When true, the broker only works
	// with data structures without publishing to channels. Useful when PUB/SUB is handled
	// by another component.
	SkipPubSub bool `mapstructure:"skip_pub_sub" json:"skip_pub_sub" envconfig:"skip_pub_sub" yaml:"skip_pub_sub" toml:"skip_pub_sub" doc:"Disable PUB/SUB for the map broker, so it only updates data structures without publishing to channels. Useful when PUB/SUB is handled by another component."`
}

// PostgresMapBroker is a configuration for PostgreSQL-based map broker.
type PostgresMapBroker struct {
	// DSN is the primary PostgreSQL connection string.
	// Example: "postgres://user:pass@localhost:5432/dbname?sslmode=disable".
	DSN string `mapstructure:"dsn" json:"dsn" envconfig:"dsn" yaml:"dsn" toml:"dsn" expose:"url" doc:"Primary PostgreSQL connection string, e.g. <<postgres://user:pass@localhost:5432/dbname?sslmode=disable>>."`
	// TLS is an optional TLS configuration for all PostgreSQL connections
	// (primary, replicas, and notify). Use instead of embedding TLS parameters
	// in the DSN when certificate files are managed outside the connection string.
	TLS TLSConfig `mapstructure:"tls" json:"tls" envconfig:"tls" yaml:"tls" toml:"tls" doc:"Optional TLS configuration applied to all PostgreSQL connections (primary, replicas and notify). Use this instead of embedding TLS parameters in the DSN."`
	// PoolSize sets the maximum number of connections in the pool. Default: 16.
	PoolSize int `mapstructure:"pool_size" json:"pool_size" envconfig:"pool_size" default:"16" yaml:"pool_size" toml:"pool_size" doc:"Maximum number of connections in the pool. Default <<16>>."`
	// NumShards is the total number of shards for parallel delivery workers.
	// Channels are distributed across shards using consistent hashing. Default: 8.
	NumShards int `mapstructure:"num_shards" json:"num_shards" envconfig:"num_shards" default:"8" yaml:"num_shards" toml:"num_shards" doc:"Number of shards for parallel delivery workers. Channels are distributed across shards using consistent hashing. Default <<8>>."`
	// TTLCheckInterval is how often to check for expired keys. Default: "1s".
	TTLCheckInterval Duration `mapstructure:"ttl_check_interval" json:"ttl_check_interval" envconfig:"ttl_check_interval" default:"1s" yaml:"ttl_check_interval" toml:"ttl_check_interval" doc:"How often to check for expired keys. Default <<1s>>."`
	// CleanupInterval is how often to clean up expired stream/meta/idempotency entries.
	// Default: "1m".
	CleanupInterval Duration `mapstructure:"cleanup_interval" json:"cleanup_interval" envconfig:"cleanup_interval" default:"1m" yaml:"cleanup_interval" toml:"cleanup_interval" doc:"How often to clean up expired stream, meta and idempotency entries. Default <<1m>>."`
	// IdempotentResultTTL is the default TTL for idempotency keys. Default: "5m".
	IdempotentResultTTL Duration `mapstructure:"idempotent_result_ttl" json:"idempotent_result_ttl" envconfig:"idempotent_result_ttl" default:"5m" yaml:"idempotent_result_ttl" toml:"idempotent_result_ttl" doc:"Default TTL for idempotency keys. Default <<5m>>."`
	// BinaryData uses BYTEA columns instead of JSONB for data fields.
	// Set to true if data payloads are not valid JSON (e.g. binary/protobuf).
	BinaryData bool `mapstructure:"binary_data" json:"binary_data" envconfig:"binary_data" yaml:"binary_data" toml:"binary_data" doc:"Store data in BYTEA columns instead of JSONB. Enable when payloads are not valid JSON (e.g. binary or protobuf)."`
	// StreamRetention controls how long stream entries are kept. Default: "24h".
	StreamRetention Duration `mapstructure:"stream_retention" json:"stream_retention" envconfig:"stream_retention" default:"24h" yaml:"stream_retention" toml:"stream_retention" doc:"How long stream entries are kept. Default <<24h>>."`
	// UseNotify enables LISTEN/NOTIFY for low-latency outbox wakeup.
	// When false (default), outbox worker uses polling only.
	UseNotify bool `mapstructure:"use_notify" json:"use_notify" envconfig:"use_notify" yaml:"use_notify" toml:"use_notify" doc:"Use PostgreSQL LISTEN/NOTIFY for low-latency outbox wakeup. When disabled (default), the outbox worker relies on polling only."`
	// NotifyDSN is an optional separate DSN for the LISTEN connection.
	// Required when DSN points at PGBouncer (transaction pooling mode is
	// incompatible with LISTEN/NOTIFY). Must be a direct PostgreSQL URL.
	NotifyDSN string `mapstructure:"notify_dsn" json:"notify_dsn" envconfig:"notify_dsn" yaml:"notify_dsn" toml:"notify_dsn" expose:"url" doc:"Optional separate DSN for the LISTEN connection. Required when the main DSN points at PGBouncer (transaction pooling is incompatible with LISTEN/NOTIFY); must be a direct PostgreSQL URL."`
	// TablePrefix is the namespace prefix for all tables created by this broker.
	// Default: "cf". Multi-tenant deployments sharing one PostgreSQL instance
	// use distinct prefixes per Centrifugo cluster (e.g. "prod_us_cf").
	TablePrefix string `mapstructure:"table_prefix" json:"table_prefix" envconfig:"table_prefix" default:"cf" yaml:"table_prefix" toml:"table_prefix" expose:"full" doc:"Namespace prefix for all tables created by this broker. Default <<cf>>. Use distinct prefixes per cluster when several Centrifugo deployments share one PostgreSQL instance."`
	// SkipSchemaInit disables automatic schema initialization on startup.
	// When true, the schema must be managed externally (e.g. via migrations).
	SkipSchemaInit bool `mapstructure:"skip_schema_init" json:"skip_schema_init" envconfig:"skip_schema_init" yaml:"skip_schema_init" toml:"skip_schema_init" doc:"Disable automatic schema initialization on startup. When enabled, you must manage the schema externally (e.g. via migrations)."`
	// Outbox configures the outbox-based delivery mode.
	Outbox PostgresMapBrokerOutbox `mapstructure:"outbox" json:"outbox" envconfig:"outbox" yaml:"outbox" toml:"outbox" doc:"Outbox-based delivery configuration."`
	// PartitionLookaheadDays controls how many future daily partitions to
	// pre-create. Required > 0 so writes don't fail at the day rollover.
	// Default: 2 (gives a 48-hour safety window if the lookahead worker stalls).
	PartitionLookaheadDays int `mapstructure:"partition_lookahead_days" json:"partition_lookahead_days" envconfig:"partition_lookahead_days" default:"2" yaml:"partition_lookahead_days" toml:"partition_lookahead_days" doc:"How many future daily partitions to pre-create. Must be greater than 0 so writes don't fail at the day rollover. Default <<2>> (a 48-hour safety window)."`
	// PartitionRetentionDays controls how old a partition can be before it
	// gets dropped whole by the partition retention worker. Default: 7.
	// Set to a large value for longer retention; the special value 0 is
	// internally promoted to 7 (treat "not set" as default).
	PartitionRetentionDays int `mapstructure:"partition_retention_days" json:"partition_retention_days" envconfig:"partition_retention_days" default:"7" yaml:"partition_retention_days" toml:"partition_retention_days" doc:"How many days a partition is kept before the retention worker drops it whole. Default <<7>>. Use a larger value for longer retention."`
}

// PostgresMapBrokerOutbox configures the outbox-based delivery for PostgreSQL map broker.
type PostgresMapBrokerOutbox struct {
	// PollInterval is how often to poll for new stream entries when idle. Default: "100ms".
	PollInterval Duration `mapstructure:"poll_interval" json:"poll_interval" envconfig:"poll_interval" default:"100ms" yaml:"poll_interval" toml:"poll_interval" doc:"How often to poll for new stream entries when idle. Default <<100ms>>."`
	// BatchSize is the maximum number of rows to process per batch. Default: 1000.
	BatchSize int `mapstructure:"batch_size" json:"batch_size" envconfig:"batch_size" default:"1000" yaml:"batch_size" toml:"batch_size" doc:"Maximum number of rows to process per batch. Default <<1000>>."`
}

// PostgresStreamBroker is a configuration for PostgreSQL-based stream broker.
// It implements centrifuge.Broker for stream subscriptions, providing
// transactional publishing alongside business writes in the same SQL transaction.
type PostgresStreamBroker struct {
	// DSN is the primary PostgreSQL connection string.
	DSN string `mapstructure:"dsn" json:"dsn" envconfig:"dsn" yaml:"dsn" toml:"dsn" expose:"url" doc:"Primary PostgreSQL connection string, e.g. <<postgres://user:pass@localhost:5432/dbname?sslmode=disable>>."`
	// TLS is an optional TLS configuration for all PostgreSQL connections
	// (primary, replicas, and notify). Use instead of embedding TLS parameters
	// in the DSN when certificate files are managed outside the connection string.
	TLS TLSConfig `mapstructure:"tls" json:"tls" envconfig:"tls" yaml:"tls" toml:"tls" doc:"Optional TLS configuration applied to all PostgreSQL connections (primary, replicas and notify). Use this instead of embedding TLS parameters in the DSN."`
	// PoolSize sets the maximum number of connections in the pool. Default: 16.
	PoolSize int `mapstructure:"pool_size" json:"pool_size" envconfig:"pool_size" default:"16" yaml:"pool_size" toml:"pool_size" doc:"Maximum number of connections in the pool. Default <<16>>."`
	// NumShards is the total number of shards for parallel delivery workers.
	// Channels are distributed across shards using consistent hashing. Default: 8.
	NumShards int `mapstructure:"num_shards" json:"num_shards" envconfig:"num_shards" default:"8" yaml:"num_shards" toml:"num_shards" doc:"Number of shards for parallel delivery workers. Channels are distributed across shards using consistent hashing. Default <<8>>."`
	// CleanupInterval is how often the cleanup and partition workers tick. Default: "1m".
	CleanupInterval Duration `mapstructure:"cleanup_interval" json:"cleanup_interval" envconfig:"cleanup_interval" default:"1m" yaml:"cleanup_interval" toml:"cleanup_interval" doc:"How often the cleanup and partition workers run. Default <<1m>>."`
	// IdempotentResultTTL is the default TTL for idempotency cache entries. Default: "5m".
	IdempotentResultTTL Duration `mapstructure:"idempotent_result_ttl" json:"idempotent_result_ttl" envconfig:"idempotent_result_ttl" default:"5m" yaml:"idempotent_result_ttl" toml:"idempotent_result_ttl" doc:"Default TTL for idempotency cache entries. Default <<5m>>."`
	// BinaryData uses BYTEA columns instead of JSONB for data fields.
	// Set to true if data payloads are not valid JSON.
	BinaryData bool `mapstructure:"binary_data" json:"binary_data" envconfig:"binary_data" yaml:"binary_data" toml:"binary_data" doc:"Store data in BYTEA columns instead of JSONB. Enable when payloads are not valid JSON (e.g. binary or protobuf)."`
	// StreamRetention is the safety floor for HistoryMetaTTL when neither
	// PublishOptions nor node config sets it. Default: "24h". Guarantees
	// every channel meta row eventually expires.
	StreamRetention Duration `mapstructure:"stream_retention" json:"stream_retention" envconfig:"stream_retention" default:"24h" yaml:"stream_retention" toml:"stream_retention" doc:"Safety floor for history meta TTL when not set per publish or in node config, ensuring every channel meta row eventually expires. Default <<24h>>."`
	// UseNotify enables LISTEN/NOTIFY for low-latency outbox wakeup.
	UseNotify bool `mapstructure:"use_notify" json:"use_notify" envconfig:"use_notify" yaml:"use_notify" toml:"use_notify" doc:"Use PostgreSQL LISTEN/NOTIFY for low-latency outbox wakeup instead of polling only."`
	// NotifyDSN is an optional separate DSN for the LISTEN connection.
	// Required when DSN points at PGBouncer (transaction pooling mode is
	// incompatible with LISTEN/NOTIFY). Must be a direct PostgreSQL URL.
	NotifyDSN string `mapstructure:"notify_dsn" json:"notify_dsn" envconfig:"notify_dsn" yaml:"notify_dsn" toml:"notify_dsn" expose:"url" doc:"Optional separate DSN for the LISTEN connection. Required when the main DSN points at PGBouncer (transaction pooling is incompatible with LISTEN/NOTIFY); must be a direct PostgreSQL URL."`
	// TablePrefix is the namespace prefix for all tables created by this broker.
	// Default: "cf". Multi-tenant deployments sharing one PostgreSQL instance
	// use distinct prefixes per Centrifugo cluster (e.g. "prod_us_cf").
	TablePrefix string `mapstructure:"table_prefix" json:"table_prefix" envconfig:"table_prefix" default:"cf" yaml:"table_prefix" toml:"table_prefix" expose:"full" doc:"Namespace prefix for all tables created by this broker. Default <<cf>>. Use distinct prefixes per cluster when several Centrifugo deployments share one PostgreSQL instance."`
	// SkipSchemaInit disables automatic schema initialization on startup.
	SkipSchemaInit bool `mapstructure:"skip_schema_init" json:"skip_schema_init" envconfig:"skip_schema_init" yaml:"skip_schema_init" toml:"skip_schema_init" doc:"Disable automatic schema initialization on startup. When enabled, you must manage the schema externally (e.g. via migrations)."`
	// Outbox configures the outbox-based delivery mode.
	Outbox PostgresMapBrokerOutbox `mapstructure:"outbox" json:"outbox" envconfig:"outbox" yaml:"outbox" toml:"outbox" doc:"Outbox-based delivery configuration."`
	// PartitionLookaheadDays controls how many future daily partitions to
	// pre-create. Default: 2 (gives a 48-hour safety window).
	PartitionLookaheadDays int `mapstructure:"partition_lookahead_days" json:"partition_lookahead_days" envconfig:"partition_lookahead_days" default:"2" yaml:"partition_lookahead_days" toml:"partition_lookahead_days" doc:"How many future daily partitions to pre-create. Default <<2>> (a 48-hour safety window)."`
	// PartitionRetentionDays controls how old a partition can be before it
	// gets dropped whole by the partition retention worker. Default: 7.
	PartitionRetentionDays int `mapstructure:"partition_retention_days" json:"partition_retention_days" envconfig:"partition_retention_days" default:"7" yaml:"partition_retention_days" toml:"partition_retention_days" doc:"How many days a partition is kept before the retention worker drops it whole. Default <<7>>."`
}

// Controller is a configuration for custom Centrifugo Controller.
// In OSS, only "postgres" type is supported.
type Controller struct {
	Enabled bool `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled" doc:"Enables the Centrifugo controller used for multi-node cluster coordination."`
	// Type of controller to use. Can be "postgres" in OSS. PRO also supports "redis" and "nats".
	Type string `mapstructure:"type" json:"type" envconfig:"type" yaml:"type" toml:"type" expose:"full" doc:"Controller backend type. In OSS only <<postgres>> is supported; PRO also supports <<redis>> and <<nats>>."`
	// Postgres is a configuration for "postgres" controller.
	Postgres PostgresController `mapstructure:"postgres" json:"postgres" envconfig:"postgres" toml:"postgres" yaml:"postgres" doc:"PostgreSQL controller configuration, used when type is <<postgres>>."`
}

// PostgresController configures the PostgreSQL-based controller for multi-node
// cluster coordination. Creates tables with the configured prefix
// (e.g. cf_controller_messages, cf_controller_shard_lock).
type PostgresController struct {
	// DSN is the primary PostgreSQL connection string.
	// Example: "postgres://user:pass@localhost:5432/dbname?sslmode=disable".
	DSN string `mapstructure:"dsn" json:"dsn" envconfig:"dsn" yaml:"dsn" toml:"dsn" expose:"url" doc:"Primary PostgreSQL connection string, e.g. <<postgres://user:pass@localhost:5432/dbname?sslmode=disable>>."`
	// TLS is an optional TLS configuration for all PostgreSQL connections.
	TLS TLSConfig `mapstructure:"tls" json:"tls" envconfig:"tls" yaml:"tls" toml:"tls" doc:"Optional TLS configuration applied to all PostgreSQL connections."`
	// PoolSize sets the maximum number of connections in the pool. Default: 8.
	PoolSize int `mapstructure:"pool_size" json:"pool_size" envconfig:"pool_size" default:"8" yaml:"pool_size" toml:"pool_size" doc:"Maximum number of connections in the pool. Default <<8>>."`
	// NumShards is the number of shards for serialized publishing. Default: 1.
	NumShards int `mapstructure:"num_shards" json:"num_shards" envconfig:"num_shards" default:"1" yaml:"num_shards" toml:"num_shards" doc:"Number of shards for serialized publishing. Default <<1>>."`
	// TablePrefix is the namespace prefix for all tables created by this controller.
	// Default: "cf". Produces names like cf_controller_messages.
	TablePrefix string `mapstructure:"table_prefix" json:"table_prefix" envconfig:"table_prefix" default:"cf" yaml:"table_prefix" toml:"table_prefix" expose:"full" doc:"Namespace prefix for all tables created by this controller. Default <<cf>> (produces names like cf_controller_messages)."`
	// PollInterval is how often to poll for new control messages when idle. Default: "50ms".
	PollInterval Duration `mapstructure:"poll_interval" json:"poll_interval" envconfig:"poll_interval" default:"50ms" yaml:"poll_interval" toml:"poll_interval" doc:"How often to poll for new control messages when idle. Default <<50ms>>."`
	// UseNotify enables LISTEN/NOTIFY for low-latency wakeup. Default: true.
	UseNotify bool `mapstructure:"use_notify" json:"use_notify" envconfig:"use_notify" yaml:"use_notify" toml:"use_notify" doc:"Use PostgreSQL LISTEN/NOTIFY for low-latency wakeup instead of polling only. Default <<true>>."`
	// NotifyDSN is an optional separate DSN for the LISTEN connection.
	// Required when DSN points at PGBouncer (transaction pooling mode is
	// incompatible with LISTEN/NOTIFY). Must be a direct PostgreSQL URL.
	NotifyDSN string `mapstructure:"notify_dsn" json:"notify_dsn" envconfig:"notify_dsn" yaml:"notify_dsn" toml:"notify_dsn" expose:"url" doc:"Optional separate DSN for the LISTEN connection. Required when the main DSN points at PGBouncer (transaction pooling is incompatible with LISTEN/NOTIFY); must be a direct PostgreSQL URL."`
	// PartitionRetentionDays controls how old a partition must be before it is
	// dropped. Default: 1 (control messages are ephemeral).
	PartitionRetentionDays int `mapstructure:"partition_retention_days" json:"partition_retention_days" envconfig:"partition_retention_days" default:"1" yaml:"partition_retention_days" toml:"partition_retention_days" doc:"How many days a partition is kept before it is dropped. Default <<1>> (control messages are ephemeral)."`
	// PartitionLookaheadDays controls how many future daily partitions to
	// pre-create. Default: 2.
	PartitionLookaheadDays int `mapstructure:"partition_lookahead_days" json:"partition_lookahead_days" envconfig:"partition_lookahead_days" default:"2" yaml:"partition_lookahead_days" toml:"partition_lookahead_days" doc:"How many future daily partitions to pre-create. Default <<2>>."`
	// PartitionCleanupInterval is how often the partition worker ticks. Default: "1m".
	PartitionCleanupInterval Duration `mapstructure:"partition_cleanup_interval" json:"partition_cleanup_interval" envconfig:"partition_cleanup_interval" default:"1m" yaml:"partition_cleanup_interval" toml:"partition_cleanup_interval" doc:"How often the partition worker runs. Default <<1m>>."`
	// BatchSize is the maximum number of rows to process per poll batch. Default: 1000.
	BatchSize int `mapstructure:"batch_size" json:"batch_size" envconfig:"batch_size" default:"1000" yaml:"batch_size" toml:"batch_size" doc:"Maximum number of rows to process per poll batch. Default <<1000>>."`
	// SkipSchemaInit disables automatic schema initialization on startup.
	// When true, the schema must be managed externally (e.g. via migrations).
	SkipSchemaInit bool `mapstructure:"skip_schema_init" json:"skip_schema_init" envconfig:"skip_schema_init" yaml:"skip_schema_init" toml:"skip_schema_init" doc:"Disable automatic schema initialization on startup. When enabled, you must manage the schema externally (e.g. via migrations)."`
}
