package engineredis

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/FZambia/go-sentinel"
	"github.com/centrifugal/centrifugo/libcentrifugo/api/v1"
	"github.com/centrifugal/centrifugo/libcentrifugo/config"
	"github.com/centrifugal/centrifugo/libcentrifugo/engine"
	"github.com/centrifugal/centrifugo/libcentrifugo/logger"
	"github.com/centrifugal/centrifugo/libcentrifugo/node"
	"github.com/centrifugal/centrifugo/libcentrifugo/plugin"
	"github.com/centrifugal/centrifugo/libcentrifugo/proto"
	"github.com/garyburd/redigo/redis"
)

func init() {
	plugin.RegisterEngine("redis", Plugin)
	plugin.RegisterConfigurator("redis", Configure)
}

// Configure is a Configurator function for Redis engine.
func Configure(setter config.Setter) error {

	setter.SetDefault("redis_prefix", "centrifugo")
	setter.SetDefault("redis_connect_timeout", 1)
	setter.SetDefault("redis_read_timeout", 10) // Must be greater than ping channel publish interval.
	setter.SetDefault("redis_write_timeout", 1)
	setter.SetDefault("redis_pubsub_num_workers", 0)
	setter.StringFlag("redis_host", "", "127.0.0.1", "redis host (Redis engine)")
	setter.StringFlag("redis_port", "", "6379", "redis port (Redis engine)")
	setter.StringFlag("redis_password", "", "", "redis auth password (Redis engine)")
	setter.StringFlag("redis_db", "", "0", "redis database (Redis engine)")
	setter.StringFlag("redis_url", "", "", "redis connection URL in format redis://:password@hostname:port/db (Redis engine)")
	setter.BoolFlag("redis_api", "", false, "enable Redis API listener (Redis engine)")
	setter.IntFlag("redis_pool", "", 256, "Redis pool size (Redis engine)")
	setter.IntFlag("redis_api_num_shards", "", 0, "Number of shards for redis API queue (Redis engine)")
	setter.StringFlag("redis_master_name", "", "", "Name of Redis master Sentinel monitors (Redis engine)")
	setter.StringFlag("redis_sentinels", "", "", "Comma separated list of Sentinels (Redis engine)")

	bindFlags := []string{
		"redis_host", "redis_port", "redis_password", "redis_db", "redis_url",
		"redis_api", "redis_pool", "redis_api_num_shards", "redis_master_name", "redis_sentinels",
	}
	for _, flag := range bindFlags {
		setter.BindFlag(flag, flag)
	}

	bindEnvs := []string{"redis_host", "redis_port", "redis_url"}
	for _, env := range bindEnvs {
		setter.BindEnv(env)
	}

	return nil
}

const (
	// RedisSubscribeChannelSize is the size for the internal buffered channels RedisEngine
	// uses to synchronize subscribe/unsubscribe. It allows for effective batching during bulk
	// re-subscriptions, and allows large volume of incoming subscriptions to not block when
	// PubSub connection is reconnecting. Two channels of this size will be allocated, one for
	// Subscribe and one for Unsubscribe
	RedisSubscribeChannelSize = 4096
	// RedisPubSubWorkerChannelSize sets buffer size of channel to which we send all
	// messages received from Redis PUB/SUB connection to process in separate goroutine.
	RedisPubSubWorkerChannelSize = 4096
	// RedisSubscribeBatchLimit is a maximum number of channels to include in a single subscribe
	// call. Redis documentation doesn't specify a maximum allowed but we think it probably makes
	// sense to keep a sane limit given how many subscriptions a single Centrifugo instance might
	// be handling.
	RedisSubscribeBatchLimit = 2048
	// RedisPublishChannelSize is the size for the internal buffered channel RedisEngine uses
	// to collect publish requests.
	RedisPublishChannelSize = 1024
	// RedisPublishBatchLimit is a maximum limit of publish requests one batched publish
	// operation can contain.
	RedisPublishBatchLimit = 2048
	// RedisDataBatchLimit limits amount of data operations combined in one pipeline.
	RedisDataBatchLimit = 8
	// RedisDataChannelSize is a buffer size of channel with data operation requests.
	RedisDataChannelSize = 256
)

type (
	// ChannelID is unique channel identificator in Redis.
	ChannelID string
)

const (
	// RedisAPIKeySuffix is a suffix for api queue (LIST) key.
	RedisAPIKeySuffix = ".api"
	// RedisControlChannelSuffix is a suffix for control channel.
	RedisControlChannelSuffix = ".control"
	// RedisPingChannelSuffix is a suffix for ping channel.
	RedisPingChannelSuffix = ".ping"
	// RedisAdminChannelSuffix is a suffix for admin channel.
	RedisAdminChannelSuffix = ".admin"
	// RedisMessageChannelPrefix is a prefix before channel name for client messages.
	RedisMessageChannelPrefix = ".message."
	// RedisJoinChannelPrefix is a prefix before channel name for join messages.
	RedisJoinChannelPrefix = ".join."
	// RedisLeaveChannelPrefix is a prefix before channel name for leave messages.
	RedisLeaveChannelPrefix = ".leave."
)

// RedisEngine uses Redis datastructures and PUB/SUB to manage Centrifugo logic.
// This engine allows to scale Centrifugo - you can run several Centrifugo instances
// connected to the same Redis and load balance clients between instances.
type RedisEngine struct {
	sync.RWMutex
	node     *node.Node
	sharding bool
	shards   []*Shard
}

// Shard has everything to connect to Redis instance.
type Shard struct {
	sync.RWMutex
	node              *node.Node
	config            *ShardConfig
	pool              *redis.Pool
	api               bool
	numApiShards      int
	subCh             chan subRequest
	pubCh             chan pubRequest
	dataCh            chan dataRequest
	pubScript         *redis.Script
	addPresenceScript *redis.Script
	remPresenceScript *redis.Script
	presenceScript    *redis.Script
	lpopManyScript    *redis.Script
	messagePrefix     string
	joinPrefix        string
	leavePrefix       string
}

// ShardConfig is struct with Redis Engine options.
type ShardConfig struct {
	// Host is Redis server host.
	Host string
	// Port is Redis server port.
	Port string
	// Password is password to use when connecting to Redis database. If empty then password not used.
	Password string
	// DB is Redis database number as string. If empty then database 0 used.
	DB string
	// MasterName is a name of Redis instance master Sentinel monitors.
	MasterName string
	// SentinelAddrs is a slice of Sentinel addresses.
	SentinelAddrs []string
	// PoolSize is a size of Redis connection pool.
	PoolSize int
	// API enables listening for API queues to publish API commands into Centrifugo via pushing
	// commands into Redis queue.
	API bool
	// NumAPIShards is a number of sharded API queues in Redis to increase volume of commands
	// (most probably publish) that Centrifugo instance can process.
	NumAPIShards int
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
	channel   ChannelID
	subscribe bool
	err       chan error
}

// newSubRequest creates a new request to subscribe or unsubscribe form a channel.
// If the caller cares about response they should set wantResponse and then call
// result() on the request once it has been pushed to the appropriate chan.
func newSubRequest(chID ChannelID, subscribe bool, wantResponse bool) subRequest {
	r := subRequest{
		channel:   chID,
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
		// No waiting, as caller didn't care about response
		return nil
	}
	return <-sr.err
}

func newPool(conf *ShardConfig) *redis.Pool {

	host := conf.Host
	port := conf.Port
	password := conf.Password
	db := "0"
	if conf.DB != "" {
		db = conf.DB
	}

	serverAddr := net.JoinHostPort(host, port)
	useSentinel := conf.MasterName != "" && len(conf.SentinelAddrs) > 0

	usingPassword := yesno(password != "")
	apiEnabled := yesno(conf.API)
	var shardsSuffix string
	if conf.API {
		shardsSuffix = fmt.Sprintf(", num API shard queues: %d", conf.NumAPIShards)
	}
	if !useSentinel {
		logger.INFO.Printf("Redis: %s/%s, pool: %d, using password: %s, API enabled: %s%s\n", serverAddr, db, conf.PoolSize, usingPassword, apiEnabled, shardsSuffix)
	} else {
		logger.INFO.Printf("Redis: Sentinel for name: %s, db: %s, pool: %d, using password: %s, API enabled: %s%s\n", conf.MasterName, db, conf.PoolSize, usingPassword, apiEnabled, shardsSuffix)
	}

	var lastMu sync.Mutex
	var lastMaster string

	maxIdle := 10
	if conf.PoolSize < maxIdle {
		maxIdle = conf.PoolSize
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
					logger.CRITICAL.Println(err)
					return nil, err
				}
				return c, nil
			},
		}

		// Periodically discover new Sentinels.
		go func() {
			if err := sntnl.Discover(); err != nil {
				logger.ERROR.Println(err)
			}
			for {
				select {
				case <-time.After(30 * time.Second):
					if err := sntnl.Discover(); err != nil {
						logger.ERROR.Println(err)
					}
				}
			}
		}()
	}

	return &redis.Pool{
		MaxIdle:     maxIdle,
		MaxActive:   conf.PoolSize,
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
					logger.INFO.Printf("Redis master discovered: %s", serverAddr)
					lastMaster = serverAddr
				}
				lastMu.Unlock()
			}

			c, err := redis.DialTimeout("tcp", serverAddr, conf.ConnectTimeout, conf.ReadTimeout, conf.WriteTimeout)
			if err != nil {
				logger.CRITICAL.Println(err)
				return nil, err
			}

			if password != "" {
				if _, err := c.Do("AUTH", password); err != nil {
					c.Close()
					logger.CRITICAL.Println(err)
					return nil, err
				}
			}

			if db != "0" {
				if _, err := c.Do("SELECT", db); err != nil {
					c.Close()
					logger.CRITICAL.Println(err)
					return nil, err
				}
			}

			return c, err
		},
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			if useSentinel {
				if !sentinel.TestRole(c, "master") {
					return errors.New("Failed master role check")
				}
				return nil
			}
			_, err := c.Do("PING")
			return err
		},
	}
}

func getConfigs(getter config.Getter) ([]*ShardConfig, error) {
	numShards := 1

	hostsConf := getter.GetString("redis_host")
	portsConf := getter.GetString("redis_port")
	urlsConf := getter.GetString("redis_url")
	masterNamesConf := getter.GetString("redis_master_name")
	sentinelsConf := getter.GetString("redis_sentinels")

	password := getter.GetString("redis_password")
	db := getter.GetString("redis_db")

	hosts := []string{}
	if hostsConf != "" {
		hosts = strings.Split(hostsConf, ",")
		if len(hosts) > numShards {
			numShards = len(hosts)
		}
	}

	ports := []string{}
	if portsConf != "" {
		ports = strings.Split(portsConf, ",")
		if len(ports) > numShards {
			numShards = len(ports)
		}
	}

	urls := []string{}
	if urlsConf != "" {
		urls = strings.Split(urlsConf, ",")
		if len(urls) > numShards {
			numShards = len(urls)
		}
	}

	masterNames := []string{}
	if masterNamesConf != "" {
		masterNames = strings.Split(masterNamesConf, ",")
		if len(masterNames) > numShards {
			numShards = len(masterNames)
		}
	}

	if masterNamesConf != "" && sentinelsConf == "" {
		return nil, fmt.Errorf("Provide at least one Sentinel address")
	}

	if masterNamesConf != "" && len(masterNames) < numShards {
		return nil, fmt.Errorf("Redis master name must be set for every Redis shard when Sentinel used")
	}

	sentinelAddrs := []string{}
	if sentinelsConf != "" {
		for _, addr := range strings.Split(sentinelsConf, ",") {
			addr := strings.TrimSpace(addr)
			if addr == "" {
				continue
			}
			if _, _, err := net.SplitHostPort(addr); err != nil {
				return nil, fmt.Errorf("Malformed Sentinel address: %s", addr)
			}
			sentinelAddrs = append(sentinelAddrs, addr)
		}
	}

	if len(hosts) <= 1 {
		newHosts := make([]string, numShards)
		for i := 0; i < numShards; i++ {
			if len(hosts) == 0 {
				newHosts[i] = ""
			} else {
				newHosts[i] = hosts[0]
			}
		}
		hosts = newHosts
	} else if len(hosts) != numShards {
		return nil, fmt.Errorf("Malformed sharding configuration: wrong number of redis hosts")
	}

	if len(ports) <= 1 {
		newPorts := make([]string, numShards)
		for i := 0; i < numShards; i++ {
			if len(ports) == 0 {
				newPorts[i] = ""
			} else {
				newPorts[i] = ports[0]
			}
		}
		ports = newPorts
	} else if len(ports) != numShards {
		return nil, fmt.Errorf("Malformed sharding configuration: wrong number of redis ports")
	}

	if len(urls) > 0 && len(urls) != numShards {
		return nil, fmt.Errorf("Malformed sharding configuration: wrong number of redis urls")
	}

	if len(masterNames) == 0 {
		newMasterNames := make([]string, numShards)
		for i := 0; i < numShards; i++ {
			newMasterNames[i] = ""
		}
		masterNames = newMasterNames
	}

	passwords := make([]string, numShards)
	for i := 0; i < numShards; i++ {
		passwords[i] = password
	}

	dbs := make([]string, numShards)
	for i := 0; i < numShards; i++ {
		dbs[i] = db
	}

	for i, confURL := range urls {
		if confURL == "" {
			continue
		}
		// If URL set then prefer it over other parameters.
		u, err := url.Parse(confURL)
		if err != nil {
			return nil, fmt.Errorf("%v", err)
		}
		if u.User != nil {
			var ok bool
			pass, ok := u.User.Password()
			if !ok {
				pass = ""
			}
			passwords[i] = pass
		}
		host, port, err := net.SplitHostPort(u.Host)
		if err != nil {
			return nil, fmt.Errorf("%v", err)
		}
		path := u.Path
		if path != "" {
			dbs[i] = path[1:]
		}
		hosts[i] = host
		ports[i] = port
	}

	var shardConfigs []*ShardConfig

	for i := 0; i < numShards; i++ {
		conf := &ShardConfig{
			Host:             hosts[i],
			Port:             ports[i],
			Password:         passwords[i],
			DB:               dbs[i],
			MasterName:       masterNames[i],
			SentinelAddrs:    sentinelAddrs,
			PoolSize:         getter.GetInt("redis_pool"),
			API:              getter.GetBool("redis_api"),
			NumAPIShards:     getter.GetInt("redis_api_num_shards"),
			Prefix:           getter.GetString("redis_prefix"),
			PubSubNumWorkers: getter.GetInt("redis_pubsub_num_workers"),
			ConnectTimeout:   time.Duration(getter.GetInt("redis_connect_timeout")) * time.Second,
			ReadTimeout:      time.Duration(getter.GetInt("redis_read_timeout")) * time.Second,
			WriteTimeout:     time.Duration(getter.GetInt("redis_write_timeout")) * time.Second,
		}
		shardConfigs = append(shardConfigs, conf)
	}

	return shardConfigs, nil
}

// Plugin returns Redis Engine.
func Plugin(n *node.Node, getter config.Getter) (engine.Engine, error) {
	configs, err := getConfigs(getter)
	if err != nil {
		return nil, err
	}
	return New(n, configs)
}

// New initializes Redis Engine.
func New(n *node.Node, configs []*ShardConfig) (*RedisEngine, error) {

	var shards []*Shard

	if len(configs) > 1 {
		logger.INFO.Printf("Redis sharding enabled: %d shards", len(configs))
	}

	for _, conf := range configs {
		shard, err := NewShard(n, conf)
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

// NewShard initializes new Redis shard.
func NewShard(n *node.Node, conf *ShardConfig) (*Shard, error) {
	shard := &Shard{
		node:              n,
		config:            conf,
		pool:              newPool(conf),
		api:               conf.API,
		numApiShards:      conf.NumAPIShards,
		pubScript:         redis.NewScript(2, pubScriptSource),
		addPresenceScript: redis.NewScript(2, addPresenceSource),
		remPresenceScript: redis.NewScript(2, remPresenceSource),
		presenceScript:    redis.NewScript(2, presenceSource),
		lpopManyScript:    redis.NewScript(1, lpopManySource),
	}
	shard.pubCh = make(chan pubRequest, RedisPublishChannelSize)
	shard.subCh = make(chan subRequest, RedisSubscribeChannelSize)
	shard.dataCh = make(chan dataRequest, RedisDataChannelSize)
	shard.messagePrefix = conf.Prefix + RedisMessageChannelPrefix
	shard.joinPrefix = conf.Prefix + RedisJoinChannelPrefix
	shard.leavePrefix = conf.Prefix + RedisLeaveChannelPrefix
	return shard, nil
}

func yesno(condition bool) string {
	if condition {
		return "yes"
	}
	return "no"
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

func (e *RedisEngine) shardIndex(channel string) int {
	if !e.sharding {
		return 0
	}
	return consistentIndex(channel, len(e.shards))
}

// Name returns name of engine.
func (e *RedisEngine) Name() string {
	return "Redis"
}

// Run runs engine after node initialized.
func (e *RedisEngine) Run() error {
	for _, shard := range e.shards {
		err := shard.Run()
		if err != nil {
			return err
		}
	}
	return nil
}

// PublishMessage - see engine interface description.
func (e *RedisEngine) PublishMessage(message *proto.Message, opts *proto.ChannelOptions) <-chan error {
	return e.shards[e.shardIndex(message.Channel)].PublishMessage(message, opts)
}

// PublishJoin - see engine interface description.
func (e *RedisEngine) PublishJoin(message *proto.JoinMessage, opts *proto.ChannelOptions) <-chan error {
	return e.shards[e.shardIndex(message.Channel)].PublishJoin(message, opts)
}

// PublishLeave - see engine interface description.
func (e *RedisEngine) PublishLeave(message *proto.LeaveMessage, opts *proto.ChannelOptions) <-chan error {
	return e.shards[e.shardIndex(message.Channel)].PublishLeave(message, opts)
}

// PublishAdmin- see engine interface description.
func (e *RedisEngine) PublishAdmin(message *proto.AdminMessage) <-chan error {
	return e.shards[0].PublishAdmin(message)
}

// PublishControl - see engine interface description.
func (e *RedisEngine) PublishControl(message *proto.ControlMessage) <-chan error {
	return e.shards[0].PublishControl(message)
}

// Subscribe - see engine interface description.
func (e *RedisEngine) Subscribe(ch string) error {
	return e.shards[e.shardIndex(ch)].Subscribe(ch)
}

// Unsubscribe - see engine interface description.
func (e *RedisEngine) Unsubscribe(ch string) error {
	return e.shards[e.shardIndex(ch)].Unsubscribe(ch)
}

// AddPresence - see engine interface description.
func (e *RedisEngine) AddPresence(ch string, uid string, info proto.ClientInfo, expire int) error {
	return e.shards[e.shardIndex(ch)].AddPresence(ch, uid, info, expire)
}

// RemovePresence - see engine interface description.
func (e *RedisEngine) RemovePresence(ch string, uid string) error {
	return e.shards[e.shardIndex(ch)].RemovePresence(ch, uid)
}

// Presence - see engine interface description.
func (e *RedisEngine) Presence(ch string) (map[string]proto.ClientInfo, error) {
	return e.shards[e.shardIndex(ch)].Presence(ch)
}

// History - see engine interface description.
func (e *RedisEngine) History(ch string, limit int) ([]proto.Message, error) {
	return e.shards[e.shardIndex(ch)].History(ch, limit)
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
	channels := make([]string, len(channelMap))
	j := 0
	for ch := range channelMap {
		channels[j] = ch
		j++
	}
	return channels, nil
}

// Run runs Redis shard.
func (e *Shard) Run() error {
	e.RLock()
	api := e.api
	e.RUnlock()
	go e.runForever(func() {
		e.runPublishPipeline()
	})
	go e.runForever(func() {
		e.runPubSub()
	})
	go e.runForever(func() {
		e.runDataPipeline()
	})
	if api {
		e.runAPI()
	}
	return nil
}

// Shutdown shuts down Redis engine.
func (e *RedisEngine) Shutdown() error {
	return errors.New("Shutdown not implemented")
}

// runForever simple keeps another function running indefinitely
// the reason this loop is not inside the function itself is so that defer
// can be used to cleanup nicely (defers only run at function return not end of block scope)
func (e *Shard) runForever(fn func()) {
	shutdownCh := e.node.NotifyShutdown()
	for {
		select {
		case <-shutdownCh:
			return
		default:
			fn()
		}
		// Sleep for a while to prevent busy loop when reconnecting to Redis.
		time.Sleep(300 * time.Millisecond)
	}
}

func (e *Shard) blpopTimeout() int {
	var timeout int
	e.RLock()
	readTimeout := e.config.ReadTimeout
	e.RUnlock()
	if readTimeout == 0 {
		// No read timeout - we can block forever in BLPOP.
		timeout = 0
	} else {
		timeout = int(readTimeout.Seconds() / 2)
		if timeout == 0 {
			timeout = 1
		}
	}
	return timeout
}

func (e *Shard) runAPIWorker(queue string) {
	logger.DEBUG.Printf("Start Redis API worker for queue %s", queue)
	shutdownCh := e.node.NotifyShutdown()

	conn := e.pool.Get()
	defer conn.Close()

	err := e.lpopManyScript.Load(conn)
	if err != nil {
		logger.ERROR.Println(err)
		return
	}

	// Start with BLPOP.
	blockingPop := true

	for {
		select {
		case <-shutdownCh:
			return
		default:
			if blockingPop {
				// BLPOP with timeout, which must be less than connection ReadTimeout to prevent
				// timeout errors. Below we handle situation when BLPOP block timeout fired
				// (ErrNil returned) and call BLPOP again.
				reply, err := conn.Do("BLPOP", queue, e.blpopTimeout())
				if err != nil {
					logger.ERROR.Println(err)
					return
				}

				values, err := redis.Values(reply, nil)
				if err != nil {
					if err == redis.ErrNil {
						continue
					}
					logger.ERROR.Println(err)
					return
				}
				if len(values) != 2 {
					logger.ERROR.Println("Wrong reply from Redis in BLPOP - expecting 2 values")
					continue
				}
				// values[0] is a name of API queue, as we listen only one queue
				// here - we don't need it.
				body, okVal := values[1].([]byte)
				if !okVal {
					logger.ERROR.Println("Wrong reply from Redis in BLPOP - can not convert value to slice of bytes")
					continue
				}
				err = e.processAPIData(body)
				if err != nil {
					logger.ERROR.Printf("Error processing Redis API data: %v", err)
				}
				// There could be a lot of messages, switch to fast retreiving using
				// lua script with LRANGE/LTRIM operations.
				blockingPop = false
			} else {
				err := e.lpopManyScript.SendHash(conn, queue, 64)
				if err != nil {
					logger.ERROR.Println(err)
					return
				}
				err = conn.Flush()
				if err != nil {
					return
				}
				values, err := redis.Values(conn.Receive())
				if err != nil {
					logger.ERROR.Println(err)
					return
				}
				if len(values) == 0 {
					// No items in queue, switch back to BLPOP.
					blockingPop = true
					continue
				}
				for _, val := range values {
					data, ok := val.([]byte)
					if !ok {
						logger.ERROR.Println("Wrong reply in API lua script response - can not convert value to slice of bytes")
						continue
					}
					err := e.processAPIData(data)
					if err != nil {
						logger.ERROR.Printf("Error processing Redis API data: %v", err)
					}
				}
			}
		}
	}
}

func (e *Shard) runAPI() {
	queues := make(map[string]bool)
	queues[e.getAPIQueueKey()] = true
	for i := 0; i < e.numApiShards; i++ {
		queues[e.getAPIShardQueueKey(i)] = true
	}
	for name := range queues {
		func(name string) {
			go e.runForever(func() {
				e.runAPIWorker(name)
			})
		}(name)
	}
}

// fillSubBatch attempts to read items from a subRequest channel and append them to split
// until it either hits maxSize or would have to block. If batch is empty and chan is empty then
// batch might end up being zero length.
func fillSubBatch(ch <-chan subRequest, batch *[]subRequest, maxSize int) {
	for len(*batch) < maxSize {
		select {
		case req := <-ch:
			*batch = append(*batch, req)
		default:
			return
		}
	}
}

func (e *Shard) runPubSub() {

	e.RLock()
	numWorkers := e.config.PubSubNumWorkers
	e.RUnlock()
	if numWorkers == 0 {
		numWorkers = runtime.NumCPU()
	}

	logger.DEBUG.Printf("Running Redis PUB/SUB, num workers: %d", numWorkers)

	conn := redis.PubSubConn{Conn: e.pool.Get()}
	defer conn.Close()

	controlChannel := e.controlChannelID()
	adminChannel := e.adminChannelID()
	pingChannel := e.pingChannelID()

	done := make(chan struct{})
	defer close(done)

	// Run subscriber goroutine.
	go func() {
		logger.TRACE.Println("Starting RedisEngine Subscriber")

		defer func() {
			logger.TRACE.Println("Stopping RedisEngine Subscriber")
		}()
		for {
			select {
			case <-done:
				return
			case r := <-e.subCh:
				// Initialize batch with subRequest just read from channel.
				batch := []subRequest{r}

				// Try to gather as many others as we can without waiting.
				fillSubBatch(e.subCh, &batch, RedisSubscribeBatchLimit)

				// Keep unique ChannelID presented in batch to subscribe.
				subs := map[ChannelID]struct{}{}

				// Keep unique ChannelID presented in batch to unsubscribe.
				unsubs := map[ChannelID]struct{}{}

				for i := range batch {
					ch := batch[i].channel

					if batch[i].subscribe {
						subs[ch] = struct{}{}
						if _, ok := unsubs[ch]; ok {
							// If we already have more recent unsubscribe to the same channel in
							// batch - we don't need to send it to Redis.
							delete(unsubs, ch)
						}
					} else {
						unsubs[ch] = struct{}{}
						if _, ok := subs[ch]; ok {
							// If we already have more recent subscribe to the same channel in
							// batch - we don't need to send it to Redis.
							delete(subs, ch)
						}
					}
				}

				// Prepare arguments for Subscribe/Unsubscribe operations which accept slice
				// of interface{}.
				subChIDs := make([]interface{}, len(subs))
				i := 0
				for ch := range subs {
					subChIDs[i] = ch
					i++
				}

				i = 0
				unsubChIDs := make([]interface{}, len(unsubs))
				for ch := range unsubs {
					unsubChIDs[i] = ch
					i++
				}

				if len(subChIDs) > 0 {
					err := conn.Subscribe(subChIDs...)
					if err != nil {
						logger.ERROR.Printf("RedisEngine Subscriber error: %v\n", err)

						// Resolve all requests in batch with err.
						for i := range batch {
							batch[i].done(err)
						}
						// Close conn, this should cause Receive to return with err below
						// and whole runPubSub method to restart.
						conn.Close()
						return
					}
				}
				// Subscribe requests in batch succeeded.
				for i := range batch {
					if batch[i].subscribe {
						batch[i].done(nil)
					}
				}
				if len(unsubChIDs) > 0 {
					err := conn.Unsubscribe(unsubChIDs...)
					if err != nil {
						logger.ERROR.Printf("RedisEngine Unsubscriber error: %v\n", err)
						// Here we should only resolve with error unsubscribe requests.
						// Subscribe requests have been already resolved without error above.
						for i := range batch {
							if !batch[i].subscribe {
								batch[i].done(err)
							}
						}
						// Close conn, this should cause Receive to return with err below
						// and whole runPubSub method to restart
						conn.Close()
						return
					}
				}
				// Unsubscribe requests in batch succeded.
				for i := range batch {
					if !batch[i].subscribe {
						batch[i].done(nil)
					}
				}
			}
		}
	}()

	// Subscribe to channels we need in bulk.
	// We don't care if they fail since conn will be closed and we'll retry
	// if they do anyway.
	// This saves a lot of allocating of pointless chans...
	r := newSubRequest(controlChannel, true, false)
	e.subCh <- r
	r = newSubRequest(adminChannel, true, false)
	e.subCh <- r
	r = newSubRequest(pingChannel, true, false)
	e.subCh <- r
	for _, ch := range e.node.ClientHub().Channels() {
		e.subCh <- newSubRequest(e.messageChannelID(ch), true, false)
		e.subCh <- newSubRequest(e.joinChannelID(ch), true, false)
		e.subCh <- newSubRequest(e.leaveChannelID(ch), true, false)
	}

	// Run workers to spread received message processing work over worker goroutines.
	workers := make(map[int]chan redis.Message)
	for i := 0; i < numWorkers; i++ {
		workerCh := make(chan redis.Message, RedisPubSubWorkerChannelSize)
		workers[i] = workerCh
		go func(ch chan redis.Message) {
			for {
				select {
				case <-done:
					return
				case n := <-ch:
					chID := ChannelID(n.Channel)
					if len(n.Data) == 0 {
						continue
					}
					switch chID {
					case controlChannel:
						var message proto.ControlMessage
						err := message.Unmarshal(n.Data)
						if err != nil {
							logger.ERROR.Println(err)
							continue
						}
						e.node.ControlMsg(&message)
					case adminChannel:
						var message proto.AdminMessage
						err := message.Unmarshal(n.Data)
						if err != nil {
							logger.ERROR.Println(err)
							continue
						}
						e.node.AdminMsg(&message)
					case pingChannel:
						// Do nothing - this message just maintains connection open.
					default:
						err := e.handleRedisClientMessage(chID, n.Data)
						if err != nil {
							logger.ERROR.Println(err)
							continue
						}
					}
				}
			}
		}(workerCh)
	}

	for {
		switch n := conn.Receive().(type) {
		case redis.Message:
			// Add message to worker channel preserving message order - i.e. messages from
			// the same channel will be processed in the same worker.
			workers[index(n.Channel, numWorkers)] <- n
		case redis.Subscription:
		case error:
			logger.ERROR.Printf("Redis receiver error: %v\n", n)
			return
		}
	}
}

func (e *Shard) handleRedisClientMessage(chID ChannelID, data []byte) error {
	msgType := e.typeFromChannelID(chID)
	switch msgType {
	case "message":
		var message proto.Message
		err := message.Unmarshal(data)
		if err != nil {
			return err
		}
		e.node.ClientMsg(&message)
	case "join":
		var message proto.JoinMessage
		err := message.Unmarshal(data)
		if err != nil {
			return err
		}
		e.node.JoinMsg(&message)
	case "leave":
		var message proto.LeaveMessage
		err := message.Unmarshal(data)
		if err != nil {
			return err
		}
		e.node.LeaveMsg(&message)
	default:
	}
	return nil
}

type pubRequest struct {
	channel    ChannelID
	message    []byte
	historyKey string
	touchKey   string
	opts       *proto.ChannelOptions
	err        *chan error
}

func (pr *pubRequest) done(err error) {
	*(pr.err) <- err
}

func (pr *pubRequest) result() error {
	return <-*(pr.err)
}

func fillPublishBatch(ch chan pubRequest, prs *[]pubRequest) {
	for len(*prs) < RedisPublishBatchLimit {
		select {
		case pr := <-ch:
			*prs = append(*prs, pr)
		default:
			return
		}
	}
}

func (e *Shard) runPublishPipeline() {
	conn := e.pool.Get()

	err := e.pubScript.Load(conn)
	if err != nil {
		logger.ERROR.Println(err)
		// Can not proceed if script has not been loaded - because we use EVALSHA command for
		// publishing with history.
		conn.Close()
		return
	}

	conn.Close()

	var prs []pubRequest

	e.RLock()
	pingTimeout := e.config.ReadTimeout / 3
	e.RUnlock()

	for {
		select {
		case <-time.After(pingTimeout):
			// We have to PUBLISH pings into connection to prevent connection close after read timeout.
			// In our case it's important to maintain PUB/SUB receiver connection alive to prevent
			// resubscribing on all our subscriptions again and again.
			conn := e.pool.Get()
			err := conn.Send("PUBLISH", e.pingChannelID(), nil)
			if err != nil {
				logger.ERROR.Printf("Error publish ping: %v", err)
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
				logger.ERROR.Printf("Error flushing publish pipeline: %v", err)
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

func (e *Shard) runDataPipeline() {

	conn := e.pool.Get()

	err := e.addPresenceScript.Load(conn)
	if err != nil {
		logger.ERROR.Println(err)
		// Can not proceed if script has not been loaded.
		conn.Close()
		return
	}

	err = e.presenceScript.Load(conn)
	if err != nil {
		logger.ERROR.Println(err)
		// Can not proceed if script has not been loaded.
		conn.Close()
		return
	}

	err = e.remPresenceScript.Load(conn)
	if err != nil {
		logger.ERROR.Println(err)
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
			fillDataBatch(e.dataCh, &drs, RedisDataChannelSize)

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
				logger.ERROR.Printf("Error flushing publish pipeline: %v", err)
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

func (e *Shard) messageChannelID(ch string) ChannelID {
	return ChannelID(e.messagePrefix + string(ch))
}

func (e *Shard) joinChannelID(ch string) ChannelID {
	return ChannelID(e.joinPrefix + string(ch))
}

func (e *Shard) leaveChannelID(ch string) ChannelID {
	return ChannelID(e.leavePrefix + string(ch))
}

func (e *Shard) typeFromChannelID(chID ChannelID) string {
	if strings.HasPrefix(string(chID), e.messagePrefix) {
		return "message"
	} else if strings.HasPrefix(string(chID), e.joinPrefix) {
		return "join"
	} else if strings.HasPrefix(string(chID), e.leavePrefix) {
		return "leave"
	} else {
		return "unknown"
	}
}

// PublishMessage - see engine interface description.
func (e *Shard) PublishMessage(message *proto.Message, opts *proto.ChannelOptions) <-chan error {
	ch := message.Channel

	eChan := make(chan error, 1)

	byteMessage, err := message.Marshal()
	if err != nil {
		eChan <- err
		return eChan
	}

	chID := e.messageChannelID(ch)

	if opts != nil && opts.HistorySize > 0 && opts.HistoryLifetime > 0 {
		pr := pubRequest{
			channel:    chID,
			message:    byteMessage,
			historyKey: e.getHistoryKey(chID),
			touchKey:   e.getHistoryTouchKey(chID),
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
func (e *Shard) PublishJoin(message *proto.JoinMessage, opts *proto.ChannelOptions) <-chan error {
	ch := message.Channel

	eChan := make(chan error, 1)

	byteMessage, err := message.Marshal()
	if err != nil {
		eChan <- err
		return eChan
	}

	chID := e.joinChannelID(ch)

	pr := pubRequest{
		channel: chID,
		message: byteMessage,
		err:     &eChan,
	}
	e.pubCh <- pr
	return eChan
}

// PublishLeave - see engine interface description.
func (e *Shard) PublishLeave(message *proto.LeaveMessage, opts *proto.ChannelOptions) <-chan error {
	ch := message.Channel

	eChan := make(chan error, 1)

	byteMessage, err := message.Marshal()
	if err != nil {
		eChan <- err
		return eChan
	}

	chID := e.leaveChannelID(ch)

	pr := pubRequest{
		channel: chID,
		message: byteMessage,
		err:     &eChan,
	}
	e.pubCh <- pr
	return eChan
}

// PublishControl - see engine interface description.
func (e *Shard) PublishControl(message *proto.ControlMessage) <-chan error {
	eChan := make(chan error, 1)

	byteMessage, err := message.Marshal()
	if err != nil {
		eChan <- err
		return eChan
	}

	chID := e.controlChannelID()

	pr := pubRequest{
		channel: chID,
		message: byteMessage,
		err:     &eChan,
	}
	e.pubCh <- pr
	return eChan
}

// PublishAdmin - see engine interface description.
func (e *Shard) PublishAdmin(message *proto.AdminMessage) <-chan error {
	eChan := make(chan error, 1)

	byteMessage, err := message.Marshal()
	if err != nil {
		eChan <- err
		return eChan
	}

	chID := e.adminChannelID()

	pr := pubRequest{
		channel: chID,
		message: byteMessage,
		err:     &eChan,
	}
	e.pubCh <- pr
	return eChan
}

// Subscribe - see engine interface description.
func (e *Shard) Subscribe(ch string) error {
	logger.TRACE.Println("Subscribe node on channel", ch)
	r := newSubRequest(e.joinChannelID(ch), true, false)
	e.subCh <- r
	r = newSubRequest(e.leaveChannelID(ch), true, false)
	e.subCh <- r
	r = newSubRequest(e.messageChannelID(ch), true, true)
	e.subCh <- r
	return r.result()
}

// Unsubscribe - see engine interface description.
func (e *Shard) Unsubscribe(ch string) error {
	logger.TRACE.Println("Unsubscribe node from channel", ch)
	r := newSubRequest(e.joinChannelID(ch), false, false)
	e.subCh <- r
	r = newSubRequest(e.leaveChannelID(ch), false, false)
	e.subCh <- r
	r = newSubRequest(e.messageChannelID(ch), false, true)
	e.subCh <- r
	if chOpts, err := e.node.ChannelOpts(ch); err == nil && chOpts.HistoryDropInactive {
		// Waiting for response here is not actually required. But this seems
		// semantically correct and allows avoid races in drop inactive tests.
		// It does not seem a big bottleneck for real usage but can be tuned in
		// future if we find any problems with it.
		dr := newDataRequest(dataOpHistoryTouch, []interface{}{e.getHistoryTouchKey(e.messageChannelID(ch)), chOpts.HistoryLifetime, ""}, true)
		e.dataCh <- dr
		dr.result()
	}
	return r.result()
}

func (e *Shard) getAPIQueueKey() string {
	return e.config.Prefix + RedisAPIKeySuffix
}

func (e *Shard) getAPIShardQueueKey(shardNum int) string {
	apiKey := e.getAPIQueueKey()
	return fmt.Sprintf("%s.%d", apiKey, shardNum)
}

func (e *Shard) controlChannelID() ChannelID {
	return ChannelID(e.config.Prefix + RedisControlChannelSuffix)
}

func (e *Shard) pingChannelID() ChannelID {
	return ChannelID(e.config.Prefix + RedisPingChannelSuffix)
}

func (e *Shard) adminChannelID() ChannelID {
	return ChannelID(e.config.Prefix + RedisAdminChannelSuffix)
}

func (e *Shard) getHashKey(chID ChannelID) string {
	return e.config.Prefix + ".presence.hash." + string(chID)
}

func (e *Shard) getSetKey(chID ChannelID) string {
	return e.config.Prefix + ".presence.set." + string(chID)
}

func (e *Shard) getHistoryKey(chID ChannelID) string {
	return e.config.Prefix + ".history.list." + string(chID)
}

func (e *Shard) getHistoryTouchKey(chID ChannelID) string {
	return e.config.Prefix + ".history.touch." + string(chID)
}

// AddPresence - see engine interface description.
func (e *Shard) AddPresence(ch string, uid string, info proto.ClientInfo, expire int) error {
	chID := e.messageChannelID(ch)
	infoJSON, err := info.Marshal()
	if err != nil {
		return err
	}
	expireAt := time.Now().Unix() + int64(expire)
	hashKey := e.getHashKey(chID)
	setKey := e.getSetKey(chID)
	dr := newDataRequest(dataOpAddPresence, []interface{}{setKey, hashKey, expire, expireAt, uid, infoJSON}, true)
	e.dataCh <- dr
	resp := dr.result()
	return resp.err
}

// RemovePresence - see engine interface description.
func (e *Shard) RemovePresence(ch string, uid string) error {
	chID := e.messageChannelID(ch)
	hashKey := e.getHashKey(chID)
	setKey := e.getSetKey(chID)
	dr := newDataRequest(dataOpRemovePresence, []interface{}{setKey, hashKey, uid}, true)
	e.dataCh <- dr
	resp := dr.result()
	return resp.err
}

// Presence - see engine interface description.
func (e *Shard) Presence(ch string) (map[string]proto.ClientInfo, error) {
	chID := e.messageChannelID(ch)
	hashKey := e.getHashKey(chID)
	setKey := e.getSetKey(chID)
	now := int(time.Now().Unix())
	dr := newDataRequest(dataOpPresence, []interface{}{setKey, hashKey, now}, true)
	e.dataCh <- dr
	resp := dr.result()
	if resp.err != nil {
		return nil, resp.err
	}
	return mapStringClientInfo(resp.reply, nil)
}

// History - see engine interface description.
func (e *Shard) History(ch string, limit int) ([]proto.Message, error) {
	chID := e.messageChannelID(ch)
	var rangeBound int = -1
	if limit > 0 {
		rangeBound = limit - 1 // Redis includes last index into result
	}
	historyKey := e.getHistoryKey(chID)
	dr := newDataRequest(dataOpHistory, []interface{}{historyKey, 0, rangeBound}, true)
	e.dataCh <- dr
	resp := dr.result()
	if resp.err != nil {
		return nil, resp.err
	}
	return sliceOfMessages(resp.reply, nil)
}

// Channels - see engine interface description.
// Requires Redis >= 2.8.0 (http://redis.io/commands/pubsub)
func (e *Shard) Channels() ([]string, error) {
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
			return nil, errors.New("error getting ChannelID value")
		}
		chID := ChannelID(value)
		channels = append(channels, string(string(chID)[len(e.messagePrefix):]))
	}
	return channels, nil
}

// Deprecated: will be removed in future releases. See Centrifugo Redis API docs for new format.
type redisAPIRequest struct {
	Data []proto.APICommand
}

func (e *Shard) processAPIData(data []byte) error {
	cmd, err := apiv1.APICommandFromJSON(data)
	if err != nil {
		return err
	}
	// Support deprecated usage of old Redis API format.
	if cmd.Method == "" {
		var req redisAPIRequest
		err := json.Unmarshal(data, &req)
		if err != nil {
			return err
		}
		for _, command := range req.Data {
			err := apiCmd(e.node, command)
			if err != nil {
				logger.ERROR.Println(err)
			}
		}
	}
	return apiCmd(e.node, cmd)
}

func apiCmd(n *node.Node, cmd proto.APICommand) error {

	var err error

	method := cmd.Method
	params := cmd.Params

	switch method {
	case "publish":
		var cmd proto.PublishAPICommand
		err = json.Unmarshal(params, &cmd)
		if err != nil {
			return err
		}
		apiv1.PublishCmdAsync(n, cmd)
	case "broadcast":
		var cmd proto.BroadcastAPICommand
		err = json.Unmarshal(params, &cmd)
		if err != nil {
			return err
		}
		apiv1.BroadcastCmdAsync(n, cmd)
	case "unsubscribe":
		var cmd proto.UnsubscribeAPICommand
		err = json.Unmarshal(params, &cmd)
		if err != nil {
			return err
		}
		_, err = apiv1.UnsubscribeCmd(n, cmd)
	case "disconnect":
		var cmd proto.DisconnectAPICommand
		err = json.Unmarshal(params, &cmd)
		if err != nil {
			return err
		}
		_, err = apiv1.DisconnectCmd(n, cmd)
	default:
		return nil
	}
	return err
}
