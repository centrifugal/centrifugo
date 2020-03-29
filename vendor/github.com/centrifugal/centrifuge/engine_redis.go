package centrifuge

import (
	"bytes"
	"crypto/tls"
	"errors"
	"fmt"
	"hash/fnv"
	"net"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/centrifugal/centrifuge/internal/timers"

	"github.com/FZambia/sentinel"
	"github.com/gomodule/redigo/redis"
	"github.com/mna/redisc"
)

const (
	// redisPubSubWorkerChannelSize sets buffer size of channel to which we send all
	// messages received from Redis PUB/SUB connection to process in separate goroutine.
	redisPubSubWorkerChannelSize = 1024
	// redisSubscribeBatchLimit is a maximum number of channels to include in a single subscribe
	// call. Redis documentation doesn't specify a maximum allowed but we think it probably makes
	// sense to keep a sane limit given how many subscriptions a single Centrifugo instance might
	// be handling.
	redisSubscribeBatchLimit = 512
	// redisPublishBatchLimit is a maximum limit of publish requests one batched publish
	// operation can contain.
	redisPublishBatchLimit = 512
	// redisDataBatchLimit is a max amount of data requests in one batch.
	redisDataBatchLimit = 64
)

const (
	defaultPrefix         = "centrifuge"
	defaultReadTimeout    = time.Second
	defaultWriteTimeout   = time.Second
	defaultConnectTimeout = time.Second
	defaultPoolSize       = 128
)

type (
	// channelID is unique channel identificator in Redis.
	channelID string
)

var errRedisOpTimeout = errors.New("operation timed out")

const (
	// redisControlChannelSuffix is a suffix for control channel.
	redisControlChannelSuffix = ".control"
	// redisPingChannelSuffix is a suffix for ping channel.
	redisPingChannelSuffix = ".ping"
	// redisClientChannelPrefix is a prefix before channel name for client messages.
	redisClientChannelPrefix = ".client."
)

// RedisEngine uses Redis datastructures and PUB/SUB to manage Centrifugo logic.
// This engine allows to scale Centrifugo - you can run several Centrifugo instances
// connected to the same Redis and load balance clients between instances.
type RedisEngine struct {
	node     *Node
	sharding bool
	config   RedisEngineConfig
	shards   []*shard
}

var _ Engine = (*RedisEngine)(nil)

type redisConnPool interface {
	Get() redis.Conn
}

// shard has everything to connect to Redis instance.
type shard struct {
	node              *Node
	engine            *RedisEngine
	config            RedisShardConfig
	pool              redisConnPool
	subCh             chan subRequest
	pubCh             chan pubRequest
	dataCh            chan dataRequest
	addPresenceScript *redis.Script
	remPresenceScript *redis.Script
	presenceScript    *redis.Script
	historyScript     *redis.Script
	addHistoryScript  *redis.Script
	messagePrefix     string
}

// RedisEngineConfig is a config for Redis Engine.
type RedisEngineConfig struct {
	// PublishOnHistoryAdd is an option to control Redis Engine behaviour in terms of
	// adding to history and publishing message to channel. Redis Engine have a role
	// of Broker, HistoryManager and PresenceManager, this option is a tip to engine
	// implementation about the fact that Redis Engine used as both Broker and
	// HistoryManager. In this case we have a possibility to save Publications into
	// channel history stream and publish into PUB/SUB Redis channel atomically. And
	// we just do this reducing network round trips.
	PublishOnHistoryAdd bool

	// SequenceTTL sets a time of sequence meta key expiration in Redis. Sequence
	// meta key is a Redis HASH that contains top position in channel and epoch value.
	// By default sequence meta keys do not expire, though in some cases – when channels
	// created for а short time and then not used anymore – created sequence meta keys
	// stay in memory while not actually useful. Setting a reasonable value to this
	// option (usually much bigger than history retention period) can help.

	// SequenceTTL sets a time of sequence data expiration in Engine.
	// Sequence meta key in Redis is a HASH that contains current sequence number
	// in channel and epoch value. By default sequence data for channels does not expire.
	//
	// Though in some cases – when channels created for а short time and then
	// not used anymore – created sequence meta data can stay in memory while
	// not actually useful. For example you can have a personal user channel but
	// after using your app for a while user left it forever. In long-term
	// perspective this can be an unwanted memory leak. Setting a reasonable
	// value to this option (usually much bigger than history retention period)
	// can help. In this case unused channel sequence data will eventually expire.
	SequenceTTL time.Duration

	// Shards is a list of Redis instance configs.
	Shards []RedisShardConfig
}

// RedisShardConfig is struct with Redis Engine options.
type RedisShardConfig struct {
	// Host is Redis server host.
	Host string
	// Port is Redis server port.
	Port int
	// Password is password to use when connecting to Redis database. If empty then password not used.
	Password string
	// DB is Redis database number. If not set then database 0 used.
	DB int
	// Whether to use TLS connection or not.
	UseTLS bool
	// Whether to skip hostname verification as part of TLS handshake.
	TLSSkipVerify bool
	// Connection TLS configuration.
	TLSConfig *tls.Config
	// MasterName is a name of Redis instance master Sentinel monitors.
	MasterName string
	// SentinelAddrs is a slice of Sentinel addresses.
	SentinelAddrs []string
	// ClusterAddrs is a slice of seed cluster addrs for this shard.
	ClusterAddrs []string
	// Prefix to use before every channel name and key in Redis.
	Prefix string
	// IdleTimeout is timeout after which idle connections to Redis will be closed.
	IdleTimeout time.Duration
	// PubSubNumWorkers sets how many PUB/SUB message processing workers will be started.
	// By default we start runtime.NumCPU() workers.
	PubSubNumWorkers int
	// ReadTimeout is a timeout on read operations. Note that at moment it should be greater
	// than node ping publish interval in order to prevent timing out Pubsub connection's
	// Receive call.
	ReadTimeout time.Duration
	// WriteTimeout is a timeout on write operations.
	WriteTimeout time.Duration
	// ConnectTimeout is a timeout on connect operation.
	ConnectTimeout time.Duration
}

// subRequest is an internal request to subscribe or unsubscribe from one or more channels
type subRequest struct {
	channels  []channelID
	subscribe bool
	err       chan error
}

// newSubRequest creates a new request to subscribe or unsubscribe form a channel.
func newSubRequest(chIDs []channelID, subscribe bool) subRequest {
	return subRequest{
		channels:  chIDs,
		subscribe: subscribe,
		err:       make(chan error, 1),
	}
}

// done should only be called once for subRequest.
func (sr *subRequest) done(err error) {
	sr.err <- err
}

func (sr *subRequest) result() error {
	return <-sr.err
}

func makePoolFactory(s *shard, n *Node, conf RedisShardConfig) func(addr string, options ...redis.DialOption) (*redis.Pool, error) {
	password := conf.Password
	db := conf.DB

	useSentinel := conf.MasterName != "" && len(conf.SentinelAddrs) > 0

	var lastMu sync.Mutex
	var lastMaster string

	poolSize := defaultPoolSize

	maxIdle := 64
	if poolSize < maxIdle {
		maxIdle = poolSize
	}

	var sntnl *sentinel.Sentinel
	if useSentinel {
		sntnl = &sentinel.Sentinel{
			Addrs:      conf.SentinelAddrs,
			MasterName: conf.MasterName,
			Dial: func(addr string) (redis.Conn, error) {
				timeout := 300 * time.Millisecond
				opts := []redis.DialOption{
					redis.DialConnectTimeout(timeout),
					redis.DialReadTimeout(timeout),
					redis.DialWriteTimeout(timeout),
				}
				c, err := redis.Dial("tcp", addr, opts...)
				if err != nil {
					n.Log(NewLogEntry(LogLevelError, "error dialing to Sentinel", map[string]interface{}{"error": err.Error()}))
					return nil, err
				}
				return c, nil
			},
		}

		// Periodically discover new Sentinels.
		go func() {
			if err := sntnl.Discover(); err != nil {
				n.Log(NewLogEntry(LogLevelError, "error discover Sentinel", map[string]interface{}{"error": err.Error()}))
			}
			for {
				select {
				case <-time.After(30 * time.Second):
					if err := sntnl.Discover(); err != nil {
						n.Log(NewLogEntry(LogLevelError, "error discover Sentinel", map[string]interface{}{"error": err.Error()}))
					}
				}
			}
		}()
	}

	return func(serverAddr string, dialOpts ...redis.DialOption) (*redis.Pool, error) {
		pool := &redis.Pool{
			MaxIdle:     maxIdle,
			MaxActive:   poolSize,
			Wait:        true,
			IdleTimeout: conf.IdleTimeout,
			Dial: func() (redis.Conn, error) {
				var err error
				if useSentinel {
					serverAddr, err = sntnl.MasterAddr()
					if err != nil {
						return nil, err
					}
					lastMu.Lock()
					if serverAddr != lastMaster {
						n.Log(NewLogEntry(LogLevelInfo, "Redis master discovered", map[string]interface{}{"addr": serverAddr}))
						lastMaster = serverAddr
					}
					lastMu.Unlock()
				}

				c, err := redis.Dial("tcp", serverAddr, dialOpts...)
				if err != nil {
					n.Log(NewLogEntry(LogLevelError, "error dialing to Redis", map[string]interface{}{"error": err.Error()}))
					return nil, err
				}

				if password != "" {
					if _, err := c.Do("AUTH", password); err != nil {
						c.Close()
						n.Log(NewLogEntry(LogLevelError, "error auth in Redis", map[string]interface{}{"error": err.Error()}))
						return nil, err
					}
				}

				if db != 0 {
					if _, err := c.Do("SELECT", db); err != nil {
						c.Close()
						n.Log(NewLogEntry(LogLevelError, "error selecting Redis db", map[string]interface{}{"error": err.Error()}))
						return nil, err
					}
				}
				return c, nil
			},
			TestOnBorrow: func(c redis.Conn, t time.Time) error {
				if useSentinel {
					if !sentinel.TestRole(c, "master") {
						return errors.New("failed master role check")
					}
					return nil
				}
				if s.useCluster() {
					// No need in this optimization outside cluster
					// use case due to utilization of pipelining.
					if time.Since(t) < time.Second {
						return nil
					}
				}
				_, err := c.Do("PING")
				return err
			},
		}
		return pool, nil
	}
}

func getDialOpts(conf RedisShardConfig) []redis.DialOption {
	var readTimeout = defaultReadTimeout
	if conf.ReadTimeout != 0 {
		readTimeout = conf.ReadTimeout
	}
	var writeTimeout = defaultWriteTimeout
	if conf.WriteTimeout != 0 {
		writeTimeout = conf.WriteTimeout
	}
	var connectTimeout = defaultConnectTimeout
	if conf.ConnectTimeout != 0 {
		connectTimeout = conf.ConnectTimeout
	}

	dialOpts := []redis.DialOption{
		redis.DialConnectTimeout(connectTimeout),
		redis.DialReadTimeout(readTimeout),
		redis.DialWriteTimeout(writeTimeout),
	}
	if conf.UseTLS {
		dialOpts = append(dialOpts, redis.DialUseTLS(true))
		if conf.TLSConfig != nil {
			dialOpts = append(dialOpts, redis.DialTLSConfig(conf.TLSConfig))
		}
		if conf.TLSSkipVerify {
			dialOpts = append(dialOpts, redis.DialTLSSkipVerify(true))
		}
	}
	return dialOpts
}

func newPool(s *shard, n *Node, conf RedisShardConfig) (redisConnPool, error) {
	host := conf.Host
	port := conf.Port
	password := conf.Password
	db := conf.DB

	useSentinel := conf.MasterName != "" && len(conf.SentinelAddrs) > 0
	usingPassword := password != ""

	poolFactory := makePoolFactory(s, n, conf)

	if !s.useCluster() {
		serverAddr := net.JoinHostPort(host, strconv.Itoa(port))
		if !useSentinel {
			n.Log(NewLogEntry(LogLevelInfo, fmt.Sprintf("Redis: %s/%d, using password: %v", serverAddr, db, usingPassword)))
		} else {
			n.Log(NewLogEntry(LogLevelInfo, fmt.Sprintf("Redis: Sentinel for name: %s, db: %d, using password: %v", conf.MasterName, db, usingPassword)))
		}
		pool, _ := poolFactory(serverAddr, getDialOpts(conf)...)
		return pool, nil
	}
	// OK, we should work with cluster.
	n.Log(NewLogEntry(LogLevelInfo, fmt.Sprintf("Redis: cluster addrs: %+v, using password: %v", conf.ClusterAddrs, usingPassword)))
	cluster := &redisc.Cluster{
		DialOptions:  getDialOpts(conf),
		StartupNodes: conf.ClusterAddrs,
		CreatePool:   poolFactory,
	}
	// Initialize cluster mapping.
	if err := cluster.Refresh(); err != nil {
		return nil, err
	}
	return cluster, nil
}

// NewRedisEngine initializes Redis Engine.
func NewRedisEngine(n *Node, config RedisEngineConfig) (*RedisEngine, error) {

	var shards []*shard

	if len(config.Shards) == 0 {
		return nil, errors.New("no Redis shards provided in configuration")
	}

	if len(config.Shards) > 1 {
		n.Log(NewLogEntry(LogLevelInfo, fmt.Sprintf("Redis sharding enabled: %d shards", len(config.Shards))))
	}

	for _, conf := range config.Shards {
		prefix := conf.Prefix
		if prefix == "" {
			conf.Prefix = defaultPrefix
		}
		shard, err := newShard(n, conf)
		if err != nil {
			return nil, err
		}
		shards = append(shards, shard)
	}

	e := &RedisEngine{
		node:     n,
		shards:   shards,
		config:   config,
		sharding: len(shards) > 1,
	}
	return e, nil
}

var (
	// Add to history and optionally publish.
	// KEYS[1] - history list key
	// KEYS[2] - sequence meta hash key
	// ARGV[1] - message payload
	// ARGV[2] - history size ltrim right bound
	// ARGV[3] - history lifetime
	// ARGV[4] - channel to publish message to if needed
	// ARGV[5] - history meta key expiration time
	addHistorySource = `
local sequence = redis.call("hincrby", KEYS[2], "s", 1)
if ARGV[5] ~= '0' then
	redis.call("expire", KEYS[2], ARGV[5])
end
local payload = "__" .. sequence .. "__" .. ARGV[1]
redis.call("lpush", KEYS[1], payload)
redis.call("ltrim", KEYS[1], 0, ARGV[2])
redis.call("expire", KEYS[1], ARGV[3])
if ARGV[4] ~= '' then
	redis.call("publish", ARGV[4], payload)
end
return sequence
		`

	// Retrieve channel history information.
	// KEYS[1] - sequence meta hash key
	// KEYS[2] - history list key
	// ARGV[1] - include publications into response
	// ARGV[2] - publications list right bound
	// ARGV[3] - sequence key expiration time
	historySource = `
redis.replicate_commands()
local sequence = redis.call("hget", KEYS[1], "s")
if ARGV[3] ~= '0' and sequence ~= false then
	redis.call("expire", KEYS[1], ARGV[3])
end
local epoch
if redis.call('exists', KEYS[1]) ~= 0 then
  epoch = redis.call("hget", KEYS[1], "e")
else
  epoch = redis.call('time')[1]
  redis.call("hset", KEYS[1], "e", epoch)
end
local pubs = nil
if ARGV[1] ~= "0" then
	pubs = redis.call("lrange", KEYS[2], 0, ARGV[2])
end
return {sequence, epoch, pubs}
	`

	// Add/update client presence information.
	// KEYS[1] - presence set key
	// KEYS[2] - presence hash key
	// ARGV[1] - key expire seconds
	// ARGV[2] - expire at for set member
	// ARGV[3] - client ID
	// ARGV[4] - info payload
	addPresenceSource = `
redis.call("zadd", KEYS[1], ARGV[2], ARGV[3])
redis.call("hset", KEYS[2], ARGV[3], ARGV[4])
redis.call("expire", KEYS[1], ARGV[1])
redis.call("expire", KEYS[2], ARGV[1])
	`

	// Remove client presence.
	// KEYS[1] - presence set key
	// KEYS[2] - presence hash key
	// ARGV[1] - client ID
	remPresenceSource = `
redis.call("hdel", KEYS[2], ARGV[1])
redis.call("zrem", KEYS[1], ARGV[1])
	`

	// Get presence information.
	// KEYS[1] - presence set key
	// KEYS[2] - presence hash key
	// ARGV[1] - current timestamp in seconds
	presenceSource = `
local expired = redis.call("zrangebyscore", KEYS[1], "0", ARGV[1])
if #expired > 0 then
  for num = 1, #expired do
    redis.call("hdel", KEYS[2], expired[num])
  end
  redis.call("zremrangebyscore", KEYS[1], "0", ARGV[1])
end
return redis.call("hgetall", KEYS[2])
	`
)

func (e *RedisEngine) getShard(channel string) *shard {
	if !e.sharding {
		return e.shards[0]
	}
	return e.shards[consistentIndex(channel, len(e.shards))]
}

// Run runs engine after node initialized.
func (e *RedisEngine) Run(h BrokerEventHandler) error {
	for _, shard := range e.shards {
		shard.engine = e
		err := shard.Run(h)
		if err != nil {
			return err
		}
	}
	return nil
}

// Publish - see engine interface description.
func (e *RedisEngine) Publish(ch string, pub *Publication, opts *ChannelOptions) error {
	return e.getShard(ch).Publish(ch, pub, opts)
}

// PublishJoin - see engine interface description.
func (e *RedisEngine) PublishJoin(ch string, join *Join, opts *ChannelOptions) error {
	return e.getShard(ch).PublishJoin(ch, join, opts)
}

// PublishLeave - see engine interface description.
func (e *RedisEngine) PublishLeave(ch string, leave *Leave, opts *ChannelOptions) error {
	return e.getShard(ch).PublishLeave(ch, leave, opts)
}

// PublishControl - see engine interface description.
func (e *RedisEngine) PublishControl(data []byte) error {
	var err error
	for _, shard := range e.shards {
		err = shard.PublishControl(data)
		if err != nil {
			continue
		}
		return nil
	}
	return fmt.Errorf("publish control error, all shards failed: last error: %v", err)
}

// Subscribe - see engine interface description.
func (e *RedisEngine) Subscribe(ch string) error {
	return e.getShard(ch).Subscribe(ch)
}

// Unsubscribe - see engine interface description.
func (e *RedisEngine) Unsubscribe(ch string) error {
	return e.getShard(ch).Unsubscribe(ch)
}

// AddPresence - see engine interface description.
func (e *RedisEngine) AddPresence(ch string, uid string, info *ClientInfo, exp time.Duration) error {
	expire := int(exp.Seconds())
	return e.getShard(ch).AddPresence(ch, uid, info, expire)
}

// RemovePresence - see engine interface description.
func (e *RedisEngine) RemovePresence(ch string, uid string) error {
	return e.getShard(ch).RemovePresence(ch, uid)
}

// Presence - see engine interface description.
func (e *RedisEngine) Presence(ch string) (map[string]*ClientInfo, error) {
	return e.getShard(ch).Presence(ch)
}

// PresenceStats - see engine interface description.
func (e *RedisEngine) PresenceStats(ch string) (PresenceStats, error) {
	return e.getShard(ch).PresenceStats(ch)
}

// History - see engine interface description.
func (e *RedisEngine) History(ch string, filter HistoryFilter) ([]*Publication, RecoveryPosition, error) {
	return e.getShard(ch).History(ch, filter, e.config.SequenceTTL)
}

// AddHistory - see engine interface description.
func (e *RedisEngine) AddHistory(ch string, pub *Publication, opts *ChannelOptions) (*Publication, error) {
	return e.getShard(ch).AddHistory(ch, pub, opts, e.config.PublishOnHistoryAdd, e.config.SequenceTTL)
}

// RemoveHistory - see engine interface description.
func (e *RedisEngine) RemoveHistory(ch string) error {
	return e.getShard(ch).RemoveHistory(ch)
}

// Channels - see engine interface description.
func (e *RedisEngine) Channels() ([]string, error) {
	channelMap := map[string]struct{}{}
	for _, shard := range e.shards {
		chans, err := shard.Channels()
		if err != nil {
			return chans, err
		}
		if !e.sharding {
			// We have all channels on one shard.
			return chans, nil
		}
		for _, ch := range chans {
			channelMap[ch] = struct{}{}
		}
	}
	channels := make([]string, 0, len(channelMap))
	for ch := range channelMap {
		channels = append(channels, ch)
	}
	return channels, nil
}

// newShard initializes new Redis shard.
func newShard(n *Node, conf RedisShardConfig) (*shard, error) {
	shard := &shard{
		node:              n,
		config:            conf,
		addPresenceScript: redis.NewScript(2, addPresenceSource),
		remPresenceScript: redis.NewScript(2, remPresenceSource),
		presenceScript:    redis.NewScript(2, presenceSource),
		historyScript:     redis.NewScript(2, historySource),
		addHistoryScript:  redis.NewScript(2, addHistorySource),
	}
	pool, err := newPool(shard, n, conf)
	if err != nil {
		return nil, err
	}
	shard.pool = pool

	shard.subCh = make(chan subRequest)
	shard.pubCh = make(chan pubRequest)
	shard.dataCh = make(chan dataRequest)

	shard.messagePrefix = conf.Prefix + redisClientChannelPrefix

	if !shard.useCluster() {
		// Only need data pipeline in non-cluster scenario.
		go shard.runForever(func() {
			shard.runDataPipeline()
		})
	}

	return shard, nil
}

func (s *shard) useCluster() bool {
	return len(s.config.ClusterAddrs) != 0
}

func (s *shard) controlChannelID() channelID {
	return channelID(s.config.Prefix + redisControlChannelSuffix)
}

func (s *shard) pingChannelID() channelID {
	return channelID(s.config.Prefix + redisPingChannelSuffix)
}

func (s *shard) messageChannelID(ch string) channelID {
	return channelID(s.messagePrefix + ch)
}

func (s *shard) presenceHashKey(ch string) channelID {
	if s.useCluster() {
		ch = "{" + ch + "}"
	}
	return channelID(s.config.Prefix + ".presence.data." + ch)
}

func (s *shard) presenceSetKey(ch string) channelID {
	if s.useCluster() {
		ch = "{" + ch + "}"
	}
	return channelID(s.config.Prefix + ".presence.expire." + ch)
}

func (s *shard) historyListKey(ch string) channelID {
	if s.useCluster() {
		ch = "{" + ch + "}"
	}
	return channelID(s.config.Prefix + ".history.list." + ch)
}

func (s *shard) sequenceMetaKey(ch string) channelID {
	if s.useCluster() {
		ch = "{" + ch + "}"
	}
	return channelID(s.config.Prefix + ".seq.meta." + ch)
}

// Run Redis shard.
func (s *shard) Run(h BrokerEventHandler) error {
	go s.runForever(func() {
		s.runPublishPipeline()
	})
	go s.runForever(func() {
		s.runPubSubPing()
	})
	go s.runForever(func() {
		s.runPubSub(h)
	})
	return nil
}

func (s *shard) readTimeout() time.Duration {
	var readTimeout = defaultReadTimeout
	if s.config.ReadTimeout != 0 {
		readTimeout = s.config.ReadTimeout
	}
	return readTimeout
}

// runForever keeps another function running indefinitely.
// The reason this loop is not inside the function itself is
// so that defer can be used to cleanup nicely.
func (s *shard) runForever(fn func()) {
	for {
		fn()
		// Sleep for a while to prevent busy loop when reconnecting to Redis.
		time.Sleep(300 * time.Millisecond)
	}
}

func (s *shard) runPubSub(eventHandler BrokerEventHandler) {

	numWorkers := s.config.PubSubNumWorkers
	if numWorkers == 0 {
		numWorkers = runtime.NumCPU()
	}

	s.node.Log(NewLogEntry(LogLevelDebug, fmt.Sprintf("running Redis PUB/SUB, num workers: %d", numWorkers)))
	defer func() {
		s.node.Log(NewLogEntry(LogLevelDebug, "stopping Redis PUB/SUB"))
	}()

	poolConn := s.pool.Get()
	if poolConn.Err() != nil {
		// At this moment test on borrow could already return an error,
		// we can't work with broken connection.
		poolConn.Close()
		return
	}

	conn := redis.PubSubConn{Conn: poolConn}

	done := make(chan struct{})
	var doneOnce sync.Once
	closeDoneOnce := func() {
		doneOnce.Do(func() {
			close(done)
		})
	}
	defer closeDoneOnce()

	// Run subscriber goroutine.
	go func() {
		s.node.Log(NewLogEntry(LogLevelDebug, "starting RedisEngine Subscriber"))
		defer func() {
			s.node.Log(NewLogEntry(LogLevelDebug, "stopping RedisEngine Subscriber"))
		}()
		for {
			select {
			case <-done:
				conn.Close()
				return
			case r := <-s.subCh:
				isSubscribe := r.subscribe
				channelBatch := []subRequest{r}

				chIDs := make([]interface{}, 0, len(r.channels))
				for _, ch := range r.channels {
					chIDs = append(chIDs, ch)
				}

				var otherR *subRequest

			loop:
				for len(chIDs) < redisSubscribeBatchLimit {
					select {
					case r := <-s.subCh:
						if r.subscribe != isSubscribe {
							// We can not mix subscribe and unsubscribe request into one batch
							// so must stop here. As we consumed a subRequest value from channel
							// we should take care of it later.
							otherR = &r
							break loop
						}
						channelBatch = append(channelBatch, r)
						for _, ch := range r.channels {
							chIDs = append(chIDs, ch)
						}
					default:
						break loop
					}
				}

				var opErr error
				if isSubscribe {
					opErr = conn.Subscribe(chIDs...)
				} else {
					opErr = conn.Unsubscribe(chIDs...)
				}

				if opErr != nil {
					for _, r := range channelBatch {
						r.done(opErr)
					}
					if otherR != nil {
						otherR.done(opErr)
					}
					// Close conn, this should cause Receive to return with err below
					// and whole runPubSub method to restart.
					conn.Close()
					return
				}
				for _, r := range channelBatch {
					r.done(nil)
				}
				if otherR != nil {
					chIDs := make([]interface{}, 0, len(otherR.channels))
					for _, ch := range otherR.channels {
						chIDs = append(chIDs, ch)
					}
					var opErr error
					if otherR.subscribe {
						opErr = conn.Subscribe(chIDs...)
					} else {
						opErr = conn.Unsubscribe(chIDs...)
					}
					if opErr != nil {
						otherR.done(opErr)
						// Close conn, this should cause Receive to return with err below
						// and whole runPubSub method to restart.
						conn.Close()
						return
					}
					otherR.done(nil)
				}
			}
		}
	}()

	controlChannel := s.controlChannelID()
	pingChannel := s.pingChannelID()

	// Run workers to spread received message processing work over worker goroutines.
	workers := make(map[int]chan redis.Message)
	for i := 0; i < numWorkers; i++ {
		workerCh := make(chan redis.Message, redisPubSubWorkerChannelSize)
		workers[i] = workerCh
		go func(ch chan redis.Message) {
			for {
				select {
				case <-done:
					return
				case n := <-ch:
					chID := channelID(n.Channel)
					switch chID {
					case controlChannel:
						err := eventHandler.HandleControl(n.Data)
						if err != nil {
							s.node.Log(NewLogEntry(LogLevelError, "error handling control message", map[string]interface{}{"error": err.Error()}))
							continue
						}
					case pingChannel:
						// Do nothing - this message just maintains connection open.
					default:
						err := s.handleRedisClientMessage(eventHandler, chID, n.Data)
						if err != nil {
							s.node.Log(NewLogEntry(LogLevelError, "error handling client message", map[string]interface{}{"error": err.Error()}))
							continue
						}
					}
				}
			}
		}(workerCh)
	}

	go func() {
		chIDs := make([]channelID, 2)
		chIDs[0] = controlChannel
		chIDs[1] = pingChannel

		for _, ch := range s.node.Hub().Channels() {
			if s.engine.getShard(ch) == s {
				chIDs = append(chIDs, s.messageChannelID(ch))
			}
		}

		batch := make([]channelID, 0)

		for i, ch := range chIDs {
			if len(batch) > 0 && i%redisSubscribeBatchLimit == 0 {
				r := newSubRequest(batch, true)
				err := s.sendSubscribe(r)
				if err != nil {
					s.node.Log(NewLogEntry(LogLevelError, "error subscribing", map[string]interface{}{"error": err.Error()}))
					closeDoneOnce()
					return
				}
				batch = nil
			}
			batch = append(batch, ch)
		}
		if len(batch) > 0 {
			r := newSubRequest(batch, true)
			err := s.sendSubscribe(r)
			if err != nil {
				s.node.Log(NewLogEntry(LogLevelError, "error subscribing", map[string]interface{}{"error": err.Error()}))
				closeDoneOnce()
				return
			}
		}
	}()

	for {
		switch n := conn.ReceiveWithTimeout(10 * time.Second).(type) {
		case redis.Message:
			// Add message to worker channel preserving message order - i.e. messages
			// from the same channel will be processed in the same worker.
			workers[index(n.Channel, numWorkers)] <- n
		case redis.Subscription:
		case error:
			s.node.Log(NewLogEntry(LogLevelError, "Redis receiver error", map[string]interface{}{"error": n.Error()}))
			return
		}
	}
}

func (s *shard) handleRedisClientMessage(eventHandler BrokerEventHandler, _ channelID, data []byte) error {
	pushData, seq, gen := extractPushData(data)
	var push Push
	err := push.Unmarshal(pushData)
	if err != nil {
		return err
	}
	switch push.Type {
	case PushTypePublication:
		var pub Publication
		err := pub.Unmarshal(push.Data)
		if err != nil {
			return err
		}
		if seq > 0 || gen > 0 {
			pub.Seq = seq
			pub.Gen = gen
		}
		eventHandler.HandlePublication(push.Channel, &pub)
	case PushTypeJoin:
		var join Join
		err := join.Unmarshal(push.Data)
		if err != nil {
			return err
		}
		eventHandler.HandleJoin(push.Channel, &join)
	case PushTypeLeave:
		var leave Leave
		err := leave.Unmarshal(push.Data)
		if err != nil {
			return err
		}
		eventHandler.HandleLeave(push.Channel, &leave)
	default:
	}
	return nil
}

type pubRequest struct {
	channel channelID
	message []byte
	err     chan error
}

func (pr *pubRequest) done(err error) {
	pr.err <- err
}

func (pr *pubRequest) result() error {
	return <-pr.err
}

func (s *shard) runPubSubPing() {
	pingTicker := time.NewTicker(time.Second)
	defer pingTicker.Stop()
	for {
		select {
		case <-pingTicker.C:
			// Publish periodically to maintain PUB/SUB connection alive and allow
			// PUB/SUB connection to close early if no data received for a period of time.
			conn := s.pool.Get()
			err := conn.Send("PUBLISH", s.pingChannelID(), nil)
			if err != nil {
				s.node.Log(NewLogEntry(LogLevelError, "error publish ping to Redis channel", map[string]interface{}{"error": err.Error()}))
				conn.Close()
				return
			}
			conn.Close()
		}
	}
}

func (s *shard) runPublishPipeline() {
	var prs []pubRequest

	for {
		select {
		case pr := <-s.pubCh:
			prs = append(prs, pr)
		loop:
			for len(prs) < redisPublishBatchLimit {
				select {
				case pr := <-s.pubCh:
					prs = append(prs, pr)
				default:
					break loop
				}
			}
			conn := s.pool.Get()
			for i := range prs {
				conn.Send("PUBLISH", prs[i].channel, prs[i].message)
			}
			err := conn.Flush()
			if err != nil {
				for i := range prs {
					prs[i].done(err)
				}
				s.node.Log(NewLogEntry(LogLevelError, "error flushing publish pipeline", map[string]interface{}{"error": err.Error()}))
				conn.Close()
				return
			}
			for i := range prs {
				_, err := conn.Receive()
				prs[i].done(err)
			}
			if conn.Err() != nil {
				conn.Close()
				return
			}
			conn.Close()
			prs = nil
		}
	}
}

type dataOp int

const (
	dataOpAddPresence dataOp = iota
	dataOpRemovePresence
	dataOpPresence
	dataOpHistory
	dataOpAddHistory
	dataOpHistoryRemove
	dataOpChannels
)

type dataResponse struct {
	reply interface{}
	err   error
}

type dataRequest struct {
	op   dataOp
	args []interface{}
	resp chan *dataResponse
}

func newDataRequest(op dataOp, args []interface{}) dataRequest {
	return dataRequest{op: op, args: args, resp: make(chan *dataResponse, 1)}
}

func (dr *dataRequest) done(reply interface{}, err error) {
	if dr.resp == nil {
		return
	}
	dr.resp <- &dataResponse{reply: reply, err: err}
}

func (dr *dataRequest) result() *dataResponse {
	if dr.resp == nil {
		// No waiting, as caller didn't care about response.
		return &dataResponse{}
	}
	return <-dr.resp
}

func (s *shard) processClusterDataRequest(dr dataRequest) (interface{}, error) {
	conn := s.pool.Get()
	defer conn.Close()

	var err error

	var key string
	switch dr.op {
	case dataOpAddPresence, dataOpRemovePresence, dataOpPresence, dataOpHistory, dataOpAddHistory:
		key = fmt.Sprintf("%s", dr.args[0])
	default:
	}
	if key != "" {
		if c, ok := conn.(*redisc.Conn); ok {
			err := c.Bind(key)
			if err != nil {
				return nil, err
			}
		}
	}

	// Handle redirections automatically.
	conn, err = redisc.RetryConn(conn, 3, 50*time.Millisecond)
	if err != nil {
		return nil, err
	}

	var reply interface{}

	switch dr.op {
	case dataOpAddPresence:
		reply, err = s.addPresenceScript.Do(conn, dr.args...)
	case dataOpRemovePresence:
		reply, err = s.remPresenceScript.Do(conn, dr.args...)
	case dataOpPresence:
		reply, err = s.presenceScript.Do(conn, dr.args...)
	case dataOpHistory:
		reply, err = s.historyScript.Do(conn, dr.args...)
	case dataOpAddHistory:
		reply, err = s.addHistoryScript.Do(conn, dr.args...)
	case dataOpHistoryRemove:
		reply, err = conn.Do("DEL", dr.args...)
	case dataOpChannels:
		reply, err = conn.Do("PUBSUB", dr.args...)
	}
	return reply, err
}

func (s *shard) runDataPipeline() {
	conn := s.pool.Get()
	scripts := []*redis.Script{
		s.addPresenceScript,
		s.presenceScript,
		s.remPresenceScript,
		s.historyScript,
		s.addHistoryScript,
	}
	for _, script := range scripts {
		err := script.Load(conn)
		if err != nil {
			s.node.Log(NewLogEntry(LogLevelError, "error loading Lua script", map[string]interface{}{"error": err.Error()}))
			// Can not proceed if script has not been loaded.
			conn.Close()
			return
		}
	}
	conn.Close()

	var drs []dataRequest

	for dr := range s.dataCh {
		drs = append(drs, dr)
	loop:
		for len(drs) < redisDataBatchLimit {
			select {
			case req := <-s.dataCh:
				drs = append(drs, req)
			default:
				break loop
			}
		}

		conn := s.pool.Get()

		for i := range drs {
			switch drs[i].op {
			case dataOpAddPresence:
				s.addPresenceScript.SendHash(conn, drs[i].args...)
			case dataOpRemovePresence:
				s.remPresenceScript.SendHash(conn, drs[i].args...)
			case dataOpPresence:
				s.presenceScript.SendHash(conn, drs[i].args...)
			case dataOpHistory:
				s.historyScript.SendHash(conn, drs[i].args...)
			case dataOpAddHistory:
				s.addHistoryScript.SendHash(conn, drs[i].args...)
			case dataOpHistoryRemove:
				conn.Send("DEL", drs[i].args...)
			case dataOpChannels:
				conn.Send("PUBSUB", drs[i].args...)
			}
		}

		err := conn.Flush()
		if err != nil {
			for i := range drs {
				drs[i].done(nil, err)
			}
			s.node.Log(NewLogEntry(LogLevelError, "error flushing data pipeline", map[string]interface{}{"error": err.Error()}))
			conn.Close()
			return
		}
		var noScriptError bool
		for i := range drs {
			reply, err := conn.Receive()
			if err != nil {
				// Check for NOSCRIPT error. In normal circumstances this should never happen.
				// The only possible situation is when Redis scripts were flushed. In this case
				// we will return from this func and load publish script from scratch.
				// Redigo does the same check but for single EVALSHA command: see
				// https://github.com/garyburd/redigo/blob/master/redis/script.go#L64
				if e, ok := err.(redis.Error); ok && strings.HasPrefix(string(e), "NOSCRIPT ") {
					noScriptError = true
				}
			}
			drs[i].done(reply, err)
		}
		if conn.Err() != nil {
			conn.Close()
			return
		}
		if noScriptError {
			// Start this func from the beginning and LOAD missing script.
			conn.Close()
			return
		}
		conn.Close()
		drs = nil
	}
}

var (
	// ErrPublished returned to indicate that node should not publish message to broker.
	ErrPublished = errors.New("message published")
)

// Publish - see engine interface description.
func (s *shard) Publish(ch string, pub *Publication, _ *ChannelOptions) error {
	eChan := make(chan error, 1)

	data, err := pub.Marshal()
	if err != nil {
		return err
	}
	push := &Push{
		Type:    PushTypePublication,
		Channel: ch,
		Data:    data,
	}
	byteMessage, err := push.Marshal()
	if err != nil {
		return err
	}

	chID := s.messageChannelID(ch)

	pr := pubRequest{
		channel: chID,
		message: byteMessage,
		err:     eChan,
	}
	select {
	case s.pubCh <- pr:
	default:
		timer := timers.AcquireTimer(s.readTimeout())
		defer timers.ReleaseTimer(timer)
		select {
		case s.pubCh <- pr:
		case <-timer.C:
			return errRedisOpTimeout
		}
	}
	return <-eChan
}

// PublishJoin - see engine interface description.
func (s *shard) PublishJoin(ch string, join *Join, _ *ChannelOptions) error {

	eChan := make(chan error, 1)

	data, err := join.Marshal()
	if err != nil {
		return err
	}
	push := &Push{
		Type:    PushTypeJoin,
		Channel: ch,
		Data:    data,
	}
	byteMessage, err := push.Marshal()
	if err != nil {
		return err
	}

	chID := s.messageChannelID(ch)

	pr := pubRequest{
		channel: chID,
		message: byteMessage,
		err:     eChan,
	}
	select {
	case s.pubCh <- pr:
	default:
		timer := timers.AcquireTimer(s.readTimeout())
		defer timers.ReleaseTimer(timer)
		select {
		case s.pubCh <- pr:
		case <-timer.C:
			return errRedisOpTimeout
		}
	}
	return <-eChan
}

// PublishLeave - see engine interface description.
func (s *shard) PublishLeave(ch string, leave *Leave, _ *ChannelOptions) error {

	eChan := make(chan error, 1)

	data, err := leave.Marshal()
	if err != nil {
		return err
	}
	push := &Push{
		Type:    PushTypeLeave,
		Channel: ch,
		Data:    data,
	}
	byteMessage, err := push.Marshal()
	if err != nil {
		return err
	}

	chID := s.messageChannelID(ch)

	pr := pubRequest{
		channel: chID,
		message: byteMessage,
		err:     eChan,
	}
	select {
	case s.pubCh <- pr:
	default:
		timer := timers.AcquireTimer(s.readTimeout())
		defer timers.ReleaseTimer(timer)
		select {
		case s.pubCh <- pr:
		case <-timer.C:
			return errRedisOpTimeout
		}
	}
	return <-eChan
}

// PublishControl - see engine interface description.
func (s *shard) PublishControl(data []byte) error {
	eChan := make(chan error, 1)

	chID := s.controlChannelID()

	pr := pubRequest{
		channel: chID,
		message: data,
		err:     eChan,
	}
	select {
	case s.pubCh <- pr:
	default:
		timer := timers.AcquireTimer(s.readTimeout())
		defer timers.ReleaseTimer(timer)
		select {
		case s.pubCh <- pr:
		case <-timer.C:
			return errRedisOpTimeout
		}
	}
	return <-eChan
}

func (s *shard) sendSubscribe(r subRequest) error {
	select {
	case s.subCh <- r:
	default:
		timer := timers.AcquireTimer(s.readTimeout())
		defer timers.ReleaseTimer(timer)
		select {
		case s.subCh <- r:
		case <-timer.C:
			return errRedisOpTimeout
		}
	}
	return r.result()
}

// Subscribe - see engine interface description.
func (s *shard) Subscribe(ch string) error {
	if s.node.LogEnabled(LogLevelDebug) {
		s.node.Log(NewLogEntry(LogLevelDebug, "subscribe node on channel", map[string]interface{}{"channel": ch}))
	}
	r := newSubRequest([]channelID{s.messageChannelID(ch)}, true)
	return s.sendSubscribe(r)
}

// Unsubscribe - see engine interface description.
func (s *shard) Unsubscribe(ch string) error {
	if s.node.LogEnabled(LogLevelDebug) {
		s.node.Log(NewLogEntry(LogLevelDebug, "unsubscribe node from channel", map[string]interface{}{"channel": ch}))
	}
	r := newSubRequest([]channelID{s.messageChannelID(ch)}, false)
	return s.sendSubscribe(r)
}

func (s *shard) getDataResponse(r dataRequest) *dataResponse {
	if s.useCluster() {
		reply, err := s.processClusterDataRequest(r)
		return &dataResponse{
			reply: reply,
			err:   err,
		}
	}
	select {
	case s.dataCh <- r:
	default:
		timer := timers.AcquireTimer(s.readTimeout())
		defer timers.ReleaseTimer(timer)
		select {
		case s.dataCh <- r:
		case <-timer.C:
			return &dataResponse{nil, errRedisOpTimeout}
		}
	}
	return r.result()
}

// AddPresence - see engine interface description.
func (s *shard) AddPresence(ch string, uid string, info *ClientInfo, expire int) error {
	infoJSON, err := info.Marshal()
	if err != nil {
		return err
	}
	expireAt := time.Now().Unix() + int64(expire)
	hashKey := s.presenceHashKey(ch)
	setKey := s.presenceSetKey(ch)
	dr := newDataRequest(dataOpAddPresence, []interface{}{setKey, hashKey, expire, expireAt, uid, infoJSON})
	resp := s.getDataResponse(dr)
	return resp.err
}

// RemovePresence - see engine interface description.
func (s *shard) RemovePresence(ch string, uid string) error {
	hashKey := s.presenceHashKey(ch)
	setKey := s.presenceSetKey(ch)
	dr := newDataRequest(dataOpRemovePresence, []interface{}{setKey, hashKey, uid})
	resp := s.getDataResponse(dr)
	return resp.err
}

// Presence - see engine interface description.
func (s *shard) Presence(ch string) (map[string]*ClientInfo, error) {
	hashKey := s.presenceHashKey(ch)
	setKey := s.presenceSetKey(ch)
	now := int(time.Now().Unix())
	dr := newDataRequest(dataOpPresence, []interface{}{setKey, hashKey, now})
	resp := s.getDataResponse(dr)
	if resp.err != nil {
		return nil, resp.err
	}
	return mapStringClientInfo(resp.reply, nil)
}

// Presence - see engine interface description.
func (s *shard) PresenceStats(ch string) (PresenceStats, error) {
	presence, err := s.Presence(ch)
	if err != nil {
		return PresenceStats{}, err
	}

	numClients := len(presence)
	numUsers := 0
	uniqueUsers := map[string]struct{}{}

	for _, info := range presence {
		userID := info.User
		if _, ok := uniqueUsers[userID]; !ok {
			uniqueUsers[userID] = struct{}{}
			numUsers++
		}
	}

	return PresenceStats{
		NumClients: numClients,
		NumUsers:   numUsers,
	}, nil
}

// History - see engine interface description.
func (s *shard) History(ch string, filter HistoryFilter, seqTTL time.Duration) ([]*Publication, RecoveryPosition, error) {
	seqMetaKey := s.sequenceMetaKey(ch)
	historyKey := s.historyListKey(ch)

	var includePubs = true
	var rightBound = -1
	if filter.Limit == 0 {
		rightBound = 0
		includePubs = false
	}

	seqKeyTTLSeconds := int(seqTTL.Seconds())

	dr := newDataRequest(dataOpHistory, []interface{}{seqMetaKey, historyKey, includePubs, rightBound, seqKeyTTLSeconds})
	resp := s.getDataResponse(dr)
	if resp.err != nil {
		return nil, RecoveryPosition{}, resp.err
	}
	results := resp.reply.([]interface{})

	sequence, err := redis.Int64(results[0], nil)
	if err != nil {
		if err != redis.ErrNil {
			return nil, RecoveryPosition{}, err
		}
		sequence = 0
	}

	seq, gen := unpackUint64(uint64(sequence))

	var epoch string
	epoch, err = redis.String(results[1], nil)
	if err != nil {
		if err != redis.ErrNil {
			return nil, RecoveryPosition{}, err
		}
		epoch = ""
	}

	var publications []*Publication
	if includePubs {
		publications, err = sliceOfPubs(results[2], nil)
		if err != nil {
			return nil, RecoveryPosition{}, err
		}
	}

	latestPosition := RecoveryPosition{Seq: seq, Gen: gen, Epoch: epoch}

	since := filter.Since
	if since == nil {
		if filter.Limit >= 0 && len(publications) >= filter.Limit {
			return publications[:filter.Limit], latestPosition, nil
		}
		return publications, latestPosition, nil
	}

	if latestPosition.Seq == since.Seq && since.Gen == latestPosition.Gen && since.Epoch == latestPosition.Epoch {
		return nil, latestPosition, nil
	}

	nextSeq, nextGen := nextSeqGen(since.Seq, since.Gen)

	position := -1

	for i := 0; i < len(publications); i++ {
		pub := publications[i]
		if pub.Seq == since.Seq && pub.Gen == since.Gen {
			position = i + 1
			break
		}
		if pub.Seq == nextSeq && pub.Gen == nextGen {
			position = i
			break
		}
	}

	if position > -1 {
		pubs := publications[position:]
		if filter.Limit >= 0 {
			return pubs[:filter.Limit], latestPosition, nil
		}
		return pubs, latestPosition, nil
	}

	if filter.Limit >= 0 {
		return publications[:filter.Limit], latestPosition, nil
	}
	return publications, latestPosition, nil
}

func (s *shard) AddHistory(ch string, pub *Publication, opts *ChannelOptions, publishOnHistoryAdd bool, seqTTL time.Duration) (*Publication, error) {
	data, err := pub.Marshal()
	if err != nil {
		return nil, err
	}
	push := &Push{
		Type:    PushTypePublication,
		Channel: ch,
		Data:    data,
	}
	byteMessage, err := push.Marshal()
	if err != nil {
		return nil, err
	}

	var publishChannel channelID
	if publishOnHistoryAdd {
		publishChannel = s.messageChannelID(ch)
	}

	historyKey := s.historyListKey(ch)
	seqMetaKey := s.sequenceMetaKey(ch)
	seqKeyTTLSeconds := int(seqTTL.Seconds())
	dr := newDataRequest(dataOpAddHistory, []interface{}{historyKey, seqMetaKey, byteMessage, opts.HistorySize - 1, opts.HistoryLifetime, publishChannel, seqKeyTTLSeconds})
	resp := s.getDataResponse(dr)
	if resp.err != nil {
		return nil, resp.err
	}

	if publishOnHistoryAdd {
		return nil, nil
	}

	index, err := redis.Int64(resp.reply, nil)
	if err != nil {
		return nil, resp.err
	}
	seq, gen := unpackUint64(uint64(index))
	pub.Seq = seq
	pub.Gen = gen
	return pub, nil
}

// RemoveHistory - see engine interface description.
func (s *shard) RemoveHistory(ch string) error {
	historyKey := s.historyListKey(ch)
	dr := newDataRequest(dataOpHistoryRemove, []interface{}{historyKey})
	resp := s.getDataResponse(dr)
	return resp.err
}

// Channels - see engine interface description.
// Requires Redis >= 2.8.0 (http://redis.io/commands/pubsub)
func (s *shard) Channels() ([]string, error) {
	if s.useCluster() {
		return nil, errors.New("channels command not supported when Redis Cluster is used")
	}
	dr := newDataRequest(dataOpChannels, []interface{}{"CHANNELS", s.messagePrefix + "*"})
	resp := s.getDataResponse(dr)
	if resp.err != nil {
		return nil, resp.err
	}
	values, err := redis.Values(resp.reply, nil)
	if err != nil {
		return nil, err
	}
	channels := make([]string, 0, len(values))
	for i := 0; i < len(values); i++ {
		value, okValue := values[i].([]byte)
		if !okValue {
			return nil, errors.New("error getting channelID value")
		}
		chID := channelID(value)
		channels = append(channels, string(chID)[len(s.messagePrefix):])
	}
	return channels, nil
}

func mapStringClientInfo(result interface{}, err error) (map[string]*ClientInfo, error) {
	values, err := redis.Values(result, err)
	if err != nil {
		return nil, err
	}
	if len(values)%2 != 0 {
		return nil, errors.New("mapStringClientInfo expects even number of values result")
	}
	m := make(map[string]*ClientInfo, len(values)/2)
	for i := 0; i < len(values); i += 2 {
		key, okKey := values[i].([]byte)
		value, okValue := values[i+1].([]byte)
		if !okKey || !okValue {
			return nil, errors.New("scanMap key not a bulk string value")
		}
		var f ClientInfo
		err = f.Unmarshal(value)
		if err != nil {
			return nil, errors.New("can not unmarshal value to ClientInfo")
		}
		m[string(key)] = &f
	}
	return m, nil
}

func extractPushData(data []byte) ([]byte, uint32, uint32) {
	var seq, gen uint32
	if bytes.HasPrefix(data, []byte("__")) {
		parts := bytes.SplitN(data, []byte("__"), 3)
		sequence, _ := strconv.ParseUint(string(parts[1]), 10, 64)
		seq, gen = unpackUint64(sequence)
		return parts[2], seq, gen
	}
	return data, seq, gen
}

func sliceOfPubs(result interface{}, err error) ([]*Publication, error) {
	values, err := redis.Values(result, err)
	if err != nil {
		return nil, err
	}
	pubs := make([]*Publication, len(values))

	j := 0
	for i := len(values) - 1; i >= 0; i-- {
		value, okValue := values[i].([]byte)
		if !okValue {
			return nil, errors.New("error getting Message value")
		}

		pushData, seq, gen := extractPushData(value)

		var push Push
		err := push.Unmarshal(pushData)
		if err != nil {
			return nil, fmt.Errorf("can not unmarshal value to Message: %v", err)
		}

		if push.Type != PushTypePublication {
			return nil, fmt.Errorf("wrong message type in history: %d", push.Type)
		}

		var pub Publication
		err = pub.Unmarshal(push.Data)
		if err != nil {
			return nil, fmt.Errorf("can not unmarshal value to Pub: %v", err)
		}

		pub.Seq = seq
		pub.Gen = gen
		pubs[j] = &pub
		j++
	}
	return pubs, nil
}

// consistentIndex is an adapted function from https://github.com/dgryski/go-jump
// package by Damian Gryski. It consistently chooses a hash bucket number in the
// range [0, numBuckets) for the given string. numBuckets must be >= 1.
func consistentIndex(s string, numBuckets int) int {

	hash := fnv.New64a()
	hash.Write([]byte(s))
	key := hash.Sum64()

	var b int64 = -1
	var j int64

	for j < int64(numBuckets) {
		b = j
		key = key*2862933555777941757 + 1
		j = int64(float64(b+1) * (float64(int64(1)<<31) / float64((key>>33)+1)))
	}

	return int(b)
}
