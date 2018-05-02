package centrifuge

import (
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

	"github.com/FZambia/sentinel"
	"github.com/gomodule/redigo/redis"
)

const (
	// redisSubscribeChannelSize is the size for the internal buffered channels RedisEngine
	// uses to synchronize subscribe/unsubscribe.
	redisSubscribeChannelSize = 4096
	// redisPubSubWorkerChannelSize sets buffer size of channel to which we send all
	// messages received from Redis PUB/SUB connection to process in separate goroutine.
	redisPubSubWorkerChannelSize = 4096
	// redisSubscribeBatchLimit is a maximum number of channels to include in a single subscribe
	// call. Redis documentation doesn't specify a maximum allowed but we think it probably makes
	// sense to keep a sane limit given how many subscriptions a single Centrifugo instance might
	// be handling.
	redisSubscribeBatchLimit = 2048
	// redisPublishChannelSize is the size for the internal buffered channel RedisEngine uses
	// to collect publish requests.
	redisPublishChannelSize = 1024
	// redisPublishBatchLimit is a maximum limit of publish requests one batched publish
	// operation can contain.
	redisPublishBatchLimit = 2048
	// redisDataChannelSize is a buffer size of channel with data operation requests.
	redisDataChannelSize = 256
)

const (
	defaultPrefix         = "centrifuge"
	defaultReadTimeout    = 5 * time.Second
	defaultWriteTimeout   = time.Second
	defaultConnectTimeout = time.Second
	defaultPoolSize       = 256
)

type (
	// channelID is unique channel identificator in Redis.
	channelID string
)

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
	// MasterName is a name of Redis instance master Sentinel monitors.
	MasterName string
	// SentinelAddrs is a slice of Sentinel addresses.
	SentinelAddrs []string
	// Prefix to use before every channel name and key in Redis.
	Prefix string
	// PubSubNumWorkers sets how many PUB/SUB message processing workers will be started.
	// By default we start runtime.NumCPU() workers.
	PubSubNumWorkers int
	// ReadTimeout is a timeout on read operations. Note that at moment it should be greater
	// than node ping publish interval in order to prevent timing out Pubsub connection's
	// Receive call.
	ReadTimeout time.Duration
	// WriteTimeout is a timeout on write operations
	WriteTimeout time.Duration
	// ConnectTimeout is a timeout on connect operation
	ConnectTimeout time.Duration
}

// subRequest is an internal request to subscribe or unsubscribe from one or more channels
type subRequest struct {
	channels  []channelID
	subscribe bool
	err       chan error
}

// newSubRequest creates a new request to subscribe or unsubscribe form a channel.
// If the caller cares about response they should set wantResponse and then call
// result() on the request once it has been pushed to the appropriate chan.
func newSubRequest(chIDs []channelID, subscribe bool, wantResponse bool) subRequest {
	r := subRequest{
		channels:  chIDs,
		subscribe: subscribe,
	}
	if wantResponse {
		r.err = make(chan error, 1)
	}
	return r
}

func (sr *subRequest) done(err error) {
	if sr.err == nil {
		return
	}
	sr.err <- err
}

func (sr *subRequest) result() error {
	if sr.err == nil {
		// No waiting, as caller didn't care about response.
		return nil
	}
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

	maxIdle := 10
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
				c, err := redis.DialTimeout("tcp", addr, timeout, timeout, timeout)
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
		IdleTimeout: 240 * time.Second,
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

			c, err := redis.DialTimeout("tcp", serverAddr, connectTimeout, readTimeout, writeTimeout)
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
	// KEYS[2] - history touch object key
	// ARGV[1] - channel to publish message to
	// ARGV[2] - message payload
	// ARGV[3] - history size
	// ARGV[4] - history lifetime
	// ARGV[5] - history drop inactive flag - "0" or "1"
	pubScriptSource = `
local n = redis.call("publish", ARGV[1], ARGV[2])
local m = 0
if ARGV[5] == "1" and n == 0 and redis.call("exists", KEYS[2]) == 0 then
  m = redis.call("lpushx", KEYS[1], ARGV[2])
else
  m = redis.call("lpush", KEYS[1], ARGV[2])
end
if m > 0 then
  redis.call("ltrim", KEYS[1], 0, ARGV[3])
  redis.call("expire", KEYS[1], ARGV[4])
end
return n
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
)

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
		pushEncoder:       proto.NewProtobufPushEncoder(),
		pushDecoder:       proto.NewProtobufPushDecoder(),
	}
	shard.pubCh = make(chan pubRequest, redisPublishChannelSize)
	shard.subCh = make(chan subRequest, redisSubscribeChannelSize)
	shard.dataCh = make(chan dataRequest, redisDataChannelSize)
	shard.messagePrefix = conf.Prefix + redisClientChannelPrefix
	return shard, nil
}

func (e *shard) messageChannelID(ch string) channelID {
	return channelID(e.messagePrefix + ch)
}

func (e *shard) controlChannelID() channelID {
	return channelID(e.config.Prefix + redisControlChannelSuffix)
}

func (e *shard) pingChannelID() channelID {
	return channelID(e.config.Prefix + redisPingChannelSuffix)
}

func (e *shard) getPresenceHashKey(ch string) channelID {
	return channelID(e.config.Prefix + ".presence.data." + ch)
}

func (e *shard) getPresenceSetKey(ch string) channelID {
	return channelID(e.config.Prefix + ".presence.expire." + ch)
}

func (e *shard) getHistoryKey(ch string) channelID {
	return channelID(e.config.Prefix + ".history.list." + ch)
}

func (e *shard) getHistoryTouchKey(ch string) channelID {
	return channelID(e.config.Prefix + ".history.touch." + ch)
}

func (e *RedisEngine) shardIndex(channel string) int {
	if !e.sharding {
		return 0
	}
	return consistentIndex(channel, len(e.shards))
}

// Name returns name of engine.
func (e *RedisEngine) name() string {
	return "Redis"
}

// Run runs engine after node initialized.
func (e *RedisEngine) run() error {
	for _, shard := range e.shards {
		err := shard.Run()
		if err != nil {
			return err
		}
	}
	return nil
}

// Publish - see engine interface description.
func (e *RedisEngine) publish(ch string, pub *proto.Publication, opts *ChannelOptions) <-chan error {
	return e.shards[e.shardIndex(ch)].Publish(ch, pub, opts)
}

// PublishJoin - see engine interface description.
func (e *RedisEngine) publishJoin(ch string, join *proto.Join, opts *ChannelOptions) <-chan error {
	return e.shards[e.shardIndex(ch)].PublishJoin(ch, join, opts)
}

// PublishLeave - see engine interface description.
func (e *RedisEngine) publishLeave(ch string, leave *proto.Leave, opts *ChannelOptions) <-chan error {
	return e.shards[e.shardIndex(ch)].PublishLeave(ch, leave, opts)
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
	return e.shards[e.shardIndex(ch)].Subscribe(ch)
}

// Unsubscribe - see engine interface description.
func (e *RedisEngine) unsubscribe(ch string) error {
	return e.shards[e.shardIndex(ch)].Unsubscribe(ch)
}

// AddPresence - see engine interface description.
func (e *RedisEngine) addPresence(ch string, uid string, info *proto.ClientInfo, exp time.Duration) error {
	expire := int(exp.Seconds())
	return e.shards[e.shardIndex(ch)].AddPresence(ch, uid, info, expire)
}

// RemovePresence - see engine interface description.
func (e *RedisEngine) removePresence(ch string, uid string) error {
	return e.shards[e.shardIndex(ch)].RemovePresence(ch, uid)
}

// Presence - see engine interface description.
func (e *RedisEngine) presence(ch string) (map[string]*proto.ClientInfo, error) {
	return e.shards[e.shardIndex(ch)].Presence(ch)
}

// PresenceStats - see engine interface description.
func (e *RedisEngine) presenceStats(ch string) (presenceStats, error) {
	return e.shards[e.shardIndex(ch)].PresenceStats(ch)
}

// History - see engine interface description.
func (e *RedisEngine) history(ch string, filter historyFilter) ([]*proto.Publication, error) {
	return e.shards[e.shardIndex(ch)].History(ch, filter)
}

// RecoverHistory - see engine interface description.
func (e *RedisEngine) recoverHistory(ch string, lastUID string) ([]*proto.Publication, bool, error) {
	return e.shards[e.shardIndex(ch)].RecoverHistory(ch, lastUID)
}

// RemoveHistory - see engine interface description.
func (e *RedisEngine) removeHistory(ch string) error {
	return e.shards[e.shardIndex(ch)].RemoveHistory(ch)
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

// Run runs Redis shard.
func (e *shard) Run() error {
	go e.runForever(func() {
		e.runPublishPipeline()
	})
	go e.runForever(func() {
		e.runPubSub()
	})
	go e.runForever(func() {
		e.runDataPipeline()
	})
	return nil
}

// runForever simple keeps another function running indefinitely
// the reason this loop is not inside the function itself is so that defer
// can be used to cleanup nicely (defers only run at function return not end of block scope)
func (e *shard) runForever(fn func()) {
	for {
		fn()
		// Sleep for a while to prevent busy loop when reconnecting to Redis.
		time.Sleep(300 * time.Millisecond)
	}
}

func (e *shard) runPubSub() {

	numWorkers := e.config.PubSubNumWorkers
	if numWorkers == 0 {
		numWorkers = runtime.NumCPU()
	}

	e.node.logger.log(newLogEntry(LogLevelDebug, fmt.Sprintf("running Redis PUB/SUB, num workers: %d", numWorkers)))
	defer func() {
		e.node.logger.log(newLogEntry(LogLevelDebug, "stopping Redis PUB/SUB"))
	}()

	poolConn := e.pool.Get()
	if poolConn.Err() != nil {
		// At this moment test on borrow could already return an error,
		// we can't work with broken connection.
		poolConn.Close()
		return
	}

	conn := redis.PubSubConn{Conn: poolConn}
	defer conn.Close()

	done := make(chan struct{})
	defer close(done)

	// Run subscriber goroutine.
	go func() {
		e.node.logger.log(newLogEntry(LogLevelDebug, "Starting RedisEngine Subscriber"))

		defer func() {
			e.node.logger.log(newLogEntry(LogLevelDebug, "Stopping RedisEngine Subscriber"))
		}()
		for {
			select {
			case <-done:
				return
			case r := <-e.subCh:
				chIDs := make([]interface{}, len(r.channels))
				i := 0
				for _, ch := range r.channels {
					chIDs[i] = ch
					i++
				}

				var opErr error
				if r.subscribe {
					opErr = conn.Subscribe(chIDs...)
				} else {
					opErr = conn.Unsubscribe(chIDs...)
				}

				if opErr != nil {
					e.node.logger.log(newLogEntry(LogLevelError, "RedisEngine Subscriber error", map[string]interface{}{"error": opErr.Error()}))
					r.done(opErr)

					// Close conn, this should cause Receive to return with err below
					// and whole runPubSub method to restart.
					conn.Close()
					return
				}
				r.done(nil)
			}
		}
	}()

	controlChannel := e.controlChannelID()
	pingChannel := e.pingChannelID()

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
						e.node.handleControl(n.Data)
					case pingChannel:
						// Do nothing - this message just maintains connection open.
					default:
						err := e.handleRedisClientMessage(chID, n.Data)
						if err != nil {
							e.node.logger.log(newLogEntry(LogLevelError, "error handling client message", map[string]interface{}{"error": err.Error()}))
							continue
						}
					}
				}
			}
		}(workerCh)
	}

	chIDs := make([]channelID, 2)
	chIDs[0] = controlChannel
	chIDs[1] = pingChannel

	for _, ch := range e.node.hub.Channels() {
		chIDs = append(chIDs, e.messageChannelID(ch))
	}

	batch := make([]channelID, 0)

	for i, ch := range chIDs {
		if len(batch) > 0 && i%redisSubscribeBatchLimit == 0 {
			r := newSubRequest(batch, true, true)
			e.subCh <- r
			err := r.result()
			if err != nil {
				e.node.logger.log(newLogEntry(LogLevelError, "error subscribing", map[string]interface{}{"error": err.Error()}))
				return
			}
			batch = nil
		}
		batch = append(batch, ch)
	}
	if len(batch) > 0 {
		r := newSubRequest(batch, true, true)
		e.subCh <- r
		err := r.result()
		if err != nil {
			e.node.logger.log(newLogEntry(LogLevelError, "error subscribing", map[string]interface{}{"error": err.Error()}))
			return
		}
	}

	e.node.logger.log(newLogEntry(LogLevelDebug, fmt.Sprintf("Successfully subscribed to %d Redis channels", len(chIDs))))

	for {
		switch n := conn.Receive().(type) {
		case redis.Message:
			// Add message to worker channel preserving message order - i.e. messages from
			// the same channel will be processed in the same worker.
			workers[index(n.Channel, numWorkers)] <- n
		case redis.Subscription:
		case error:
			e.node.logger.log(newLogEntry(LogLevelError, "Redis receiver error", map[string]interface{}{"error": n.Error()}))
			return
		}
	}
}

func (e *shard) handleRedisClientMessage(chID channelID, data []byte) error {
	var message proto.Push
	err := message.Unmarshal(data)
	if err != nil {
		return err
	}
	return e.handleClientPush(&message)
}

func (e *shard) handleClientPush(push *proto.Push) error {
	switch push.Type {
	case proto.PushTypePublication:
		pub, err := e.pushDecoder.DecodePublication(push.Data)
		if err != nil {
			return err
		}
		e.node.handlePublication(push.Channel, pub)
	case proto.PushTypeJoin:
		join, err := e.pushDecoder.DecodeJoin(push.Data)
		if err != nil {
			return err
		}
		e.node.handleJoin(push.Channel, join)
	case proto.PushTypeLeave:
		leave, err := e.pushDecoder.DecodeLeave(push.Data)
		if err != nil {
			return err
		}
		e.node.handleLeave(push.Channel, leave)
	default:
	}
	return nil
}

type pubRequest struct {
	channel    channelID
	message    []byte
	historyKey channelID
	touchKey   channelID
	opts       *ChannelOptions
	err        *chan error
}

func (pr *pubRequest) done(err error) {
	*(pr.err) <- err
}

func (pr *pubRequest) result() error {
	return <-*(pr.err)
}

func fillPublishBatch(ch chan pubRequest, prs *[]pubRequest) {
	for len(*prs) < redisPublishBatchLimit {
		select {
		case pr := <-ch:
			*prs = append(*prs, pr)
		default:
			return
		}
	}
}

func (e *shard) runPublishPipeline() {
	conn := e.pool.Get()

	err := e.pubScript.Load(conn)
	if err != nil {
		e.node.logger.log(newLogEntry(LogLevelError, "error loading publish Lua script", map[string]interface{}{"error": err.Error()}))
		// Can not proceed if script has not been loaded - because we use EVALSHA command for
		// publishing with history.
		conn.Close()
		return
	}

	conn.Close()

	var prs []pubRequest

	var readTimeout = defaultReadTimeout
	if e.config.ReadTimeout != 0 {
		readTimeout = e.config.ReadTimeout
	}

	pingTimeout := readTimeout / 3

	for {
		select {
		case <-time.After(pingTimeout):
			// We have to PUBLISH pings into connection to prevent connection close after read timeout.
			// In our case it's important to maintain PUB/SUB receiver connection alive to prevent
			// resubscribing on all our subscriptions again and again.
			conn := e.pool.Get()
			err := conn.Send("PUBLISH", e.pingChannelID(), nil)
			if err != nil {
				e.node.logger.log(newLogEntry(LogLevelError, "error publish ping to Redis channel", map[string]interface{}{"error": err.Error()}))
				conn.Close()
				return
			}
			conn.Close()
		case pr := <-e.pubCh:
			prs = append(prs, pr)
			fillPublishBatch(e.pubCh, &prs)
			conn := e.pool.Get()
			for i := range prs {
				if prs[i].opts != nil && prs[i].opts.HistorySize > 0 && prs[i].opts.HistoryLifetime > 0 {
					e.pubScript.SendHash(conn, prs[i].historyKey, prs[i].touchKey, prs[i].channel, prs[i].message, prs[i].opts.HistorySize, prs[i].opts.HistoryLifetime, prs[i].opts.HistoryDropInactive)
				} else {
					conn.Send("PUBLISH", prs[i].channel, prs[i].message)
				}
			}
			err := conn.Flush()
			if err != nil {
				for i := range prs {
					prs[i].done(err)
				}
				e.node.logger.log(newLogEntry(LogLevelError, "error flushing publish pipeline", map[string]interface{}{"error": err.Error()}))
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
	dataOpHistoryRemove
	dataOpChannels
	dataOpHistoryTouch
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

func newDataRequest(op dataOp, args []interface{}, wantResponse bool) dataRequest {
	r := dataRequest{op: op, args: args}
	if wantResponse {
		r.resp = make(chan *dataResponse, 1)
	}
	return r
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

func fillDataBatch(ch <-chan dataRequest, batch *[]dataRequest, maxSize int) {
	for len(*batch) < maxSize {
		select {
		case req := <-ch:
			*batch = append(*batch, req)
		default:
			return
		}
	}
}

func (e *shard) runDataPipeline() {

	conn := e.pool.Get()

	err := e.addPresenceScript.Load(conn)
	if err != nil {
		e.node.logger.log(newLogEntry(LogLevelError, "error loading add presence Lua", map[string]interface{}{"error": err.Error()}))
		// Can not proceed if script has not been loaded.
		conn.Close()
		return
	}

	err = e.presenceScript.Load(conn)
	if err != nil {
		e.node.logger.log(newLogEntry(LogLevelError, "error loading presence Lua", map[string]interface{}{"error": err.Error()}))
		// Can not proceed if script has not been loaded.
		conn.Close()
		return
	}

	err = e.remPresenceScript.Load(conn)
	if err != nil {
		e.node.logger.log(newLogEntry(LogLevelError, "error loading remove presence Lua", map[string]interface{}{"error": err.Error()}))
		// Can not proceed if script has not been loaded.
		conn.Close()
		return
	}

	conn.Close()

	var drs []dataRequest

	for {
		select {
		case dr := <-e.dataCh:
			drs = append(drs, dr)
			fillDataBatch(e.dataCh, &drs, redisDataChannelSize)

			conn := e.pool.Get()

			for i := range drs {
				switch drs[i].op {
				case dataOpAddPresence:
					e.addPresenceScript.SendHash(conn, drs[i].args...)
				case dataOpRemovePresence:
					e.remPresenceScript.SendHash(conn, drs[i].args...)
				case dataOpPresence:
					e.presenceScript.SendHash(conn, drs[i].args...)
				case dataOpHistory:
					conn.Send("LRANGE", drs[i].args...)
				case dataOpHistoryRemove:
					conn.Send("DEL", drs[i].args...)
				case dataOpChannels:
					conn.Send("PUBSUB", drs[i].args...)
				case dataOpHistoryTouch:
					conn.Send("SETEX", drs[i].args...)
				}
			}

			err := conn.Flush()
			if err != nil {
				for i := range drs {
					drs[i].done(nil, err)
				}
				e.node.logger.log(newLogEntry(LogLevelError, "error flushing data pipeline", map[string]interface{}{"error": err.Error()}))
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
}

// Publish - see engine interface description.
func (e *shard) Publish(ch string, pub *proto.Publication, opts *ChannelOptions) <-chan error {

	eChan := make(chan error, 1)

	data, err := e.pushEncoder.EncodePublication(pub)
	if err != nil {
		eChan <- err
		return eChan
	}
	byteMessage, err := e.pushEncoder.Encode(proto.NewPublication(ch, data))
	if err != nil {
		eChan <- err
		return eChan
	}

	chID := e.messageChannelID(ch)

	if opts != nil && opts.HistorySize > 0 && opts.HistoryLifetime > 0 {
		pr := pubRequest{
			channel:    chID,
			message:    byteMessage,
			historyKey: e.getHistoryKey(ch),
			touchKey:   e.getHistoryTouchKey(ch),
			opts:       opts,
			err:        &eChan,
		}
		e.pubCh <- pr
		return eChan
	}

	pr := pubRequest{
		channel: chID,
		message: byteMessage,
		err:     &eChan,
	}
	e.pubCh <- pr
	return eChan
}

// PublishJoin - see engine interface description.
func (e *shard) PublishJoin(ch string, join *proto.Join, opts *ChannelOptions) <-chan error {

	eChan := make(chan error, 1)

	data, err := e.pushEncoder.EncodeJoin(join)
	if err != nil {
		eChan <- err
		return eChan
	}
	byteMessage, err := e.pushEncoder.Encode(proto.NewJoin(ch, data))
	if err != nil {
		eChan <- err
		return eChan
	}

	chID := e.messageChannelID(ch)

	pr := pubRequest{
		channel: chID,
		message: byteMessage,
		err:     &eChan,
	}
	e.pubCh <- pr
	return eChan
}

// PublishLeave - see engine interface description.
func (e *shard) PublishLeave(ch string, leave *proto.Leave, opts *ChannelOptions) <-chan error {

	eChan := make(chan error, 1)

	data, err := e.pushEncoder.EncodeLeave(leave)
	if err != nil {
		eChan <- err
		return eChan
	}
	byteMessage, err := e.pushEncoder.Encode(proto.NewLeave(ch, data))
	if err != nil {
		eChan <- err
		return eChan
	}

	chID := e.messageChannelID(ch)

	pr := pubRequest{
		channel: chID,
		message: byteMessage,
		err:     &eChan,
	}
	e.pubCh <- pr
	return eChan
}

// PublishControl - see engine interface description.
func (e *shard) PublishControl(data []byte) <-chan error {
	eChan := make(chan error, 1)

	chID := e.controlChannelID()

	pr := pubRequest{
		channel: chID,
		message: data,
		err:     &eChan,
	}
	e.pubCh <- pr
	return eChan
}

// Subscribe - see engine interface description.
func (e *shard) Subscribe(ch string) error {
	e.node.logger.log(newLogEntry(LogLevelDebug, "subscribe node on channel", map[string]interface{}{"channel": ch}))
	channel := e.messageChannelID(ch)
	r := newSubRequest([]channelID{channel}, true, true)
	e.subCh <- r
	return r.result()
}

// Unsubscribe - see engine interface description.
func (e *shard) Unsubscribe(ch string) error {
	e.node.logger.log(newLogEntry(LogLevelDebug, "unsubscribe node from channel", map[string]interface{}{"channel": ch}))
	channel := e.messageChannelID(ch)
	r := newSubRequest([]channelID{channel}, false, true)
	e.subCh <- r

	if chOpts, ok := e.node.ChannelOpts(ch); ok && chOpts.HistoryDropInactive {
		// Waiting for response here is not actually required. But this seems
		// semantically correct and allows avoid races in drop inactive tests.
		// It does not seem a big bottleneck for real usage but can be tuned in
		// future if we find any problems with it.
		dr := newDataRequest(dataOpHistoryTouch, []interface{}{e.getHistoryTouchKey(ch), chOpts.HistoryLifetime, ""}, true)
		e.dataCh <- dr
		dr.result()
	}
	return r.result()
}

// AddPresence - see engine interface description.
func (e *shard) AddPresence(ch string, uid string, info *proto.ClientInfo, expire int) error {
	infoJSON, err := info.Marshal()
	if err != nil {
		return err
	}
	expireAt := time.Now().Unix() + int64(expire)
	hashKey := e.getPresenceHashKey(ch)
	setKey := e.getPresenceSetKey(ch)
	dr := newDataRequest(dataOpAddPresence, []interface{}{setKey, hashKey, expire, expireAt, uid, infoJSON}, true)
	e.dataCh <- dr
	resp := dr.result()
	return resp.err
}

// RemovePresence - see engine interface description.
func (e *shard) RemovePresence(ch string, uid string) error {
	hashKey := e.getPresenceHashKey(ch)
	setKey := e.getPresenceSetKey(ch)
	dr := newDataRequest(dataOpRemovePresence, []interface{}{setKey, hashKey, uid}, true)
	e.dataCh <- dr
	resp := dr.result()
	return resp.err
}

// Presence - see engine interface description.
func (e *shard) Presence(ch string) (map[string]*proto.ClientInfo, error) {
	hashKey := e.getPresenceHashKey(ch)
	setKey := e.getPresenceSetKey(ch)
	now := int(time.Now().Unix())
	dr := newDataRequest(dataOpPresence, []interface{}{setKey, hashKey, now}, true)
	e.dataCh <- dr
	resp := dr.result()
	if resp.err != nil {
		return nil, resp.err
	}
	return mapStringClientInfo(resp.reply, nil)
}

// Presence - see engine interface description.
func (e *shard) PresenceStats(ch string) (presenceStats, error) {
	presence, err := e.Presence(ch)
	if err != nil {
		return presenceStats{}, err
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

	return presenceStats{
		NumClients: numClients,
		NumUsers:   numUsers,
	}, nil
}

// History - see engine interface description.
func (e *shard) History(ch string, filter historyFilter) ([]*proto.Publication, error) {
	limit := filter.Limit
	var rangeBound = -1
	if limit > 0 {
		rangeBound = limit - 1 // Redis includes last index into result
	}
	historyKey := e.getHistoryKey(ch)
	dr := newDataRequest(dataOpHistory, []interface{}{historyKey, 0, rangeBound}, true)
	e.dataCh <- dr
	resp := dr.result()
	if resp.err != nil {
		return nil, resp.err
	}
	return sliceOfPubs(e, resp.reply, nil)
}

// RecoverHistory - see engine interface description.
func (e *shard) RecoverHistory(ch string, last string) ([]*proto.Publication, bool, error) {
	publications, err := e.History(ch, historyFilter{Limit: 0})
	if err != nil {
		return nil, false, err
	}

	if last == "" {
		// Client wants to recover publications but it seems that there were no
		// messages in history before, so client missed all existing messages.
		return publications, false, nil
	}

	position := -1
	for index, msg := range publications {
		if msg.UID == last {
			position = index
			break
		}
	}
	if position > -1 {
		// Last uid provided found in history. Set recovered flag which means that
		// server thinks missed messages fully recovered.
		return publications[0:position], true, nil
	}
	// Provided last UID not found in history messages. This means that client
	// most probably missed too many messages (or maybe wrong last uid provided but
	// it's not a normal case). So we try to compensate as many as we can. But
	// recovered flag stays false so we do not give a guarantee all missed messages
	// recovered successfully.
	return publications, false, nil
}

// RemoveHistory - see engine interface description.
func (e *shard) RemoveHistory(ch string) error {
	historyKey := e.getHistoryKey(ch)
	dr := newDataRequest(dataOpHistoryRemove, []interface{}{historyKey}, true)
	e.dataCh <- dr
	resp := dr.result()
	if resp.err != nil {
		return resp.err
	}
	return nil
}

// Channels - see engine interface description.
// Requires Redis >= 2.8.0 (http://redis.io/commands/pubsub)
func (e *shard) Channels() ([]string, error) {
	dr := newDataRequest(dataOpChannels, []interface{}{"CHANNELS", e.messagePrefix + "*"}, true)
	e.dataCh <- dr
	resp := dr.result()
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
		channels = append(channels, string(chID)[len(e.messagePrefix):])
	}
	return channels, nil
}

func mapStringClientInfo(result interface{}, err error) (map[string]*proto.ClientInfo, error) {
	values, err := redis.Values(result, err)
	if err != nil {
		return nil, err
	}
	if len(values)%2 != 0 {
		return nil, errors.New("mapStringClientInfo expects even number of values result")
	}
	m := make(map[string]*proto.ClientInfo, len(values)/2)
	for i := 0; i < len(values); i += 2 {
		key, okKey := values[i].([]byte)
		value, okValue := values[i+1].([]byte)
		if !okKey || !okValue {
			return nil, errors.New("ScanMap key not a bulk string value")
		}
		var f proto.ClientInfo
		err = f.Unmarshal(value)
		if err != nil {
			return nil, errors.New("can not unmarshal value to ClientInfo")
		}
		m[string(key)] = &f
	}
	return m, nil
}

func sliceOfPubs(n *shard, result interface{}, err error) ([]*proto.Publication, error) {
	values, err := redis.Values(result, err)
	if err != nil {
		return nil, err
	}
	msgs := make([]*proto.Publication, len(values))
	for i := 0; i < len(values); i++ {
		value, okValue := values[i].([]byte)
		if !okValue {
			return nil, errors.New("error getting Message value")
		}

		msg, err := n.pushDecoder.Decode(value)
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
