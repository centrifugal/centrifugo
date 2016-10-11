package engineredis

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/FZambia/go-logger"
	"github.com/FZambia/go-sentinel"
	"github.com/centrifugal/centrifugo/libcentrifugo/engine"
	"github.com/centrifugal/centrifugo/libcentrifugo/node"
	"github.com/centrifugal/centrifugo/libcentrifugo/plugin"
	"github.com/centrifugal/centrifugo/libcentrifugo/proto"
	"github.com/garyburd/redigo/redis"
)

func init() {
	plugin.RegisterEngine("redis", RedisEnginePlugin)
	plugin.RegisterConfigurator("redis", RedisEngineConfigure)
}

func RedisEngineConfigure(setter plugin.ConfigSetter) error {

	setter.SetDefault("redis_prefix", "centrifugo")

	setter.SetDefault("redis_connect_timeout", 1)
	setter.SetDefault("redis_read_timeout", 10) // Must be greater than minimal periodic node ping interval.
	setter.SetDefault("redis_write_timeout", 1)

	setter.StringFlag("redis_host", "", "127.0.0.1", "redis host (Redis engine)")
	setter.StringFlag("redis_port", "", "6379", "redis port (Redis engine)")
	setter.StringFlag("redis_password", "", "", "redis auth password (Redis engine)")
	setter.StringFlag("redis_db", "", "0", "redis database (Redis engine)")
	setter.StringFlag("redis_url", "", "", "redis connection URL (Redis engine)")
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
	// uses to synchronize subscribe/unsubscribe. It allows for effective batching during bulk re-subscriptions,
	// and allows large volume of incoming subscriptions to not block when PubSub connection is reconnecting.
	// Two channels of this size will be allocated, one for Subscribe and one for Unsubscribe
	RedisSubscribeChannelSize = 4096
	// Maximum number of channels to include in a single subscribe call. Redis documentation doesn't specify a
	// maximum allowed but we think it probably makes sense to keep a sane limit given how many subscriptions a single
	// Centrifugo instance might be handling
	RedisSubscribeBatchLimit = 2048
	// RedisPublishChannelSize is the size for the internal buffered channel RedisEngine
	// uses to collect publish requests.
	RedisPublishChannelSize = 1024
	// RedisPublishBatchLimit is a maximum limit of publish requests one batched publish
	// operation can contain.
	RedisPublishBatchLimit = 2048
)

type (
	// ChannelID is unique channel identificator in Redis.
	ChannelID string
)

const (
	RedisAPIKeySuffix         = ".api"
	RedisControlChannelSuffix = ".control"
	RedisAdminChannelSuffix   = ".admin"
	RedisMessageChannelPrefix = ".message."
	RedisJoinChannelPrefix    = ".join."
	RedisLeaveChannelPrefix   = ".leave."
)

// RedisEngine uses Redis datastructures and PUB/SUB to manage Centrifugo logic.
// This engine allows to scale Centrifugo - you can run several Centrifugo instances
// connected to the same Redis and load balance clients between instances.
type RedisEngine struct {
	sync.RWMutex
	node              node.Node
	config            *RedisEngineConfig
	pool              *redis.Pool
	api               bool
	numApiShards      int
	subCh             chan subRequest
	unSubCh           chan subRequest
	pubCh             chan *pubRequest
	pubScript         *redis.Script
	addPresenceScript *redis.Script
	remPresenceScript *redis.Script
	presenceScript    *redis.Script
	messagePrefix     string
	joinPrefix        string
	leavePrefix       string
}

// RedisEngineConfig is struct with Redis Engine options.
type RedisEngineConfig struct {
	// Host is Redis server host.
	Host string
	// Port is Redis server port.
	Port string
	// Password is password to use when connecting to Redis database. If empty then password not used.
	Password string
	// DB is Redis database number as string. If empty then database 0 used.
	DB string
	// URL to redis server in format redis://:password@hostname:port/db_number
	URL string
	// PoolSize is a size of Redis connection pool.
	PoolSize int
	// API enables listening for API queues to publish API commands into Centrifugo via pushing
	// commands into Redis queue.
	API bool
	// NumAPIShards is a number of sharded API queues in Redis to increase volume of commands
	// (most probably publish) that Centrifugo instance can process.
	NumAPIShards int

	// MasterName is a name of Redis instance master Sentinel monitors.
	MasterName string
	// SentinelAddrs is a slice of Sentinel addresses.
	SentinelAddrs []string

	// Prefix to use before every channel name and key in Redis.
	Prefix string

	// Timeout on read operations. Note that at moment it should be greater than node
	// ping interval in order to prevent timing out Pubsub connection's Receive call.
	ReadTimeout time.Duration
	// Timeout on write operations
	WriteTimeout time.Duration
	// Timeout on connect operation
	ConnectTimeout time.Duration
}

// subRequest is an internal request to subscribe or unsubscribe from one or more channels
type subRequest struct {
	Channel ChannelID
	err     *chan error
}

// newSubRequest creates a new request to subscribe or unsubscribe form a channel.
// If the caller cares about response they should set wantResponse and then call
// result() on the request once it has been pushed to the appropriate chan.
func newSubRequest(chID ChannelID, wantResponse bool) subRequest {
	r := subRequest{
		Channel: chID,
	}
	if wantResponse {
		eChan := make(chan error)
		r.err = &eChan
	}
	return r
}

func (sr *subRequest) done(err error) {
	if sr.err == nil {
		return
	}
	*(sr.err) <- err
}

func (sr *subRequest) result() error {
	if sr.err == nil {
		// No waiting, as caller didn't care about response
		return nil
	}
	return <-*(sr.err)
}

func newPool(conf *RedisEngineConfig) *redis.Pool {

	host := conf.Host
	port := conf.Port
	password := conf.Password

	db := "0"
	if conf.DB != "" {
		db = conf.DB
	}

	// If URL set then prefer it over other parameters.
	if conf.URL != "" {
		u, err := url.Parse(conf.URL)
		if err != nil {
			logger.FATAL.Fatalln(err)
		}
		if u.User != nil {
			var ok bool
			password, ok = u.User.Password()
			if !ok {
				password = ""
			}
		}
		host, port, err = net.SplitHostPort(u.Host)
		if err != nil {
			logger.FATAL.Fatalln(err)
		}
		path := u.Path
		if path != "" {
			db = path[1:]
		}
	}

	serverAddr := net.JoinHostPort(host, port)
	useSentinel := conf.MasterName != "" && len(conf.SentinelAddrs) > 0

	usingPassword := yesno(password != "")
	apiEnabled := yesno(conf.API)
	var shardsSuffix string
	if conf.API {
		shardsSuffix = fmt.Sprintf(", num shard queues: %d", conf.NumAPIShards)
	}
	if !useSentinel {
		logger.INFO.Printf("Redis engine: %s/%s, pool: %d, using password: %s, API enabled: %s%s\n", serverAddr, db, conf.PoolSize, usingPassword, apiEnabled, shardsSuffix)
	} else {
		logger.INFO.Printf("Redis engine: Sentinel for name: %s, db: %s, pool: %d, using password: %s, API enabled: %s%s\n", conf.MasterName, db, conf.PoolSize, usingPassword, apiEnabled, shardsSuffix)
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

func getRedisEngineConfig(getter plugin.ConfigGetter) *RedisEngineConfig {
	masterName := getter.GetString("redis_master_name")
	sentinels := getter.GetString("redis_sentinels")
	if masterName != "" && sentinels == "" {
		logger.FATAL.Fatalf("Provide at least one Sentinel address")
	}

	sentinelAddrs := []string{}
	if sentinels != "" {
		for _, addr := range strings.Split(sentinels, ",") {
			addr := strings.TrimSpace(addr)
			if addr == "" {
				continue
			}
			if _, _, err := net.SplitHostPort(addr); err != nil {
				logger.FATAL.Fatalf("Malformed Sentinel address: %s", addr)
			}
			sentinelAddrs = append(sentinelAddrs, addr)
		}
	}

	if len(sentinelAddrs) > 0 && masterName == "" {
		logger.FATAL.Fatalln("Redis master name required when Sentinel used")
	}

	conf := &RedisEngineConfig{
		Host:           getter.GetString("redis_host"),
		Port:           getter.GetString("redis_port"),
		Password:       getter.GetString("redis_password"),
		DB:             getter.GetString("redis_db"),
		URL:            getter.GetString("redis_url"),
		PoolSize:       getter.GetInt("redis_pool"),
		API:            getter.GetBool("redis_api"),
		NumAPIShards:   getter.GetInt("redis_api_num_shards"),
		Prefix:         getter.GetString("redis_prefix"),
		MasterName:     masterName,
		SentinelAddrs:  sentinelAddrs,
		ConnectTimeout: time.Duration(getter.GetInt("redis_connect_timeout")) * time.Second,
		ReadTimeout:    time.Duration(getter.GetInt("redis_read_timeout")) * time.Second,
		WriteTimeout:   time.Duration(getter.GetInt("redis_write_timeout")) * time.Second,
	}

	return conf
}

func RedisEnginePlugin(n node.Node, getter plugin.ConfigGetter) (engine.Engine, error) {
	conf := getRedisEngineConfig(getter)
	return NewRedisEngine(n, conf)
}

// NewRedisEngine initializes Redis Engine.
func NewRedisEngine(n node.Node, conf *RedisEngineConfig) (engine.Engine, error) {
	e := &RedisEngine{
		node:              n,
		config:            conf,
		pool:              newPool(conf),
		api:               conf.API,
		numApiShards:      conf.NumAPIShards,
		pubScript:         redis.NewScript(1, pubScriptSource),
		addPresenceScript: redis.NewScript(2, addPresenceSource),
		remPresenceScript: redis.NewScript(2, remPresenceSource),
		presenceScript:    redis.NewScript(2, presenceSource),
	}
	e.pubCh = make(chan *pubRequest, RedisPublishChannelSize)
	e.subCh = make(chan subRequest, RedisSubscribeChannelSize)
	e.unSubCh = make(chan subRequest, RedisSubscribeChannelSize)
	e.messagePrefix = conf.Prefix + RedisMessageChannelPrefix
	e.joinPrefix = conf.Prefix + RedisJoinChannelPrefix
	e.leavePrefix = conf.Prefix + RedisLeaveChannelPrefix
	return e, nil
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
	// ARGV[1] - channel to publish message to
	// ARGV[2] - message payload
	// ARGV[3] - history size
	// ARGV[4] - history lifetime
	// ARGV[5] - history drop inactive flag - "0" or "1"
	pubScriptSource = `
local n = redis.call("publish", ARGV[1], ARGV[2])
local m = 0
if ARGV[5] == "1" and n == 0 then
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
)

func (e *RedisEngine) Name() string {
	return "Redis"
}

func (e *RedisEngine) Run() error {
	e.RLock()
	api := e.api
	e.RUnlock()
	go e.runForever(func() {
		e.runPublishPipeline()
	})
	go e.runForever(func() {
		e.runPubSub()
	})
	if api {
		go e.runForever(func() {
			e.runAPI()
		})
	}
	return nil
}

func (e *RedisEngine) Shutdown() error {
	return errors.New("Shutdown not implemented")
}

type redisAPIRequest struct {
	Data []proto.ApiCommand
}

// runForever simple keeps another function running indefinitely
// the reason this loop is not inside the function itself is so that defer
// can be used to cleanup nicely (defers only run at function return not end of block scope)
func (e *RedisEngine) runForever(fn func()) {
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

func (e *RedisEngine) blpopTimeout() int {
	var timeout int
	e.RLock()
	readTimeout := e.config.ReadTimeout
	e.RUnlock()
	if readTimeout == 0 {
		// No read timeout - we can block frever in BLOP.
		timeout = 0
	} else {
		timeout = int(readTimeout.Seconds() / 2)
		if timeout == 0 {
			timeout = 1
		}
	}
	return timeout
}

func (e *RedisEngine) runAPI() {
	conn := e.pool.Get()
	defer conn.Close()
	logger.TRACE.Println("Enter runAPI")
	defer logger.TRACE.Println("Return from runAPI")

	apiKey := e.getAPIQueueKey()

	done := make(chan struct{})
	defer close(done)

	popParams := []interface{}{apiKey}
	workQueues := make(map[string]chan []byte)
	workQueues[apiKey] = make(chan []byte, 256)

	for i := 0; i < e.numApiShards; i++ {
		queueKey := e.getAPIShardQueueKey(i)
		popParams = append(popParams, queueKey)
		workQueues[queueKey] = make(chan []byte, 256)
	}

	// Add timeout param, it must be less than connection ReadTimeout to prevent
	// timeout errors. Below we handle situation when BLPOP block timeout fired
	// (ErrNil returned) and call BLPOP again.
	popParams = append(popParams, e.blpopTimeout())

	// Start a worker for each queue
	for name, ch := range workQueues {
		go func(name string, in <-chan []byte) {
			logger.INFO.Printf("Starting worker for API queue %s", name)
			for {
				select {
				case body, ok := <-in:
					if !ok {
						return
					}
					var req redisAPIRequest
					err := json.Unmarshal(body, &req)
					if err != nil {
						logger.ERROR.Println(err)
						continue
					}
					for _, command := range req.Data {
						_, err := e.node.APICmd(command, nil)
						if err != nil {
							logger.ERROR.Println(err)
						}
					}
				case <-done:
					return
				}
			}
		}(name, ch)
	}

	for {
		reply, err := conn.Do("BLPOP", popParams...)
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

		queue, okQ := values[0].([]byte)
		body, okVal := values[1].([]byte)
		if !okQ || !okVal {
			logger.ERROR.Println("Wrong reply from Redis in BLPOP - can not convert value")
			continue
		}

		// Pick worker based on queue
		q, ok := workQueues[string(queue)]
		if !ok {
			logger.ERROR.Println("Got message from a queue we didn't even know about!")
			continue
		}

		q <- body
	}
}

// fillBatchFromChan attempts to read items from a subRequest channel and append them to split
// until it either hits maxSize or would have to block. If batch is empty and chan is empty then
// batch might end up being zero length.
func fillBatchFromChan(ch <-chan subRequest, batch *[]subRequest, chIDs *[]interface{}, maxSize int) {
	for len(*batch) < maxSize {
		select {
		case req := <-ch:
			*batch = append(*batch, req)
			*chIDs = append(*chIDs, req.Channel)
		default:
			return
		}
	}
}

func (e *RedisEngine) runPubSub() {
	conn := redis.PubSubConn{Conn: e.pool.Get()}
	defer conn.Close()
	logger.TRACE.Println("Enter runPubSub")
	defer logger.TRACE.Println("Return from runPubSub")

	controlChannel := e.controlChannelID()
	adminChannel := e.adminChannelID()

	done := make(chan struct{})
	defer close(done)

	// Run subscriber routine
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
				// Something to subscribe
				chIDs := []interface{}{r.Channel}
				batch := []subRequest{r}

				// Try to gather as many others as we can without waiting
				fillBatchFromChan(e.subCh, &batch, &chIDs, RedisSubscribeBatchLimit)
				// Send them all
				err := conn.Subscribe(chIDs...)
				if err != nil {
					// Subscribe error is fatal
					logger.ERROR.Printf("RedisEngine Subscriber error: %v\n", err)

					for i := range batch {
						batch[i].done(err)
					}

					// Close conn, this should cause Receive to return with err below
					// and whole runPubSub method to restart
					conn.Close()
					return
				}
				for i := range batch {
					batch[i].done(nil)
				}
			case r := <-e.unSubCh:
				// Something to subscribe
				chIDs := []interface{}{r.Channel}
				batch := []subRequest{r}
				// Try to gather as many others as we can without waiting
				fillBatchFromChan(e.unSubCh, &batch, &chIDs, RedisSubscribeBatchLimit)
				// Send them all
				err := conn.Unsubscribe(chIDs...)
				if err != nil {
					// Subscribe error is fatal
					logger.ERROR.Printf("RedisEngine Unsubscriber error: %v\n", err)

					for i := range batch {
						batch[i].done(err)
					}

					// Close conn, this should cause Receive to return with err below
					// and whole runPubSub method to restart
					conn.Close()
					return
				}
				for i := range batch {
					batch[i].done(nil)
				}
			}
		}
	}()

	// Subscribe to channels we need in bulk.
	// We don't care if they fail since conn will be closed and we'll retry
	// if they do anyway.
	// This saves a lot of allocating of pointless chans...
	r := newSubRequest(controlChannel, false)
	e.subCh <- r
	r = newSubRequest(adminChannel, false)
	e.subCh <- r
	for _, ch := range e.node.ClientHub().Channels() {
		e.subCh <- newSubRequest(e.messageChannelID(ch), false)
		e.subCh <- newSubRequest(e.joinChannelID(ch), false)
		e.subCh <- newSubRequest(e.leaveChannelID(ch), false)
	}

	for {
		switch n := conn.Receive().(type) {
		case redis.Message:
			chID := ChannelID(n.Channel)
			if len(n.Data) == 0 {
				continue
			}
			switch chID {
			case controlChannel:
				message, err := decodeEngineControlMessage(n.Data)
				if err != nil {
					logger.ERROR.Println(err)
					continue
				}
				e.node.EngineHandler().ControlMsg(message)
			case adminChannel:
				message, err := decodeEngineAdminMessage(n.Data)
				if err != nil {
					logger.ERROR.Println(err)
					continue
				}
				e.node.EngineHandler().AdminMsg(message)
			default:
				err := e.handleRedisClientMessage(chID, n.Data)
				if err != nil {
					logger.ERROR.Println(err)
					continue
				}
			}
		case redis.Subscription:
		case error:
			logger.ERROR.Printf("RedisEngine Receiver error: %v\n", n)
			return
		}
	}
}

func (e *RedisEngine) handleRedisClientMessage(chID ChannelID, data []byte) error {
	ch, msgType := e.channelFromChannelID(chID)
	switch msgType {
	case "message":
		message, err := decodeEngineClientMessage(data)
		if err != nil {
			return err
		}
		e.node.EngineHandler().ClientMsg(ch, message)
	case "join":
		message, err := decodeEngineJoinMessage(data)
		if err != nil {
			return err
		}
		e.node.EngineHandler().JoinMsg(ch, message)
	case "leave":
		message, err := decodeEngineLeaveMessage(data)
		if err != nil {
			return err
		}
		e.node.EngineHandler().LeaveMsg(ch, message)
	default:
	}
	return nil
}

type pubRequest struct {
	channel    ChannelID
	message    []byte
	historyKey string
	opts       *proto.ChannelOptions
	err        *chan error
}

func (pr *pubRequest) done(err error) {
	*(pr.err) <- err
}

func (pr *pubRequest) result() error {
	return <-*(pr.err)
}

func fillPublishBatch(ch chan *pubRequest, prs *[]*pubRequest) {
	for len(*prs) < RedisPublishBatchLimit {
		select {
		case pr := <-ch:
			*prs = append(*prs, pr)
		default:
			return
		}
	}
}

func (e *RedisEngine) runPublishPipeline() {

	conn := e.pool.Get()
	err := e.pubScript.Load(conn)
	if err != nil {
		logger.ERROR.Println(err)
		conn.Close()
		// Can not proceed if script has not been loaded - because we use EVALSHA command for
		// publishing with history.
		return
	}
	conn.Close()

	var prs []*pubRequest

	for {
		pr := <-e.pubCh
		prs = append(prs, pr)
		fillPublishBatch(e.pubCh, &prs)

		conn := e.pool.Get()

		for i := range prs {
			if prs[i].opts != nil && prs[i].opts.HistorySize > 0 && prs[i].opts.HistoryLifetime > 0 {
				e.pubScript.SendHash(conn, prs[i].historyKey, prs[i].channel, prs[i].message, prs[i].opts.HistorySize, prs[i].opts.HistoryLifetime, prs[i].opts.HistoryDropInactive)
			} else {
				conn.Send("PUBLISH", prs[i].channel, prs[i].message)
			}
		}
		err := conn.Flush()
		if err != nil {
			for i := range prs {
				prs[i].done(err)
			}
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
		conn.Close()
		if noScriptError {
			// Start this func from the beginning and LOAD missing script.
			return
		}
		prs = nil
	}
}

func (e *RedisEngine) messageChannelID(ch proto.Channel) ChannelID {
	return ChannelID(e.messagePrefix + string(ch))
}

func (e *RedisEngine) joinChannelID(ch proto.Channel) ChannelID {
	return ChannelID(e.joinPrefix + string(ch))
}

func (e *RedisEngine) leaveChannelID(ch proto.Channel) ChannelID {
	return ChannelID(e.leavePrefix + string(ch))
}

func (e *RedisEngine) channelFromChannelID(chID ChannelID) (proto.Channel, string) {
	if strings.HasPrefix(string(chID), e.messagePrefix) {
		return proto.Channel(strings.TrimPrefix(string(chID), e.messagePrefix)), "message"
	} else if strings.HasPrefix(string(chID), e.joinPrefix) {
		return proto.Channel(strings.TrimPrefix(string(chID), e.joinPrefix)), "join"
	} else if strings.HasPrefix(string(chID), e.leavePrefix) {
		return proto.Channel(strings.TrimPrefix(string(chID), e.leavePrefix)), "leave"
	} else {
		return proto.Channel(""), "unknown"
	}
}

func (e *RedisEngine) PublishMessage(ch proto.Channel, message *proto.Message, opts *proto.ChannelOptions) <-chan error {
	eChan := make(chan error, 1)

	byteMessage, err := encodeEngineClientMessage(message)
	if err != nil {
		eChan <- err
		return eChan
	}

	chID := e.messageChannelID(ch)

	if opts != nil && opts.HistorySize > 0 && opts.HistoryLifetime > 0 {
		pr := &pubRequest{
			channel:    chID,
			message:    byteMessage,
			historyKey: e.getHistoryKey(chID),
			opts:       opts,
			err:        &eChan,
		}
		e.pubCh <- pr
		return eChan
	}

	pr := &pubRequest{
		channel: chID,
		message: byteMessage,
		err:     &eChan,
	}
	e.pubCh <- pr
	return eChan
}

func (e *RedisEngine) PublishJoin(ch proto.Channel, message *proto.JoinMessage) <-chan error {
	eChan := make(chan error, 1)

	byteMessage, err := encodeEngineJoinMessage(message)
	if err != nil {
		eChan <- err
		return eChan
	}

	chID := e.joinChannelID(ch)

	pr := &pubRequest{
		channel: chID,
		message: byteMessage,
		err:     &eChan,
	}
	e.pubCh <- pr
	return eChan
}

func (e *RedisEngine) PublishLeave(ch proto.Channel, message *proto.LeaveMessage) <-chan error {
	eChan := make(chan error, 1)

	byteMessage, err := encodeEngineLeaveMessage(message)
	if err != nil {
		eChan <- err
		return eChan
	}

	chID := e.leaveChannelID(ch)

	pr := &pubRequest{
		channel: chID,
		message: byteMessage,
		err:     &eChan,
	}
	e.pubCh <- pr
	return eChan
}

func (e *RedisEngine) PublishControl(message *proto.ControlMessage) <-chan error {
	eChan := make(chan error, 1)

	byteMessage, err := encodeEngineControlMessage(message)
	if err != nil {
		eChan <- err
		return eChan
	}

	chID := e.controlChannelID()

	pr := &pubRequest{
		channel: chID,
		message: byteMessage,
		err:     &eChan,
	}
	e.pubCh <- pr
	return eChan
}

func (e *RedisEngine) PublishAdmin(message *proto.AdminMessage) <-chan error {
	eChan := make(chan error, 1)

	byteMessage, err := encodeEngineAdminMessage(message)
	if err != nil {
		eChan <- err
		return eChan
	}

	chID := e.adminChannelID()

	pr := &pubRequest{
		channel: chID,
		message: byteMessage,
		err:     &eChan,
	}
	e.pubCh <- pr
	return eChan
}

func (e *RedisEngine) Subscribe(ch proto.Channel) error {
	logger.TRACE.Println("Subscribe node on channel", ch)
	r := newSubRequest(e.joinChannelID(ch), false)
	e.subCh <- r
	r = newSubRequest(e.leaveChannelID(ch), false)
	e.subCh <- r
	r = newSubRequest(e.messageChannelID(ch), true)
	e.subCh <- r
	return r.result()
}

func (e *RedisEngine) Unsubscribe(ch proto.Channel) error {
	logger.TRACE.Println("Unsubscribe node from channel", ch)
	r := newSubRequest(e.joinChannelID(ch), false)
	e.unSubCh <- r
	r = newSubRequest(e.leaveChannelID(ch), false)
	e.unSubCh <- r
	r = newSubRequest(e.messageChannelID(ch), true)
	e.unSubCh <- r
	return r.result()
}

func (e *RedisEngine) getAPIQueueKey() string {
	return e.config.Prefix + RedisAPIKeySuffix
}

func (e *RedisEngine) getAPIShardQueueKey(shardNum int) string {
	apiKey := e.getAPIQueueKey()
	return fmt.Sprintf("%s.%d", apiKey, shardNum)
}

func (e *RedisEngine) controlChannelID() ChannelID {
	return ChannelID(e.config.Prefix + RedisControlChannelSuffix)
}

func (e *RedisEngine) adminChannelID() ChannelID {
	return ChannelID(e.config.Prefix + RedisAdminChannelSuffix)
}

func (e *RedisEngine) getHashKey(chID ChannelID) string {
	return e.config.Prefix + ".presence.hash." + string(chID)
}

func (e *RedisEngine) getSetKey(chID ChannelID) string {
	return e.config.Prefix + ".presence.set." + string(chID)
}

func (e *RedisEngine) getHistoryKey(chID ChannelID) string {
	return e.config.Prefix + ".history.list." + string(chID)
}

func (e *RedisEngine) AddPresence(ch proto.Channel, uid proto.ConnID, info proto.ClientInfo, expire int) error {
	chID := e.messageChannelID(ch)
	conn := e.pool.Get()
	defer conn.Close()
	infoJSON, err := info.Marshal()
	if err != nil {
		return err
	}
	expireAt := time.Now().Unix() + int64(expire)
	hashKey := e.getHashKey(chID)
	setKey := e.getSetKey(chID)
	_, err = e.addPresenceScript.Do(conn, setKey, hashKey, expire, expireAt, uid, infoJSON)
	return err
}

func (e *RedisEngine) RemovePresence(ch proto.Channel, uid proto.ConnID) error {
	chID := e.messageChannelID(ch)
	conn := e.pool.Get()
	defer conn.Close()
	hashKey := e.getHashKey(chID)
	setKey := e.getSetKey(chID)
	_, err := e.remPresenceScript.Do(conn, setKey, hashKey, uid)
	return err
}

func mapStringClientInfo(result interface{}, err error) (map[proto.ConnID]proto.ClientInfo, error) {
	values, err := redis.Values(result, err)
	if err != nil {
		return nil, err
	}
	if len(values)%2 != 0 {
		return nil, errors.New("mapStringClientInfo expects even number of values result")
	}
	m := make(map[proto.ConnID]proto.ClientInfo, len(values)/2)
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
		m[proto.ConnID(key)] = f
	}
	return m, nil
}

func (e *RedisEngine) Presence(ch proto.Channel) (map[proto.ConnID]proto.ClientInfo, error) {
	chID := e.messageChannelID(ch)
	conn := e.pool.Get()
	defer conn.Close()
	hashKey := e.getHashKey(chID)
	setKey := e.getSetKey(chID)
	now := int(time.Now().Unix())
	reply, err := e.presenceScript.Do(conn, setKey, hashKey, now)
	if err != nil {
		return nil, err
	}
	return mapStringClientInfo(reply, nil)
}

func sliceOfMessages(result interface{}, err error) ([]proto.Message, error) {
	values, err := redis.Values(result, err)
	if err != nil {
		return nil, err
	}
	msgs := make([]proto.Message, len(values))
	for i := 0; i < len(values); i++ {
		value, okValue := values[i].([]byte)
		if !okValue {
			return nil, errors.New("error getting Message value")
		}
		var m proto.Message
		err = m.Unmarshal(value)
		if err != nil {
			return nil, errors.New("can not unmarshal value to Message")
		}
		msgs[i] = m
	}
	return msgs, nil
}

func (e *RedisEngine) History(ch proto.Channel, limit int) ([]proto.Message, error) {
	chID := e.messageChannelID(ch)
	conn := e.pool.Get()
	defer conn.Close()
	var rangeBound int = -1
	if limit > 0 {
		rangeBound = limit - 1 // Redis includes last index into result
	}
	historyKey := e.getHistoryKey(chID)
	reply, err := conn.Do("LRANGE", historyKey, 0, rangeBound)
	if err != nil {
		logger.ERROR.Printf("%#v", err)
		return nil, err
	}
	return sliceOfMessages(reply, nil)
}

// Requires Redis >= 2.8.0 (http://redis.io/commands/pubsub)
func (e *RedisEngine) Channels() ([]proto.Channel, error) {
	conn := e.pool.Get()
	defer conn.Close()

	messagePrefix := e.messagePrefix
	joinPrefix := e.joinPrefix
	leavePrefix := e.leavePrefix

	reply, err := conn.Do("PUBSUB", "CHANNELS", messagePrefix+"*")
	if err != nil {
		return nil, err
	}

	values, err := redis.Values(reply, err)
	if err != nil {
		return nil, err
	}

	channels := make([]proto.Channel, 0, len(values))
	for i := 0; i < len(values); i++ {
		value, okValue := values[i].([]byte)
		if !okValue {
			return nil, errors.New("error getting ChannelID value")
		}
		chID := ChannelID(value)
		if !strings.HasPrefix(string(chID), joinPrefix) && !strings.HasPrefix(string(chID), leavePrefix) {
			channels = append(channels, proto.Channel(string(chID)[len(messagePrefix):]))
		}
	}
	return channels, nil
}

func decodeEngineClientMessage(data []byte) (*proto.Message, error) {
	var msg proto.Message
	err := msg.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &msg, nil
}

func decodeEngineJoinMessage(data []byte) (*proto.JoinMessage, error) {
	var msg proto.JoinMessage
	err := msg.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &msg, nil
}

func decodeEngineLeaveMessage(data []byte) (*proto.LeaveMessage, error) {
	var msg proto.LeaveMessage
	err := msg.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &msg, nil
}

func decodeEngineControlMessage(data []byte) (*proto.ControlMessage, error) {
	var msg proto.ControlMessage
	err := msg.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &msg, nil
}

func decodeEngineAdminMessage(data []byte) (*proto.AdminMessage, error) {
	var msg proto.AdminMessage
	err := msg.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &msg, nil
}

func encodeEngineClientMessage(msg *proto.Message) ([]byte, error) {
	return msg.Marshal()
}

func encodeEngineJoinMessage(msg *proto.JoinMessage) ([]byte, error) {
	return msg.Marshal()
}

func encodeEngineLeaveMessage(msg *proto.LeaveMessage) ([]byte, error) {
	return msg.Marshal()
}

func encodeEngineControlMessage(msg *proto.ControlMessage) ([]byte, error) {
	return msg.Marshal()
}

func encodeEngineAdminMessage(msg *proto.AdminMessage) ([]byte, error) {
	return msg.Marshal()
}
