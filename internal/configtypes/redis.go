package configtypes

type Redis struct {
	// Address is a list of Redis shard addresses. In most cases a single shard is used. But when many
	// addresses provided Centrifugo will distribute keys between shards using consistent hashing.
	Address []string `mapstructure:"address" json:"address" envconfig:"address" default:"redis://127.0.0.1:6379" yaml:"address" toml:"address" expose:"url" doc:"List of Redis shard addresses, e.g. <<redis://127.0.0.1:6379>>. Provide several to shard keys across shards using consistent hashing."`
	// ConnectTimeout is a timeout for establishing connection to Redis.
	ConnectTimeout Duration `mapstructure:"connect_timeout" json:"connect_timeout" envconfig:"connect_timeout" default:"1s" yaml:"connect_timeout" toml:"connect_timeout" doc:"Timeout for establishing a connection to Redis. Default <<1s>>."`
	// IOTimeout is a timeout for all read/write operations against Redis (can be considered as a request timeout).
	IOTimeout Duration `mapstructure:"io_timeout" json:"io_timeout" envconfig:"io_timeout" default:"4s" yaml:"io_timeout" toml:"io_timeout" doc:"Timeout for read/write operations against Redis (effectively a request timeout). Default <<4s>>."`
	// DB is a Redis database to use. Generally it's not recommended to use non-zero DB. Note, that Redis
	// PUB/SUB is global for all databases in a single Redis instance. So when using non-zero DB make sure
	// that different Centrifugo setups use different prefixes.
	DB int `mapstructure:"db" json:"db" envconfig:"db" default:"0" yaml:"db" toml:"db" doc:"Redis database number to use. Using a non-zero DB is generally not recommended since Redis PUB/SUB is shared across all databases; if you do, make sure different Centrifugo setups use different prefixes."`
	// User is a Redis user.
	User string `mapstructure:"user" json:"user" envconfig:"user" yaml:"user" toml:"user" expose:"full" doc:"Redis user for authentication (Redis ACL)."`
	// Password is a Redis password.
	Password string `mapstructure:"password" json:"password" envconfig:"password" yaml:"password" toml:"password" doc:"Redis password for authentication."`
	// ClientName allows changing a Redis client name used when connecting.
	ClientName string `mapstructure:"client_name" json:"client_name" envconfig:"client_name" yaml:"client_name" toml:"client_name" expose:"full" doc:"Client name Centrifugo presents when connecting to Redis."`
	// ForceResp2 forces use of Redis Resp2 protocol for communication.
	ForceResp2 bool `mapstructure:"force_resp2" json:"force_resp2" envconfig:"force_resp2" yaml:"force_resp2" toml:"force_resp2" doc:"Forces use of the Redis RESP2 protocol instead of RESP3."`
	// ClusterAddress is a list of Redis cluster addresses. When several provided - data will be sharded
	// between them using consistent hashing. Several Cluster addresses within one shard may be passed
	// comma-separated.
	ClusterAddress []string `mapstructure:"cluster_address" json:"cluster_address" envconfig:"cluster_address" yaml:"cluster_address" toml:"cluster_address" expose:"url" doc:"List of Redis Cluster addresses to enable cluster mode. Provide several to shard across clusters using consistent hashing; addresses within one shard may be comma-separated."`
	// TLS is a configuration for Redis TLS support.
	TLS TLSConfig `mapstructure:"tls" json:"tls" envconfig:"tls" yaml:"tls" toml:"tls" doc:"TLS configuration for connections to Redis."`
	// SentinelAddress allows setting Redis Sentinel addresses. When provided - Sentinel will be used.
	// When multiple addresses provided - data will be sharded between them using consistent hashing.
	// Several Sentinel addresses within one shard may be passed comma-separated.
	SentinelAddress []string `mapstructure:"sentinel_address" json:"sentinel_address" envconfig:"sentinel_address" yaml:"sentinel_address" toml:"sentinel_address" expose:"url" doc:"List of Redis Sentinel addresses to enable Sentinel mode. Provide several to shard across them using consistent hashing; addresses within one shard may be comma-separated."`
	// SentinelUser is a Redis Sentinel user.
	SentinelUser string `mapstructure:"sentinel_user" json:"sentinel_user" envconfig:"sentinel_user" yaml:"sentinel_user" toml:"sentinel_user" expose:"full" doc:"User for authenticating with Redis Sentinel."`
	// SentinelPassword is a Redis Sentinel password.
	SentinelPassword string `mapstructure:"sentinel_password" json:"sentinel_password" envconfig:"sentinel_password" yaml:"sentinel_password" toml:"sentinel_password" doc:"Password for authenticating with Redis Sentinel."`
	// SentinelMasterName is a Redis master name in Sentinel setup.
	SentinelMasterName string `mapstructure:"sentinel_master_name" json:"sentinel_master_name" envconfig:"sentinel_master_name" yaml:"sentinel_master_name" toml:"sentinel_master_name" expose:"full" doc:"Name of the Redis master to discover via Sentinel. Required when using Sentinel."`
	// SentinelClientName is a Redis Sentinel client name used when connecting.
	SentinelClientName string `mapstructure:"sentinel_client_name" json:"sentinel_client_name" envconfig:"sentinel_client_name" yaml:"sentinel_client_name" toml:"sentinel_client_name" expose:"full" doc:"Client name Centrifugo presents when connecting to Redis Sentinel."`
	// SentinelTLS is a configuration for Redis Sentinel TLS support.
	SentinelTLS TLSConfig `mapstructure:"sentinel_tls" json:"sentinel_tls" envconfig:"sentinel_tls" yaml:"sentinel_tls" toml:"sentinel_tls" doc:"TLS configuration for connections to Redis Sentinel."`
	// ReplicaClient is a configuration for Redis replica client.
	ReplicaClient RedisReplicaClient `mapstructure:"replica_client" json:"replica_client" envconfig:"replica_client" yaml:"replica_client" toml:"replica_client" doc:"Configuration of a Redis replica client to offload read operations to replicas."`
	// AWSIAMAuth enables AWS IAM authentication for Amazon ElastiCache (Valkey or Redis OSS).
	AWSIAMAuth RedisAWSIAMAuth `mapstructure:"aws_iam_auth" json:"aws_iam_auth" envconfig:"aws_iam_auth" yaml:"aws_iam_auth" toml:"aws_iam_auth" doc:"AWS IAM authentication for Amazon ElastiCache (Valkey or Redis OSS)."`
	// GCPIAMAuth enables Google Cloud IAM authentication for Memorystore (Valkey or Redis Cluster).
	GCPIAMAuth RedisGCPIAMAuth `mapstructure:"gcp_iam_auth" json:"gcp_iam_auth" envconfig:"gcp_iam_auth" yaml:"gcp_iam_auth" toml:"gcp_iam_auth" doc:"Google Cloud IAM authentication for Memorystore (Valkey or Redis Cluster)."`
	// AzureEntraAuth enables Microsoft Entra ID authentication for Azure Managed Redis and Azure Cache for Redis.
	AzureEntraAuth RedisAzureEntraAuth `mapstructure:"azure_entra_auth" json:"azure_entra_auth" envconfig:"azure_entra_auth" yaml:"azure_entra_auth" toml:"azure_entra_auth" doc:"Microsoft Entra ID authentication for Azure Managed Redis and Azure Cache for Redis."`
}

type RedisPrefixed struct {
	// Prefix for all Redis keys and channels.
	Prefix string `mapstructure:"prefix" json:"prefix" envconfig:"prefix" default:"centrifugo" yaml:"prefix" toml:"prefix" expose:"full" doc:"Prefix for all Redis keys and channels used by Centrifugo. Default <<centrifugo>>."`
	Redis  `mapstructure:",squash" yaml:",inline"`
}

// RedisEmptyPrefixed is like RedisPrefixed but defaults to an empty prefix.
// Useful for standalone pub/sub channels where a prefix is not needed by default.
type RedisEmptyPrefixed struct {
	Prefix string `mapstructure:"prefix" json:"prefix" envconfig:"prefix" yaml:"prefix" toml:"prefix" expose:"full" doc:"Prefix for Redis keys and channels. Empty by default."`
	Redis  `mapstructure:",squash" yaml:",inline"`
}

// RedisReplicaClient allows configuring Redis replica options.
type RedisReplicaClient struct {
	// Enabled enables replica client.
	Enabled bool `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled" doc:"Enables a separate Redis replica client so read operations can be served from replicas."`
}

// RedisAWSIAMAuth configures AWS IAM authentication for Amazon ElastiCache. Centrifugo generates
// short-lived IAM auth tokens using AWS credentials from the default AWS SDK chain (environment,
// shared config, ECS task role, EC2 instance metadata, etc.) and uses them as a Redis password.
// The Redis user must be set to the ElastiCache IAM-enabled user ID.
type RedisAWSIAMAuth struct {
	// Enabled enables AWS IAM authentication.
	Enabled bool `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled" doc:"Enables AWS IAM authentication for Amazon ElastiCache. Requires <<user>> to be set to the IAM-enabled ElastiCache user ID, TLS must be enabled on the cache."`
	// ReplicationGroupID of a node-based ElastiCache cache. Exactly one of ReplicationGroupID
	// and ServerlessCacheName must be set.
	ReplicationGroupID string `mapstructure:"replication_group_id" json:"replication_group_id" envconfig:"replication_group_id" yaml:"replication_group_id" toml:"replication_group_id" expose:"full" doc:"Replication group ID of a node-based ElastiCache cache. Set either this or <<serverless_cache_name>>."`
	// ServerlessCacheName of an ElastiCache Serverless cache. Exactly one of ReplicationGroupID
	// and ServerlessCacheName must be set.
	ServerlessCacheName string `mapstructure:"serverless_cache_name" json:"serverless_cache_name" envconfig:"serverless_cache_name" yaml:"serverless_cache_name" toml:"serverless_cache_name" expose:"full" doc:"Name of an ElastiCache Serverless cache. Set either this or <<replication_group_id>>."`
	// Region is the AWS region of the cache. If empty, the region from the default AWS SDK chain is used.
	Region string `mapstructure:"region" json:"region" envconfig:"region" yaml:"region" toml:"region" expose:"full" doc:"AWS region of the cache. If empty, the region is resolved from the default AWS SDK chain (e.g. <<AWS_REGION>>)."`
}

// RedisGCPIAMAuth configures Google Cloud IAM authentication for Memorystore. Centrifugo obtains
// OAuth2 access tokens using Google Cloud Application Default Credentials (ADC) and uses them
// as a Redis password.
type RedisGCPIAMAuth struct {
	// Enabled enables Google Cloud IAM authentication.
	Enabled bool `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled" doc:"Enables Google Cloud IAM authentication for Memorystore using Application Default Credentials (ADC)."`
}

// RedisAzureEntraAuth configures Microsoft Entra ID authentication for Azure Managed Redis and
// Azure Cache for Redis. Centrifugo obtains access tokens using DefaultAzureCredential (environment,
// workload identity, managed identity, etc.), uses them as a Redis password and re-authenticates
// established connections before the token expires.
type RedisAzureEntraAuth struct {
	// Enabled enables Microsoft Entra ID authentication.
	Enabled bool `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled" doc:"Enables Microsoft Entra ID authentication using DefaultAzureCredential. If <<user>> is empty, the object ID from the access token is used as a username."`
}
