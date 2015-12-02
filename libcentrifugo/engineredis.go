package libcentrifugo

import (
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"sync"
	"time"

	"github.com/centrifugal/centrifugo/Godeps/_workspace/src/github.com/FZambia/go-logger"
	"github.com/centrifugal/centrifugo/Godeps/_workspace/src/github.com/garyburd/redigo/redis"
)

// RedisEngine uses Redis datastructures and PUB/SUB to manage Centrifugo logic.
// This engine allows to scale Centrifugo - you can run several Centrifugo instances
// connected to the same Redis and load balance clients between instances.
type RedisEngine struct {
	sync.RWMutex
	app      *Application
	pool     *redis.Pool
	psc      redis.PubSubConn
	api      bool
	inPubSub bool
	inAPI    bool
}

func newPool(server, password, db string, psize int) *redis.Pool {
	return &redis.Pool{
		MaxIdle:     3,
		MaxActive:   psize,
		Wait:        true,
		IdleTimeout: 240 * time.Second,
		Dial: func() (redis.Conn, error) {
			c, err := redis.Dial("tcp", server)
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
			if _, err := c.Do("SELECT", db); err != nil {
				c.Close()
				logger.CRITICAL.Println(err)
				return nil, err
			}
			return c, err
		},
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			_, err := c.Do("PING")
			return err
		},
	}
}

// NewRedisEngine initializes Redis Engine.
func NewRedisEngine(app *Application, host, port, password, db, redisURL string, api bool, psize int) *RedisEngine {
	if redisURL != "" {
		u, err := url.Parse(redisURL)
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
	if db == "" {
		db = "0"
	}
	server := host + ":" + port
	pool := newPool(server, password, db, psize)

	e := &RedisEngine{
		app:  app,
		pool: pool,
		api:  api,
	}
	logger.INFO.Printf("Redis engine: %s, database %s, pool size %d\n", server, db, psize)
	e.psc = redis.PubSubConn{Conn: e.pool.Get()}
	return e
}

func (e *RedisEngine) name() string {
	return "Redis"
}

func (e *RedisEngine) run() error {
	e.RLock()
	api := e.api
	e.RUnlock()
	go e.initializePubSub()
	if api {
		go e.initializeApi()
	}
	go e.checkConnectionStatus()
	return nil
}

func (e *RedisEngine) checkConnectionStatus() {
	for {
		time.Sleep(time.Second)
		e.RLock()
		inPubSub := e.inPubSub
		inAPI := e.inAPI
		e.RUnlock()
		if !inPubSub {
			e.psc = redis.PubSubConn{Conn: e.pool.Get()}
			go e.initializePubSub()
		}
		if e.api && !inAPI {
			go e.initializeApi()
		}
	}
}

type redisApiRequest struct {
	Data []apiCommand
}

func (e *RedisEngine) initializeApi() {
	e.Lock()
	e.inAPI = true
	e.Unlock()
	conn := e.pool.Get()
	defer conn.Close()
	defer func() {
		e.Lock()
		e.inAPI = false
		e.Unlock()
	}()
	e.app.RLock()
	apiKey := e.app.config.ChannelPrefix + "." + "api"
	e.app.RUnlock()

	done := make(chan struct{})
	bodies := make(chan []byte, 256)
	defer close(done)

	go func() {
		for {
			select {
			case body, ok := <-bodies:
				if !ok {
					return
				}
				var req redisApiRequest
				err := json.Unmarshal(body, &req)
				if err != nil {
					logger.ERROR.Println(err)
					continue
				}
				for _, command := range req.Data {
					_, err := e.app.apiCmd(command)
					if err != nil {
						logger.ERROR.Println(err)
					}
				}
			case <-done:
				return
			}
		}
	}()

	for {
		reply, err := conn.Do("BLPOP", apiKey, 0)
		if err != nil {
			logger.ERROR.Println(err)
			return
		}

		values, err := redis.Values(reply, nil)
		if err != nil {
			logger.ERROR.Println(err)
			return
		}
		if len(values) != 2 {
			logger.ERROR.Println("Wrong reply from Redis in BLPOP - expecting 2 values")
			continue
		}

		body, okValue := values[1].([]byte)

		if !okValue {
			logger.ERROR.Println("Wrong reply from Redis in BLPOP - can not value convert to bytes")
			continue
		}

		bodies <- body
	}
}

func (e *RedisEngine) initializePubSub() {
	defer e.psc.Close()
	e.Lock()
	e.inPubSub = true
	e.Unlock()
	defer func() {
		e.Lock()
		e.inPubSub = false
		e.Unlock()
	}()
	e.app.RLock()
	adminChannel := e.app.config.AdminChannel
	controlChannel := e.app.config.ControlChannel
	e.app.RUnlock()
	err := e.psc.Subscribe(adminChannel)
	if err != nil {
		e.psc.Close()
		return
	}
	err = e.psc.Subscribe(controlChannel)
	if err != nil {
		e.psc.Close()
		return
	}
	for _, chID := range e.app.clients.channels() {
		err = e.psc.Subscribe(chID)
		if err != nil {
			e.psc.Close()
			return
		}
	}
	for {
		switch n := e.psc.Receive().(type) {
		case redis.Message:
			e.app.handleMsg(ChannelID(n.Channel), n.Data)
		case redis.Subscription:
		case error:
			logger.ERROR.Printf("error: %v\n", n)
			e.psc.Close()
			return
		}
	}
}

func (e *RedisEngine) publish(chID ChannelID, message []byte) error {
	conn := e.pool.Get()
	defer conn.Close()
	_, err := conn.Do("PUBLISH", chID, message)
	return err
}

func (e *RedisEngine) subscribe(chID ChannelID) error {
	logger.DEBUG.Println("subscribe on Redis channel", chID)
	return e.psc.Subscribe(chID)
}

func (e *RedisEngine) unsubscribe(chID ChannelID) error {
	logger.DEBUG.Println("unsubscribe from Redis channel", chID)
	return e.psc.Unsubscribe(chID)
}

func (e *RedisEngine) getHashKey(chID ChannelID) string {
	e.app.RLock()
	defer e.app.RUnlock()
	return e.app.config.ChannelPrefix + ".presence.hash." + string(chID)
}

func (e *RedisEngine) getSetKey(chID ChannelID) string {
	e.app.RLock()
	defer e.app.RUnlock()
	return e.app.config.ChannelPrefix + ".presence.set." + string(chID)
}

func (e *RedisEngine) getHistoryKey(chID ChannelID) string {
	e.app.RLock()
	defer e.app.RUnlock()
	return e.app.config.ChannelPrefix + ".history.list." + string(chID)
}

func (e *RedisEngine) getLastMessageIDKey(chID ChannelID) string {
	e.app.RLock()
	defer e.app.RUnlock()
	return e.app.config.ChannelPrefix + ".last_message_id." + string(chID)
}

func (e *RedisEngine) addPresence(chID ChannelID, uid ConnID, info ClientInfo) error {
	e.app.RLock()
	presenceExpireInterval := e.app.config.PresenceExpireInterval
	e.app.RUnlock()
	conn := e.pool.Get()
	defer conn.Close()
	infoJson, err := json.Marshal(info)
	if err != nil {
		return err
	}
	expireAt := time.Now().Unix() + int64(presenceExpireInterval.Seconds())
	hashKey := e.getHashKey(chID)
	setKey := e.getSetKey(chID)
	conn.Send("MULTI")
	conn.Send("ZADD", setKey, expireAt, uid)
	conn.Send("HSET", hashKey, uid, infoJson)
	conn.Send("EXPIRE", setKey, presenceExpireInterval)
	conn.Send("EXPIRE", hashKey, presenceExpireInterval)
	_, err = conn.Do("EXEC")
	return err
}

func (e *RedisEngine) removePresence(chID ChannelID, uid ConnID) error {
	conn := e.pool.Get()
	defer conn.Close()
	hashKey := e.getHashKey(chID)
	setKey := e.getSetKey(chID)
	conn.Send("MULTI")
	conn.Send("HDEL", hashKey, uid)
	conn.Send("ZREM", setKey, uid)
	_, err := conn.Do("EXEC")
	return err
}

func mapStringClientInfo(result interface{}, err error) (map[ConnID]ClientInfo, error) {
	values, err := redis.Values(result, err)
	if err != nil {
		return nil, err
	}
	if len(values)%2 != 0 {
		return nil, errors.New("mapStringClientInfo expects even number of values result")
	}
	m := make(map[ConnID]ClientInfo, len(values)/2)
	for i := 0; i < len(values); i += 2 {
		key, okKey := values[i].([]byte)
		value, okValue := values[i+1].([]byte)
		if !okKey || !okValue {
			return nil, errors.New("ScanMap key not a bulk string value")
		}
		var f ClientInfo
		err = json.Unmarshal(value, &f)
		if err != nil {
			return nil, errors.New("can not unmarshal value to ClientInfo")
		}
		m[ConnID(key)] = f
	}
	return m, nil
}

func (e *RedisEngine) presence(chID ChannelID) (map[ConnID]ClientInfo, error) {
	conn := e.pool.Get()
	defer conn.Close()
	now := time.Now().Unix()
	hashKey := e.getHashKey(chID)
	setKey := e.getSetKey(chID)
	reply, err := conn.Do("ZRANGEBYSCORE", setKey, 0, now)
	if err != nil {
		return nil, err
	}
	expiredKeys, err := redis.Strings(reply, nil)
	if err != nil {
		return nil, err
	}
	if len(expiredKeys) > 0 {
		conn.Send("MULTI")
		conn.Send("ZREMRANGEBYSCORE", setKey, 0, now)
		for _, key := range expiredKeys {
			conn.Send("HDEL", hashKey, key)
		}
		_, err = conn.Do("EXEC")
		if err != nil {
			return nil, err
		}
	}
	reply, err = conn.Do("HGETALL", hashKey)
	if err != nil {
		return nil, err
	}
	return mapStringClientInfo(reply, nil)
}

func (e *RedisEngine) addHistory(chID ChannelID, message Message, opts historyOptions) error {
	conn := e.pool.Get()
	defer conn.Close()

	historyKey := e.getHistoryKey(chID)
	messageJson, err := json.Marshal(message)
	if err != nil {
		return err
	}

	conn.Send("MULTI")
	conn.Send("LPUSH", historyKey, messageJson)
	conn.Send("LTRIM", historyKey, 0, opts.Size-1)
	conn.Send("EXPIRE", historyKey, opts.Lifetime)
	conn.Send("SET", e.getLastMessageIDKey(chID), message.UID)
	_, err = conn.Do("EXEC")
	return err
}

func sliceOfMessages(result interface{}, err error) ([]Message, error) {
	values, err := redis.Values(result, err)
	if err != nil {
		return nil, err
	}
	msgs := make([]Message, len(values))
	for i := 0; i < len(values); i += 1 {
		value, okValue := values[i].([]byte)
		if !okValue {
			return nil, errors.New("error getting Message value")
		}
		var m Message
		err = json.Unmarshal(value, &m)
		if err != nil {
			return nil, errors.New("can not unmarshal value to Message")
		}
		msgs[i] = m
	}
	return msgs, nil
}

func (e *RedisEngine) history(chID ChannelID) ([]Message, error) {
	conn := e.pool.Get()
	defer conn.Close()
	historyKey := e.getHistoryKey(chID)
	reply, err := conn.Do("LRANGE", historyKey, 0, -1)
	if err != nil {
		return nil, err
	}
	return sliceOfMessages(reply, nil)
}

func sliceOfChannelIDs(result interface{}, prefix string, err error) ([]ChannelID, error) {
	values, err := redis.Values(result, err)
	if err != nil {
		return nil, err
	}
	channels := make([]ChannelID, len(values))
	for i := 0; i < len(values); i += 1 {
		value, okValue := values[i].([]byte)
		if !okValue {
			return nil, errors.New("error getting ChannelID value")
		}
		chID := ChannelID(value)
		channels[i] = chID
	}
	return channels, nil
}

// Requires Redis >= 2.8.0 (http://redis.io/commands/pubsub)
func (e *RedisEngine) channels() ([]ChannelID, error) {
	conn := e.pool.Get()
	defer conn.Close()
	prefix := e.app.channelIDPrefix()
	reply, err := conn.Do("PUBSUB", "CHANNELS", prefix+"*")
	if err != nil {
		return nil, err
	}
	return sliceOfChannelIDs(reply, prefix, nil)
}

func (e *RedisEngine) lastMessageID(ch ChannelID) (MessageID, error) {
	conn := e.pool.Get()
	defer conn.Close()
	reply, err := conn.Do("GET", e.getLastMessageIDKey(ch))
	if err != nil {
		return MessageID(""), err
	}
	if reply == nil {
		return MessageID(""), nil
	}
	strVal, err := redis.String(reply, err)
	if err != nil {
		return MessageID(""), err
	}
	return MessageID(strVal), nil
}
