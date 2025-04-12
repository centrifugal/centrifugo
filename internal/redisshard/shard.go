package redisshard

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redis/rueidis"
)

const (
	defaultRedisIOTimeout      = 4 * time.Second
	defaultRedisConnectTimeout = time.Second
)

type RedisShard struct {
	config        RedisShardConfig
	client        rueidis.Client
	replicaClient rueidis.Client
	closeCh       chan struct{}
	closeOnce     sync.Once
	isCluster     bool
	finalAddress  []string
}

// NewRedisShard initializes new Redis shard.
func NewRedisShard(conf RedisShardConfig) (*RedisShard, error) {
	if conf.ConnectTimeout == 0 {
		conf.ConnectTimeout = defaultRedisConnectTimeout
	}
	if conf.IOTimeout == 0 {
		conf.IOTimeout = defaultRedisIOTimeout
	}
	options := rueidis.ClientOption{
		SelectDB:         conf.DB,
		ConnWriteTimeout: conf.IOTimeout,
		TLSConfig:        conf.TLSConfig,
		Username:         conf.User,
		Password:         conf.Password,
		ClientName:       conf.ClientName,
		ShuffleInit:      true,
		DisableCache:     true,
		AlwaysPipelining: true,
		AlwaysRESP2:      conf.ForceRESP2,
		MaxFlushDelay:    100 * time.Microsecond,
		Dialer: net.Dialer{
			Timeout: conf.ConnectTimeout,
		},
	}

	var isCluster bool
	var isSentinel bool
	replicaClientEnabled := conf.ReplicaClientEnabled

	if len(conf.SentinelAddresses) > 0 {
		isSentinel = true
		options.InitAddress = conf.SentinelAddresses
		options.Sentinel = rueidis.SentinelOption{
			TLSConfig:  conf.SentinelTLSConfig,
			MasterSet:  conf.SentinelMasterName,
			Username:   conf.SentinelUser,
			Password:   conf.SentinelPassword,
			ClientName: conf.SentinelClientName,
		}
	} else if len(conf.ClusterAddresses) > 0 {
		isCluster = true
		options.InitAddress = conf.ClusterAddresses
	} else {
		addressOpts, err := optionsFromAddress(conf.Address, options)
		if err != nil {
			return nil, fmt.Errorf("error processing Redis address: %v", err)
		}
		options, isCluster, isSentinel, replicaClientEnabled =
			addressOpts.ClientOption, addressOpts.IsCluster, addressOpts.IsSentinel, addressOpts.ReplicaClientEnabled
	}

	if isSentinel && options.Sentinel.MasterSet == "" {
		return nil, errors.New("sentinel master name must be configured for Redis Sentinel setup")
	}

	client, err := rueidis.NewClient(options)
	if err != nil {
		return nil, fmt.Errorf("error creating Redis client: %v", err)
	}
	if strings.Contains(strings.ToLower(fmt.Sprintf("%T", client)), "cluster") {
		// Cluster mode is not explicitly set but client is a cluster client – thus set isCluster to true.
		// This scenario covered with tests for our main integrations: see TestNewRedisShard.
		// Centrifuge need to know that it's working with Redis Cluster to construct proper keys.
		isCluster = true
	}

	shard := &RedisShard{
		config:       conf,
		isCluster:    isCluster,
		closeCh:      make(chan struct{}),
		finalAddress: options.InitAddress,
	}

	shard.client = client

	if replicaClientEnabled {
		if !isCluster && !isSentinel {
			return nil, errors.New("replica client may be initialized only in cluster and sentinel mode")
		}
		options.ReplicaOnly = true
		replicaClient, err := rueidis.NewClient(options)
		if err != nil {
			return nil, fmt.Errorf("error creating Redis replica client: %w", err)
		}
		shard.replicaClient = replicaClient
	}

	return shard, nil
}

var knownRedisURLPrefixes = []string{
	"redis://",
	"rediss://",
	"redis+sentinel://",
	"rediss+sentinel://",
	"redis+cluster://",
	"unix://",
	"tcp://",
}

type fromAddressOptions struct {
	ClientOption         rueidis.ClientOption
	IsCluster            bool
	IsSentinel           bool
	ReplicaClientEnabled bool
}

func optionsFromAddress(address string, options rueidis.ClientOption) (fromAddressOptions, error) {
	result := fromAddressOptions{ClientOption: options}

	hasKnownURLPrefix := false
	for _, prefix := range knownRedisURLPrefixes {
		if strings.HasPrefix(address, prefix) {
			hasKnownURLPrefix = true
			break
		}
	}

	if !hasKnownURLPrefix {
		if host, port, err := net.SplitHostPort(address); err == nil && host != "" && port != "" {
			result.ClientOption.InitAddress = []string{address}
			return result, nil
		}
		return result, errors.New("malformed connection address, must be Redis URL or host:port")
	}

	u, err := url.Parse(address)
	if err != nil {
		return result, fmt.Errorf("malformed connection address, not a valid URL: %w", err)
	}

	var addresses []string

	query := u.Query()

	if query.Has("sentinel_master_name") {
		result.ClientOption.Sentinel.MasterSet = query.Get("sentinel_master_name")
	}

	isCluster := u.Scheme == "redis+cluster"
	isSentinel := u.Scheme == "redis+sentinel" || u.Scheme == "rediss+sentinel" || result.ClientOption.Sentinel.MasterSet != ""
	if isSentinel && result.ClientOption.Sentinel.MasterSet == "" {
		return result, errors.New("sentinel master name must be configured for Redis Sentinel setup")
	}

	switch u.Scheme {
	case "tcp", "redis", "redis+sentinel", "redis+cluster", "rediss", "rediss+sentinel":
		addresses = []string{u.Host}
		if u.Path != "" {
			db, err := strconv.Atoi(strings.TrimPrefix(u.Path, "/"))
			if err != nil {
				return result, fmt.Errorf("can't parse Redis DB number from connection address: %s is not a number", u.Path)
			}
			result.ClientOption.SelectDB = db
		}
		if strings.HasPrefix(u.Scheme, "rediss") {
			if result.ClientOption.TLSConfig == nil {
				result.ClientOption.TLSConfig = &tls.Config{}
			}
			if isSentinel && result.ClientOption.Sentinel.TLSConfig == nil {
				result.ClientOption.Sentinel.TLSConfig = &tls.Config{}
			}
		}
	case "unix":
		addresses = []string{u.Path}
		result.ClientOption.DialFn = func(s string, d *net.Dialer, c *tls.Config) (net.Conn, error) {
			return d.Dial("unix", s)
		}
	}

	if u.User != nil {
		if u.User.Username() != "" {
			result.ClientOption.Username = u.User.Username()
		}
		if pass, ok := u.User.Password(); ok {
			result.ClientOption.Password = pass
		}
	}

	addresses = append(addresses, query["addr"]...)

	if query.Has("connect_timeout") {
		to, err := time.ParseDuration(query.Get("connect_timeout"))
		if err != nil {
			return result, fmt.Errorf("invalid connect timeout: %q", query.Get("connect_timeout"))
		}
		result.ClientOption.Dialer.Timeout = to
	}

	if query.Has("io_timeout") {
		to, err := time.ParseDuration(query.Get("io_timeout"))
		if err != nil {
			return result, fmt.Errorf("invalid io timeout: %q", query.Get("io_timeout"))
		}
		result.ClientOption.ConnWriteTimeout = to
	}

	if query.Has("tls_enabled") && result.ClientOption.TLSConfig == nil {
		val, err := strconv.ParseBool(query.Get("tls_enabled"))
		if err != nil {
			return result, fmt.Errorf("invalid tls_enabled value: %q", query.Get("tls_enabled"))
		}
		if val {
			result.ClientOption.TLSConfig = &tls.Config{}
		}
	}

	if query.Has("force_resp2") {
		val, err := strconv.ParseBool(query.Get("force_resp2"))
		if err != nil {
			return result, fmt.Errorf("invalid force_resp2 value: %q", query.Get("force_resp2"))
		}
		result.ClientOption.AlwaysRESP2 = val
	}

	if query.Has("sentinel_user") {
		result.ClientOption.Sentinel.Username = query.Get("sentinel_user")
	}

	if query.Has("sentinel_password") {
		result.ClientOption.Sentinel.Password = query.Get("sentinel_password")
	}

	if query.Has("sentinel_tls_enabled") && result.ClientOption.Sentinel.TLSConfig == nil {
		val, err := strconv.ParseBool(query.Get("sentinel_tls_enabled"))
		if err != nil {
			return result, fmt.Errorf("invalid sentinel_tls_enabled value: %q", query.Get("sentinel_tls_enabled"))
		}
		if val {
			result.ClientOption.Sentinel.TLSConfig = &tls.Config{}
		}
	}

	if query.Has("replica_client_enabled") {
		val, err := strconv.ParseBool(query.Get("replica_client_enabled"))
		if err != nil {
			return result, fmt.Errorf("invalid replica_client_enabled value: %q", query.Get("replica_client_enabled"))
		}
		result.ReplicaClientEnabled = val
	}

	result.ClientOption.InitAddress = addresses
	result.IsCluster = isCluster
	result.IsSentinel = isSentinel

	return result, nil
}

// RedisShardConfig contains Redis connection options.
type RedisShardConfig struct {
	// Address is a Redis server connection address. Address can be:
	// - host:port
	// - tcp://[[[user]:password]@]host:port[/db][?option1=value1&optionN=valueN]
	// - redis://[[[user]:password]@]host:port[/db][?option1=value1&optionN=valueN]
	// - unix://[[[user]:password]@]path[?option1=value1&optionN=valueN]
	// It's also possible to use Address with redis+sentinel:// and redis+cluster://
	// schemes when connecting to Redis Sentinel and Redis Cluster respectively.
	// Examples:
	// - redis+sentinel://[[[user]:password]@]host:port?sentinel_master_name=mymaster
	// - redis+cluster://[[[user]:password]@]host:port[?addr=host2:port2&addr=host3:port3]
	// If you need to connect to Redis Cluster then you need to provide ClusterAddresses
	// or must use redis+cluster:// scheme in Address.
	// If you need to connect to Redis Sentinel then you need to provide SentinelAddresses
	// or must use redis+sentinel:// scheme in Address.
	// I.e. Centrifuge requires you to explicitly specify the type of Redis setup you want
	// to connect to.
	Address string
	// ClusterAddresses is a slice of seed cluster addresses to connect to.
	// Each address should be in form of host:port. If ClusterAddresses set then
	// RedisShardConfig.Address not used at all.
	ClusterAddresses []string
	// SentinelAddresses is a slice of Sentinel addresses. Each address should
	// be in form of host:port. If set then Redis address will be automatically
	// discovered from Sentinel. For Sentinel the name of the master instance
	// Sentinel monitors (SentinelMasterName) must be provided. If SentinelAddresses
	// set then RedisShardConfig.Address not used at all.
	SentinelAddresses []string

	// SentinelMasterName is a name of Redis instance master Sentinel monitors.
	SentinelMasterName string
	// SentinelUser is a user for Sentinel ACL-based auth.
	SentinelUser string
	// SentinelPassword is a password for Sentinel. Works with Sentinel >= 5.0.1.
	SentinelPassword string
	// SentinelClientName is a client name for established connections to Sentinel.
	SentinelClientName string
	// SentinelTLSConfig is a TLS configuration for Sentinel connections.
	SentinelTLSConfig *tls.Config

	// DB is Redis database number. If not set then database 0 used. Does not make sense in Redis Cluster case.
	DB int
	// User is a username for Redis ACL-based auth.
	User string
	// Password is password to use when connecting to Redis database.
	// If zero then password not used.
	Password string
	// ClientName for established connections. See https://redis.io/commands/client-setname/
	ClientName string
	// TLSConfig contains connection TLS configuration.
	TLSConfig *tls.Config

	// ConnectTimeout is a timeout on connect operation.
	// By default, 1 second is used.
	ConnectTimeout time.Duration
	// IOTimeout is a timeout on Redis connection operations.
	// By default, 4 seconds is used.
	IOTimeout time.Duration

	// ForceRESP2 if set to true forces using RESP2 protocol for communicating with Redis.
	// By default, Redis client tries to detect supported Redis protocol automatically
	// trying RESP3 first.
	ForceRESP2 bool

	// ReplicaClientEnabled once set to true will initialize replica client for this shard.
	// Replica client can then be used for read-only operations from replica nodes in Redis
	// Cluster or Redis Sentinel setups (single Redis is not allowed). Replica client will
	// be initialized with the same options as the main client but with ReplicaOnly option
	// set to true.
	ReplicaClientEnabled bool
}

func (s *RedisShard) Close() {
	s.closeOnce.Do(func() {
		close(s.closeCh)
		s.client.Close()
		if s.replicaClient != nil {
			s.replicaClient.Close()
		}
	})
}

func (s *RedisShard) String() string {
	return strings.Join(s.finalAddress, ",")
}

type Op func(client rueidis.Client) rueidis.RedisResult

// RunOp allows to run Op on selected shard.
func (s *RedisShard) RunOp(op Op) rueidis.RedisResult {
	return op(s.client)
}

// RunOpReplica allows to run Op on selected shard, possibly on replica.
func (s *RedisShard) RunOpReplica(op Op, onReplica bool) rueidis.RedisResult {
	if onReplica && s.replicaClient != nil {
		return op(s.replicaClient)
	}
	return op(s.client)
}

type MultiOp func(client rueidis.Client) []rueidis.RedisResult

// RunMulti allows executing multiple commands.
func (s *RedisShard) RunMulti(op MultiOp) []rueidis.RedisResult {
	return op(s.client)
}
