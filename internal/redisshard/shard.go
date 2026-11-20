package redisshard

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"math/rand/v2"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/redisiam"

	"github.com/redis/rueidis"
	"github.com/rs/zerolog/log"
)

const (
	defaultRedisIOTimeout      = 4 * time.Second
	defaultRedisConnectTimeout = time.Second

	// sentinelTopologyRefreshInterval makes the Sentinel client periodically
	// re-check its targets with Sentinels instead of relying solely on the
	// +switch-master event. That event is the only way the client learns about
	// a failover, and it can be missed — a demoted master keeps its connection
	// alive and a role change sends no MOVED, so a client which missed the
	// event stays pinned to a node that is now a read-only replica until
	// restart. This is easy to hit with a Sentinel co-located with each Redis
	// node (as in the Bitnami Redis Helm chart), where the client loses its
	// Sentinel connection at the same moment the master dies and Redis PUB/SUB
	// does not replay the missed message.
	sentinelTopologyRefreshInterval = 5 * time.Second

	// Delays between connection attempts made during StartupWaitTimeout grow
	// from the initial one to the max one, each shortened by a random jitter.
	redisStartupWaitInitialDelay = 100 * time.Millisecond
	redisStartupWaitMaxDelay     = 5 * time.Second
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

// redisClientOptions builds the rueidis options of a shard from its config
// (with ConnectTimeout and IOTimeout already set). It reports whether the
// shard is a Redis Cluster or a Sentinel setup, and whether a replica client
// is enabled.
func redisClientOptions(conf RedisShardConfig) (options rueidis.ClientOption, isCluster bool, isSentinel bool, replicaClientEnabled bool, err error) {
	options = rueidis.ClientOption{
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
		// Redis commands are issued without per-request context deadlines
		// and rely on IOTimeout to bound every call. rueidis retries
		// read-only commands on network errors until the context is done —
		// which, without a deadline, means a read command issued during a
		// Redis outage never returns instead of failing within the IOTimeout
		// envelope like writes do. Disable client-level retries entirely:
		// errors are surfaced to callers, and reads and writes should fail
		// the same way. Same change as in centrifuge RedisShard.
		DisableRetry: true,
		Dialer: net.Dialer{
			Timeout: conf.ConnectTimeout,
			// KeepAlive doubles as the rueidis keepalive PING cadence — the
			// mechanism that detects a silently stalled connection (peer
			// stops replying while TCP stays open) and errors out every
			// pending command. Worst-case failure time for a command on a
			// stalled connection is 2*KeepAlive + IOTimeout: a straggler
			// reply can make one keepalive tick look active, only the next
			// tick sends a PING, and the PING waits a full IOTimeout. The
			// rueidis default of 1s gives 6s with the default 4s IOTimeout;
			// 400ms keeps the worst case within 5s (4.8s), since commands
			// are issued without per-request context deadlines. Same change
			// as in centrifuge RedisShard, where a regression test pins the
			// bound against a blackholed connection.
			KeepAlive: 400 * time.Millisecond,
		},
	}

	if conf.CredentialsProvider != nil {
		options.AuthCredentialsFn = func(_ rueidis.AuthCredentialsContext) (rueidis.AuthCredentials, error) {
			creds, err := conf.CredentialsProvider()
			if err != nil {
				return rueidis.AuthCredentials{}, err
			}
			return rueidis.AuthCredentials{
				Username:     creds.Username,
				Password:     creds.Password,
				RefreshAfter: creds.RefreshAfter,
			}, nil
		}
	}

	replicaClientEnabled = conf.ReplicaClientEnabled

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
			return rueidis.ClientOption{}, false, false, false, fmt.Errorf("error processing Redis address: %v", err)
		}
		options, isCluster, isSentinel, replicaClientEnabled =
			addressOpts.ClientOption, addressOpts.IsCluster, addressOpts.IsSentinel, addressOpts.ReplicaClientEnabled
	}

	if isSentinel {
		if options.Sentinel.MasterSet == "" {
			return rueidis.ClientOption{}, false, false, false, errors.New("sentinel master name must be configured for Redis Sentinel setup")
		}
		options.Sentinel.TopologyRefreshInterval = sentinelTopologyRefreshInterval
		// Sentinel connections get the connect timeout and keep-alive of the
		// data connections. rueidis dials Sentinels with Sentinel.Dialer only,
		// and a zero dialer has no connect timeout and no keep-alive PING. A
		// Sentinel whose packets are silently dropped then holds the client:
		// a dial waits for the kernel to give up, a request without an answer
		// waits for TCP retransmissions, and the topology refresh waits behind
		// both. Meanwhile the client stays on the old master's address after a
		// failover (centrifugo#1325). Same change as in centrifuge RedisShard.
		options.Sentinel.Dialer = options.Dialer
	}
	return options, isCluster, isSentinel, replicaClientEnabled, nil
}

// NewRedisShard initializes new Redis shard.
func NewRedisShard(conf RedisShardConfig) (*RedisShard, error) {
	if conf.ConnectTimeout == 0 {
		conf.ConnectTimeout = defaultRedisConnectTimeout
	}
	if conf.IOTimeout == 0 {
		conf.IOTimeout = defaultRedisIOTimeout
	}
	options, isCluster, isSentinel, replicaClientEnabled, err := redisClientOptions(conf)
	if err != nil {
		return nil, err
	}

	// One deadline for both clients: StartupWaitTimeout bounds how long
	// NewRedisShard blocks in total. Same as in centrifuge RedisShard.
	var startupDeadline time.Time
	if conf.StartupWaitTimeout > 0 {
		startupDeadline = time.Now().Add(conf.StartupWaitTimeout)
	}

	client, err := newRedisClient(options, startupDeadline)
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
			client.Close()
			return nil, errors.New("replica client may be initialized only in cluster and sentinel mode")
		}
		options.ReplicaOnly = true
		replicaClient, err := newRedisClient(options, startupDeadline)
		if err != nil {
			client.Close()
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
				result.ClientOption.TLSConfig = &tls.Config{
					MinVersion: tls.VersionTLS12,
				}
			}
			if isSentinel && result.ClientOption.Sentinel.TLSConfig == nil {
				result.ClientOption.Sentinel.TLSConfig = &tls.Config{
					MinVersion: tls.VersionTLS12,
				}
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
			result.ClientOption.TLSConfig = &tls.Config{
				MinVersion: tls.VersionTLS12,
			}
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
			result.ClientOption.Sentinel.TLSConfig = &tls.Config{
				MinVersion: tls.VersionTLS12,
			}
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

// newRedisClient creates a Redis client. Without a deadline it makes one
// attempt. With it, it keeps trying until Redis accepts a connection and
// answers PING, so a Redis which is not reachable yet or still loads its
// dataset is waited for until the deadline.
func newRedisClient(options rueidis.ClientOption, deadline time.Time) (rueidis.Client, error) {
	if deadline.IsZero() {
		return rueidis.NewClient(options)
	}
	connect := func() (rueidis.Client, error) {
		client, err := rueidis.NewClient(options)
		if err != nil {
			return nil, err
		}
		// An address may set io_timeout to zero, the client then uses a
		// default of its own: a zero timeout here would fail every PING.
		ioTimeout := options.ConnWriteTimeout
		if ioTimeout <= 0 {
			ioTimeout = defaultRedisIOTimeout
		}
		ctx, cancel := context.WithTimeout(context.Background(), ioTimeout)
		defer cancel()
		if err := client.Do(ctx, client.B().Ping().Build()).Error(); err != nil {
			client.Close()
			return nil, err
		}
		return client, nil
	}
	onRetry := func(err error, delay time.Duration) {
		log.Info().Err(err).
			Str("address", strings.Join(options.InitAddress, ",")).
			Str("retry_in", delay.String()).
			Msg("waiting for Redis on startup")
	}
	return waitForRedisStartup(deadline, connect, onRetry)
}

// errRedisStartupWaitTimeout is returned when the startup deadline passes with
// no failed attempt to report.
var errRedisStartupWaitTimeout = errors.New("timeout waiting for Redis on startup")

// waitForRedisStartup calls connect until it succeeds, fails with an error
// which waiting can't fix, or the deadline passes. In the last case it returns
// the error of the last failed attempt. It calls onRetry before each delay.
//
// The deadline bounds the attempts too: an attempt takes up to ConnectTimeout
// plus IOTimeout, so one still in flight at the deadline is left to finish in
// the background, and a client it then creates is closed.
func waitForRedisStartup(
	deadline time.Time,
	connect func() (rueidis.Client, error),
	onRetry func(err error, delay time.Duration),
) (rueidis.Client, error) {
	type attempt struct {
		client rueidis.Client
		err    error
	}
	lastErr := errRedisStartupWaitTimeout
	maxDelay := redisStartupWaitInitialDelay
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return nil, lastErr
		}
		done := make(chan attempt, 1)
		go func() {
			client, err := connect()
			done <- attempt{client: client, err: err}
		}()
		timer := time.NewTimer(remaining)
		select {
		case res := <-done:
			timer.Stop()
			if res.err == nil || !isRedisStartupErrorTransient(res.err) {
				return res.client, res.err
			}
			lastErr = res.err
		case <-timer.C:
			go func() {
				if res := <-done; res.client != nil {
					res.client.Close()
				}
			}()
			return nil, lastErr
		}
		remaining = time.Until(deadline)
		if remaining <= 0 {
			return nil, lastErr
		}
		// Sleep from a half of maxDelay to the full maxDelay.
		//nolint:gosec // it's a jitter.
		delay := maxDelay/2 + rand.N(maxDelay/2+1)
		delay = min(delay, remaining)
		onRetry(lastErr, delay)
		time.Sleep(delay)
		maxDelay = min(2*maxDelay, redisStartupWaitMaxDelay)
	}
}

// isRedisStartupErrorTransient tells whether an error of a connection attempt
// may go away while waiting. A network error may: Redis is not up yet. Of the
// errors Redis replies with only LOADING may, the rest (wrong password, wrong
// database) come from the configuration.
func isRedisStartupErrorTransient(err error) bool {
	if redisErr, ok := rueidis.IsRedisErr(err); ok {
		return redisErr.IsLoading()
	}
	return true
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
	// StartupWaitTimeout is how long NewRedisShard keeps trying to connect to
	// a Redis which is not available yet: not reachable, or still loading its
	// dataset. It bounds NewRedisShard in total, the replica client included.
	// Attempts are made with a backoff of up to 5 seconds. An error which
	// waiting can't fix, like a wrong password, is returned right away.
	// By default, zero: NewRedisShard makes one attempt to connect.
	StartupWaitTimeout time.Duration

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

	// CredentialsProvider is an optional function to dynamically provide auth credentials
	// for each new connection (used for cloud IAM auth). Overrides User and Password.
	CredentialsProvider redisiam.Provider
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

// Publish sends data to a Redis PUB/SUB channel.
// Uses SPUBLISH for cluster mode, PUBLISH otherwise.
func (s *RedisShard) Publish(ctx context.Context, channel string, data []byte) error {
	var cmd rueidis.Completed
	if s.isCluster {
		cmd = s.client.B().Spublish().Channel(channel).Message(string(data)).Build()
	} else {
		cmd = s.client.B().Publish().Channel(channel).Message(string(data)).Build()
	}
	return s.client.Do(ctx, cmd).Error()
}

// Subscribe listens on a Redis PUB/SUB channel and calls handler for each message.
// Uses SSUBSCRIBE for cluster mode, SUBSCRIBE otherwise.
// Blocks until ctx is cancelled. Handles reconnections automatically.
func (s *RedisShard) Subscribe(ctx context.Context, channel string, handler func(data []byte)) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		err := s.runSubscribeLoop(ctx, channel, handler)
		if err != nil && ctx.Err() != nil {
			return ctx.Err()
		}
		// Reconnect after a short delay.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(300 * time.Millisecond):
		}
	}
}

func (s *RedisShard) runSubscribeLoop(ctx context.Context, channel string, handler func(data []byte)) error {
	conn, cancel := s.client.Dedicate()
	defer cancel()
	defer conn.Close()

	done := make(chan struct{})
	var doneOnce sync.Once

	wait := conn.SetPubSubHooks(rueidis.PubSubHooks{
		OnMessage: func(msg rueidis.PubSubMessage) {
			handler([]byte(msg.Message))
		},
		OnSubscription: func(ps rueidis.PubSubSubscription) {
			if s.isCluster && ps.Kind == "sunsubscribe" && ps.Channel == channel {
				doneOnce.Do(func() { close(done) })
			}
		},
	})

	var err error
	if s.isCluster {
		err = conn.Do(ctx, conn.B().Ssubscribe().Channel(channel).Build()).Error()
	} else {
		err = conn.Do(ctx, conn.B().Subscribe().Channel(channel).Build()).Error()
	}
	if err != nil {
		return err
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return errors.New("subscription lost due to slot migration")
	case err = <-wait:
		return err
	}
}
