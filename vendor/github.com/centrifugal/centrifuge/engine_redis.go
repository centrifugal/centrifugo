package centrifuge

import (
	"bytes"
	"context"
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

	"github.com/centrifugal/centrifuge/internal/proto"
	"github.com/centrifugal/centrifuge/internal/timers"

	"github.com/FZambia/sentinel"
	"github.com/gomodule/redigo/redis"
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
	defaultPoolSize       = 256
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
	shards   []*shard
}

// shard has everything to connect to Redis instance.
type shard struct {
	node              *Node
	engine            *RedisEngine
	eventHandler      EngineEventHandler
	config            RedisShardConfig
	pool              *redis.Pool
	subCh             chan subRequest
	pubCh             chan pubRequest
	dataCh            chan dataRequest
	pubScript         *redis.Script
	addPresenceScript *redis.Script
	remPresenceScript *redis.Script
	presenceScript    *redis.Script
	lpopManyScript    *redis.Script
	historySeqScript  *redis.Script
	messagePrefix     string

	pushEncoder proto.PushEncoder
	pushDecoder proto.PushDecoder
}

// RedisEngineConfig of Redis Engine.
type RedisEngineConfig struct {
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

func newPool(n *Node, conf RedisShardConfig) *redis.Pool {

	host := conf.Host
	port := conf.Port
	password := conf.Password
	db := conf.DB

	serverAddr := net.JoinHostPort(host, strconv.Itoa(port))
	useSentinel := conf.MasterName != "" && len(conf.SentinelAddrs) > 0

	usingPassword := password != ""
	if !useSentinel {
		n.logger.log(newLogEntry(LogLevelInfo, fmt.Sprintf("Redis: %s/%d, using password: %v", serverAddr, db, usingPassword)))
	} else {
		n.logger.log(newLogEntry(LogLevelInfo, fmt.Sprintf("Redis: Sentinel for name: %s, db: %d, using password: %v", conf.MasterName, db, usingPassword)))
	}

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
					n.logger.log(newLogEntry(LogLevelError, "error dialing to Sentinel", map[string]interface{}{"error": err.Error()}))
					return nil, err
				}
				return c, nil
			},
		}

		// Periodically discover new Sentinels.
		go func() {
			if err := sntnl.Discover(); err != nil {
				n.logger.log(newLogEntry(LogLevelError, "error discover Sentinel", map[string]interface{}{"error": err.Error()}))
			}
			for {
				select {
				case <-time.After(30 * time.Second):
					if err := sntnl.Discover(); err != nil {
						n.logger.log(newLogEntry(LogLevelError, "error discover Sentinel", map[string]interface{}{"error": err.Error()}))
					}
				}
			}
		}()
	}

	return &redis.Pool{
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
					n.logger.log(newLogEntry(LogLevelInfo, "Redis master discovered", map[string]interface{}{"addr": serverAddr}))
					lastMaster = serverAddr
				}
				lastMu.Unlock()
			}

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

			opts := []redis.DialOption{
				redis.DialConnectTimeout(connectTimeout),
				redis.DialReadTimeout(readTimeout),
				redis.DialWriteTimeout(writeTimeout),
			}
			if conf.UseTLS {
				opts = append(opts, redis.DialUseTLS(true))
				if conf.TLSConfig != nil {
					opts = append(opts, redis.DialTLSConfig(conf.TLSConfig))
				}
				if conf.TLSSkipVerify {
					opts = append(opts, redis.DialTLSSkipVerify(true))
				}
			}
			c, err := redis.Dial("tcp", serverAddr, opts...)
			if err != nil {
				n.logger.log(newLogEntry(LogLevelError, "error dialing to Redis", map[string]interface{}{"error": err.Error()}))
				return nil, err
			}

			if password != "" {
				if _, err := c.Do("AUTH", password); err != nil {
					c.Close()
					n.logger.log(newLogEntry(LogLevelError, "error auth in Redis", map[string]interface{}{"error": err.Error()}))
					return nil, err
				}
			}

			if db != 0 {
				if _, err := c.Do("SELECT", db); err != nil {
					c.Close()
					n.logger.log(newLogEntry(LogLevelError, "error selecting Redis db", map[string]interface{}{"error": err.Error()}))
					return nil, err
				}
			}

			return c, err
		},
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			if useSentinel {
				if !sentinel.TestRole(c, "master") {
					return errors.New("failed master role check")
				}
				return nil
			}
			_, err := c.Do("PING")
			return err
		},
	}
}

// NewRedisEngine initializes Redis Engine.
func NewRedisEngine(n *Node, config RedisEngineConfig) (*RedisEngine, error) {

	var shards []*shard

	if len(config.Shards) == 0 {
		return nil, errors.New("no Redis shards provided in configuration")
	}

	if len(config.Shards) > 1 {
		n.logger.log(newLogEntry(LogLevelInfo, fmt.Sprintf("Redis sharding enabled: %d shards", len(config.Shards))))
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
		sharding: len(shards) > 1,
	}
	return e, nil
}

var (
	// pubScriptSource contains lua script we register in Redis to call when publishing
	// client message. It publishes message into channel and adds message to history
	// list maintaining history size and expiration time. This is an optimization to make
	// 1 round trip to Redis instead of 2.
	// KEYS[1] - history list key
	// KEYS[2] - history sequence key
	// ARGV[1] - channel to publish message to
	// ARGV[2] - message payload
	// ARGV[3] - history size ltrim right bound
	// ARGV[4] - history lifetime
	pubScriptSource = `
local sequence = redis.call("incr", KEYS[2])
local payload = "__" .. sequence .. "__" .. ARGV[2]
redis.call("lpush", KEYS[1], payload)
redis.call("ltrim", KEYS[1], 0, ARGV[3])
redis.call("expire", KEYS[1], ARGV[4])
return redis.call("publish", ARGV[1], payload)
	`

	// KEYS[1] - presence set key
	// KEYS[2] - presence hash key
	// ARGV[1] - key expire seconds
	// ARGV[2] - expire at for set member
	// ARGV[3] - uid
	// ARGV[4] - info payload
	addPresenceSource = `
redis.call("zadd", KEYS[1], ARGV[2], ARGV[3])
redis.call("hset", KEYS[2], ARGV[3], ARGV[4])
redis.call("expire", KEYS[1], ARGV[1])
redis.call("expire", KEYS[2], ARGV[1])
	`

	// KEYS[1] - presence set key
	// KEYS[2] - presence hash key
	// ARGV[1] - uid
	remPresenceSource = `
redis.call("hdel", KEYS[2], ARGV[1])
redis.call("zrem", KEYS[1], ARGV[1])
	`

	// KEYS[1] - presence set key
	// KEYS[2] - presence hash key
	// ARGV[1] - now string
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

	// KEYS[1] - API list (queue) key
	// ARGV[1] - maximum amount of items to get
	lpopManySource = `
local entries = redis.call("lrange", KEYS[1], "0", ARGV[1])
if #entries > 0 then
  redis.call("ltrim", KEYS[1], #entries, -1)
end
return entries
	`

	// KEYS[1] - history sequence key
	// KEYS[2] - history gen key
	historySeqSource = `
redis.replicate_commands()
local seq = redis.call("get", KEYS[1])
local gen
if redis.call('EXISTS', KEYS[2]) ~= 0 then
  gen = redis.call("get", KEYS[2])
else
  gen = redis.call('TIME')[1]
  redis.call("set", KEYS[2], gen)
end
return {seq, gen}
	`
)

func (e *RedisEngine) getShard(channel string) *shard {
	if !e.sharding {
		return e.shards[0]
	}
	return e.shards[consistentIndex(channel, len(e.shards))]
}

// Name returns name of engine.
func (e *RedisEngine) name() string {
	return "Redis"
}

// Run runs engine after node initialized.
func (e *RedisEngine) run(h EngineEventHandler) error {
	for _, shard := range e.shards {
		shard.engine = e
		err := shard.Run(h)
		if err != nil {
			return err
		}
	}
	return nil
}

func (e *RedisEngine) shutdown(ctx context.Context) error {
	return nil
}

// Publish - see engine interface description.
func (e *RedisEngine) publish(ch string, pub *Publication, opts *ChannelOptions) <-chan error {
	return e.getShard(ch).Publish(ch, pub, opts)
}

// PublishJoin - see engine interface description.
func (e *RedisEngine) publishJoin(ch string, join *Join, opts *ChannelOptions) <-chan error {
	return e.getShard(ch).PublishJoin(ch, join, opts)
}

// PublishLeave - see engine interface description.
func (e *RedisEngine) publishLeave(ch string, leave *Leave, opts *ChannelOptions) <-chan error {
	return e.getShard(ch).PublishLeave(ch, leave, opts)
}

// PublishControl - see engine interface description.
func (e *RedisEngine) publishControl(data []byte) <-chan error {
	var err error
	for _, shard := range e.shards {
		err = <-shard.PublishControl(data)
		if err != nil {
			continue
		}
		errCh := make(chan error, 1)
		errCh <- nil
		return errCh
	}
	errCh := make(chan error, 1)
	errCh <- fmt.Errorf("publish control error, all shards failed: last error: %v", err)
	return errCh
}

// Subscribe - see engine interface description.
func (e *RedisEngine) subscribe(ch string) error {
	return e.getShard(ch).Subscribe(ch)
}

// Unsubscribe - see engine interface description.
func (e *RedisEngine) unsubscribe(ch string) error {
	return e.getShard(ch).Unsubscribe(ch)
}

// AddPresence - see engine interface description.
func (e *RedisEngine) addPresence(ch string, uid string, info *ClientInfo, exp time.Duration) error {
	expire := int(exp.Seconds())
	return e.getShard(ch).AddPresence(ch, uid, info, expire)
}

// RemovePresence - see engine interface description.
func (e *RedisEngine) removePresence(ch string, uid string) error {
	return e.getShard(ch).RemovePresence(ch, uid)
}

// Presence - see engine interface description.
func (e *RedisEngine) presence(ch string) (map[string]*ClientInfo, error) {
	return e.getShard(ch).Presence(ch)
}

// PresenceStats - see engine interface description.
func (e *RedisEngine) presenceStats(ch string) (PresenceStats, error) {
	return e.getShard(ch).PresenceStats(ch)
}

// History - see engine interface description.
func (e *RedisEngine) history(ch string, limit int) ([]*Publication, error) {
	return e.getShard(ch).History(ch, limit)
}

// RecoverHistory - see engine interface description.
func (e *RedisEngine) recoverHistory(ch string, since *recovery) ([]*Publication, bool, recovery, error) {
	return e.getShard(ch).RecoverHistory(ch, since)
}

// RemoveHistory - see engine interface description.
func (e *RedisEngine) removeHistory(ch string) error {
	return e.getShard(ch).RemoveHistory(ch)
}

// Channels - see engine interface description.
func (e *RedisEngine) channels() ([]string, error) {
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
	channels := make([]string, len(channelMap))
	j := 0
	for ch := range channelMap {
		channels[j] = ch
		j++
	}
	return channels, nil
}

// newShard initializes new Redis shard.
func newShard(n *Node, conf RedisShardConfig) (*shard, error) {
	shard := &shard{
		node:              n,
		config:            conf,
		pool:              newPool(n, conf),
		pubScript:         redis.NewScript(2, pubScriptSource),
		addPresenceScript: redis.NewScript(2, addPresenceSource),
		remPresenceScript: redis.NewScript(2, remPresenceSource),
		presenceScript:    redis.NewScript(2, presenceSource),
		lpopManyScript:    redis.NewScript(1, lpopManySource),
		historySeqScript:  redis.NewScript(2, historySeqSource),
		pushEncoder:       proto.NewProtobufPushEncoder(),
		pushDecoder:       proto.NewProtobufPushDecoder(),
	}
	shard.pubCh = make(chan pubRequest)
	shard.subCh = make(chan subRequest)
	shard.dataCh = make(chan dataRequest)
	shard.messagePrefix = conf.Prefix + redisClientChannelPrefix
	return shard, nil
}

func (s *shard) messageChannelID(ch string) channelID {
	return channelID(s.messagePrefix + ch)
}

func (s *shard) controlChannelID() channelID {
	return channelID(s.config.Prefix + redisControlChannelSuffix)
}

func (s *shard) pingChannelID() channelID {
	return channelID(s.config.Prefix + redisPingChannelSuffix)
}

func (s *shard) getPresenceHashKey(ch string) channelID {
	return channelID(s.config.Prefix + ".presence.data." + ch)
}

func (s *shard) getPresenceSetKey(ch string) channelID {
	return channelID(s.config.Prefix + ".presence.expire." + ch)
}

func (s *shard) getHistoryKey(ch string) channelID {
	return channelID(s.config.Prefix + ".history.list." + ch)
}

func (s *shard) gethistorySeqKey(ch string) channelID {
	return channelID(s.config.Prefix + ".history.seq." + ch)
}

func (s *shard) gethistoryEpochKey(ch string) channelID {
	return channelID(s.config.Prefix + ".history.epoch." + ch)
}

// Run runs Redis shard.
func (s *shard) Run(h EngineEventHandler) error {
	s.eventHandler = h
	go s.runForever(func() {
		s.runPublishPipeline()
	})
	go s.runForever(func() {
		s.runPubSub()
	})
	go s.runForever(func() {
		s.runDataPipeline()
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

func (s *shard) runPubSub() {

	numWorkers := s.config.PubSubNumWorkers
	if numWorkers == 0 {
		numWorkers = runtime.NumCPU()
	}

	s.node.logger.log(newLogEntry(LogLevelDebug, fmt.Sprintf("running Redis PUB/SUB, num workers: %d", numWorkers)))
	defer func() {
		s.node.logger.log(newLogEntry(LogLevelDebug, "stopping Redis PUB/SUB"))
	}()

	poolConn := s.pool.Get()
	if poolConn.Err() != nil {
		// At this moment test on borrow could already return an error,
		// we can't work with broken connection.
		poolConn.Close()
		return
	}

	conn := redis.PubSubConn{Conn: poolConn}
	defer conn.Close()

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
		s.node.logger.log(newLogEntry(LogLevelDebug, "starting RedisEngine Subscriber"))
		defer func() {
			s.node.logger.log(newLogEntry(LogLevelDebug, "stopping RedisEngine Subscriber"))
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
					if len(n.Data) == 0 {
						continue
					}
					switch chID {
					case controlChannel:
						err := s.eventHandler.HandleControl(n.Data)
						if err != nil {
							s.node.logger.log(newLogEntry(LogLevelError, "error handling control message", map[string]interface{}{"error": err.Error()}))
							continue
						}
					case pingChannel:
						// Do nothing - this message just maintains connection open.
					default:
						err := s.handleRedisClientMessage(chID, n.Data)
						if err != nil {
							s.node.logger.log(newLogEntry(LogLevelError, "error handling client message", map[string]interface{}{"error": err.Error()}))
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

		for _, ch := range s.node.hub.Channels() {
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
					s.node.logger.log(newLogEntry(LogLevelError, "error subscribing", map[string]interface{}{"error": err.Error()}))
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
				s.node.logger.log(newLogEntry(LogLevelError, "error subscribing", map[string]interface{}{"error": err.Error()}))
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
			s.node.logger.log(newLogEntry(LogLevelError, "Redis receiver error", map[string]interface{}{"error": n.Error()}))
			return
		}
	}
}

func (s *shard) handleRedisClientMessage(chID channelID, data []byte) error {
	pushData, seq, gen := extractPushData(data)
	var push proto.Push
	err := push.Unmarshal(pushData)
	if err != nil {
		return err
	}
	switch push.Type {
	case proto.PushTypePublication:
		pub, err := s.pushDecoder.DecodePublication(push.Data)
		if err != nil {
			return err
		}
		pub.Seq = seq
		pub.Gen = gen
		s.eventHandler.HandlePublication(push.Channel, pub)
	case proto.PushTypeJoin:
		join, err := s.pushDecoder.DecodeJoin(push.Data)
		if err != nil {
			return err
		}
		s.eventHandler.HandleJoin(push.Channel, join)
	case proto.PushTypeLeave:
		leave, err := s.pushDecoder.DecodeLeave(push.Data)
		if err != nil {
			return err
		}
		s.eventHandler.HandleLeave(push.Channel, leave)
	default:
	}
	return nil
}

type pubRequest struct {
	channel    channelID
	message    []byte
	historyKey channelID
	indexKey   channelID
	opts       *ChannelOptions
	err        chan error
}

func (pr *pubRequest) done(err error) {
	pr.err <- err
}

func (pr *pubRequest) result() error {
	return <-pr.err
}

func (s *shard) runPublishPipeline() {
	conn := s.pool.Get()

	err := s.pubScript.Load(conn)
	if err != nil {
		s.node.logger.log(newLogEntry(LogLevelError, "error loading publish Lua script", map[string]interface{}{"error": err.Error()}))
		// Can not proceed if script has not been loaded - because we use EVALSHA command for
		// publishing with history.
		conn.Close()
		return
	}

	conn.Close()

	var prs []pubRequest

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
				s.node.logger.log(newLogEntry(LogLevelError, "error publish ping to Redis channel", map[string]interface{}{"error": err.Error()}))
				conn.Close()
				return
			}
			conn.Close()
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
				if prs[i].opts != nil && prs[i].opts.HistorySize > 0 && prs[i].opts.HistoryLifetime > 0 {
					s.pubScript.SendHash(conn, prs[i].historyKey, prs[i].indexKey, prs[i].channel, prs[i].message, prs[i].opts.HistorySize-1, prs[i].opts.HistoryLifetime)
				} else {
					conn.Send("PUBLISH", prs[i].channel, prs[i].message)
				}
			}
			err := conn.Flush()
			if err != nil {
				for i := range prs {
					prs[i].done(err)
				}
				s.node.logger.log(newLogEntry(LogLevelError, "error flushing publish pipeline", map[string]interface{}{"error": err.Error()}))
				conn.Close()
				return
			}
			var noScriptError bool
			for i := range prs {
				_, err := conn.Receive()
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
				prs[i].done(err)
			}
			if noScriptError {
				// Start this func from the beginning and LOAD missing script.
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
	dataOphistorySeq
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

func (s *shard) runDataPipeline() {

	conn := s.pool.Get()

	err := s.addPresenceScript.Load(conn)
	if err != nil {
		s.node.logger.log(newLogEntry(LogLevelError, "error loading add presence Lua", map[string]interface{}{"error": err.Error()}))
		// Can not proceed if script has not been loaded.
		conn.Close()
		return
	}

	err = s.presenceScript.Load(conn)
	if err != nil {
		s.node.logger.log(newLogEntry(LogLevelError, "error loading presence Lua", map[string]interface{}{"error": err.Error()}))
		// Can not proceed if script has not been loaded.
		conn.Close()
		return
	}

	err = s.remPresenceScript.Load(conn)
	if err != nil {
		s.node.logger.log(newLogEntry(LogLevelError, "error loading remove presence Lua", map[string]interface{}{"error": err.Error()}))
		// Can not proceed if script has not been loaded.
		conn.Close()
		return
	}

	err = s.historySeqScript.Load(conn)
	if err != nil {
		s.node.logger.log(newLogEntry(LogLevelError, "error loading history seq Lua", map[string]interface{}{"error": err.Error()}))
		// Can not proceed if script has not been loaded.
		conn.Close()
		return
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
				conn.Send("LRANGE", drs[i].args...)
			case dataOphistorySeq:
				s.historySeqScript.SendHash(conn, drs[i].args...)
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
			s.node.logger.log(newLogEntry(LogLevelError, "error flushing data pipeline", map[string]interface{}{"error": err.Error()}))
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
		if noScriptError {
			// Start this func from the beginning and LOAD missing script.
			conn.Close()
			return
		}
		conn.Close()
		drs = nil
	}
}

// Publish - see engine interface description.
func (s *shard) Publish(ch string, pub *Publication, opts *ChannelOptions) <-chan error {

	eChan := make(chan error, 1)

	data, err := s.pushEncoder.EncodePublication(pub)
	if err != nil {
		eChan <- err
		return eChan
	}
	byteMessage, err := s.pushEncoder.Encode(proto.NewPublicationPush(ch, data))
	if err != nil {
		eChan <- err
		return eChan
	}

	chID := s.messageChannelID(ch)

	if opts != nil && opts.HistorySize > 0 && opts.HistoryLifetime > 0 {
		pr := pubRequest{
			channel:    chID,
			message:    byteMessage,
			historyKey: s.getHistoryKey(ch),
			indexKey:   s.gethistorySeqKey(ch),
			opts:       opts,
			err:        eChan,
		}
		select {
		case s.pubCh <- pr:
		default:
			timer := timers.AcquireTimer(s.readTimeout())
			defer timers.ReleaseTimer(timer)
			select {
			case s.pubCh <- pr:
			case <-timer.C:
				eChan <- errRedisOpTimeout
				return eChan
			}
		}
		return eChan
	}

	pr := pubRequest{
		channel: chID,
		message: byteMessage,
		err:     eChan,
	}
	timer := timers.AcquireTimer(s.readTimeout())
	defer timers.ReleaseTimer(timer)
	select {
	case s.pubCh <- pr:
	case <-timer.C:
		eChan <- errRedisOpTimeout
		return eChan
	}
	return eChan
}

// PublishJoin - see engine interface description.
func (s *shard) PublishJoin(ch string, join *Join, opts *ChannelOptions) <-chan error {

	eChan := make(chan error, 1)

	data, err := s.pushEncoder.EncodeJoin(join)
	if err != nil {
		eChan <- err
		return eChan
	}
	byteMessage, err := s.pushEncoder.Encode(proto.NewJoinPush(ch, data))
	if err != nil {
		eChan <- err
		return eChan
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
			eChan <- errRedisOpTimeout
			return eChan
		}
	}
	return eChan
}

// PublishLeave - see engine interface description.
func (s *shard) PublishLeave(ch string, leave *Leave, opts *ChannelOptions) <-chan error {

	eChan := make(chan error, 1)

	data, err := s.pushEncoder.EncodeLeave(leave)
	if err != nil {
		eChan <- err
		return eChan
	}
	byteMessage, err := s.pushEncoder.Encode(proto.NewLeavePush(ch, data))
	if err != nil {
		eChan <- err
		return eChan
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
			eChan <- errRedisOpTimeout
			return eChan
		}
	}
	return eChan
}

// PublishControl - see engine interface description.
func (s *shard) PublishControl(data []byte) <-chan error {
	eChan := make(chan error, 1)

	chID := s.controlChannelID()

	pr := pubRequest{
		channel: chID,
		message: data,
		err:     eChan,
	}
	s.pubCh <- pr
	return eChan
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
	if s.node.logger.enabled(LogLevelDebug) {
		s.node.logger.log(newLogEntry(LogLevelDebug, "subscribe node on channel", map[string]interface{}{"channel": ch}))
	}
	r := newSubRequest([]channelID{s.messageChannelID(ch)}, true)
	return s.sendSubscribe(r)
}

// Unsubscribe - see engine interface description.
func (s *shard) Unsubscribe(ch string) error {
	if s.node.logger.enabled(LogLevelDebug) {
		s.node.logger.log(newLogEntry(LogLevelDebug, "unsubscribe node from channel", map[string]interface{}{"channel": ch}))
	}
	r := newSubRequest([]channelID{s.messageChannelID(ch)}, false)
	return s.sendSubscribe(r)
}

func (s *shard) getDataResponse(r dataRequest) *dataResponse {
	select {
	case s.dataCh <- r:
	default:
		select {
		case s.dataCh <- r:
		case <-time.After(s.readTimeout()):
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
	hashKey := s.getPresenceHashKey(ch)
	setKey := s.getPresenceSetKey(ch)
	dr := newDataRequest(dataOpAddPresence, []interface{}{setKey, hashKey, expire, expireAt, uid, infoJSON})
	resp := s.getDataResponse(dr)
	return resp.err
}

// RemovePresence - see engine interface description.
func (s *shard) RemovePresence(ch string, uid string) error {
	hashKey := s.getPresenceHashKey(ch)
	setKey := s.getPresenceSetKey(ch)
	dr := newDataRequest(dataOpRemovePresence, []interface{}{setKey, hashKey, uid})
	resp := s.getDataResponse(dr)
	return resp.err
}

// Presence - see engine interface description.
func (s *shard) Presence(ch string) (map[string]*ClientInfo, error) {
	hashKey := s.getPresenceHashKey(ch)
	setKey := s.getPresenceSetKey(ch)
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
func (s *shard) History(ch string, limit int) ([]*Publication, error) {
	var rangeBound = -1
	if limit > 0 {
		rangeBound = limit - 1 // Redis includes last index into result
	}
	historyKey := s.getHistoryKey(ch)
	dr := newDataRequest(dataOpHistory, []interface{}{historyKey, 0, rangeBound})
	resp := s.getDataResponse(dr)
	if resp.err != nil {
		return nil, resp.err
	}
	return sliceOfPubs(s, resp.reply, nil)
}

// History - see engine interface description.
func (s *shard) HistorySequence(ch string) (recovery, error) {
	historySeqKey := s.gethistorySeqKey(ch)
	historyEpochKey := s.gethistoryEpochKey(ch)
	dr := newDataRequest(dataOphistorySeq, []interface{}{historySeqKey, historyEpochKey})
	resp := s.getDataResponse(dr)
	if resp.err != nil {
		return recovery{}, resp.err
	}
	results := resp.reply.([]interface{})

	sequence, err := redis.Int64(results[0], nil)
	if err != nil {
		if err != redis.ErrNil {
			return recovery{}, err
		}
		sequence = 0
	}

	seq, gen := unpackUint64(uint64(sequence))

	var epoch string
	epoch, err = redis.String(results[1], nil)
	if err != nil {
		if err != redis.ErrNil {
			return recovery{}, err
		}
		epoch = ""
	}

	return recovery{seq, gen, epoch}, nil
}

// RecoverHistory - see engine interface description.
func (s *shard) RecoverHistory(ch string, since *recovery) ([]*Publication, bool, recovery, error) {

	currentRecovery, err := s.HistorySequence(ch)
	if err != nil {
		return nil, false, recovery{}, err
	}

	if since == nil {
		return nil, false, currentRecovery, nil
	}

	if currentRecovery.Seq == since.Seq && since.Gen == currentRecovery.Gen && since.Epoch == currentRecovery.Epoch {
		return nil, true, currentRecovery, nil
	}

	publications, err := s.History(ch, 0)
	if err != nil {
		return nil, false, recovery{}, err
	}

	nextSeq := since.Seq + 1
	nextGen := since.Gen

	if nextSeq > maxSeq {
		nextSeq = 0
		nextGen = nextGen + 1
	}

	position := -1

	for i := len(publications) - 1; i >= 0; i-- {
		msg := publications[i]
		if msg.Seq == since.Seq && msg.Gen == since.Gen {
			position = i
			break
		}
		if msg.Seq == nextSeq && msg.Gen == nextGen {
			position = i + 1
			break
		}
	}
	if position > -1 {
		return publications[0:position], currentRecovery.Epoch == since.Epoch, currentRecovery, nil
	}

	return publications, false, currentRecovery, nil
}

// RemoveHistory - see engine interface description.
func (s *shard) RemoveHistory(ch string) error {
	historyKey := s.getHistoryKey(ch)
	dr := newDataRequest(dataOpHistoryRemove, []interface{}{historyKey})
	resp := s.getDataResponse(dr)
	return resp.err
}

// Channels - see engine interface description.
// Requires Redis >= 2.8.0 (http://redis.io/commands/pubsub)
func (s *shard) Channels() ([]string, error) {
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
			return nil, errors.New("ScanMap key not a bulk string value")
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

func sliceOfPubs(n *shard, result interface{}, err error) ([]*Publication, error) {
	values, err := redis.Values(result, err)
	if err != nil {
		return nil, err
	}
	msgs := make([]*Publication, len(values))
	for i := 0; i < len(values); i++ {
		value, okValue := values[i].([]byte)
		if !okValue {
			return nil, errors.New("error getting Message value")
		}

		pushData, seq, gen := extractPushData(value)

		msg, err := n.pushDecoder.Decode(pushData)
		if err != nil {
			return nil, fmt.Errorf("can not unmarshal value to Message: %v", err)
		}

		if msg.Type != proto.PushTypePublication {
			return nil, fmt.Errorf("wrong message type in history: %d", msg.Type)
		}

		publication, err := n.pushDecoder.DecodePublication(msg.Data)
		if err != nil {
			return nil, fmt.Errorf("can not unmarshal value to Pub: %v", err)
		}

		publication.Seq = seq
		publication.Gen = gen
		msgs[i] = publication
	}
	return msgs, nil
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

// index chooses bucket number in range [0, numBuckets).
func index(s string, numBuckets int) int {
	hash := fnv.New64a()
	hash.Write([]byte(s))
	return int(hash.Sum64() % uint64(numBuckets))
}
