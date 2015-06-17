package libcentrifugo

import (
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"time"

	"github.com/centrifugal/centrifugo/libcentrifugo/logger"
	"github.com/garyburd/redigo/redis"
)

// redisEngine uses Redis datastructures and PUB/SUB to manage Centrifugo logic.
// This engine allows to scale Centrifugo - you can run several Centrifugo instances
// connected to the same Redis and load balance clients between instances.
type RedisEngine struct {
	app      *Application
	pool     *redis.Pool
	psc      redis.PubSubConn
	api      bool
	inPubSub bool
	inAPI    bool
}

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

	go e.initializePubSub()
	if e.api {
		go e.initializeApi()
	}
	go e.checkConnectionStatus()

	return e
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

func (e *RedisEngine) name() string {
	return "Redis"
}

func (e *RedisEngine) checkConnectionStatus() {
	for {
		time.Sleep(time.Second)
		if !e.inPubSub {
			go e.initializePubSub()
		}
		if e.api && !e.inAPI {
			go e.initializeApi()
		}
	}
}

type redisApiRequest struct {
	Project ProjectKey
	Data    []apiCommand
}

func (e *RedisEngine) initializeApi() {
	e.inAPI = true
	conn := e.pool.Get()
	defer conn.Close()
	defer func() {
		e.inAPI = false
	}()
	e.app.RLock()
	apiKey := e.app.config.ChannelPrefix + "." + "api"
	e.app.RUnlock()
	for {
		reply, err := conn.Do("BLPOP", apiKey, 0)
		if err != nil {
			logger.ERROR.Println(err)
			return
		}
		a, err := mapStringByte(reply, nil)
		if err != nil {
			logger.ERROR.Println(err)
			continue
		}
		body, ok := a[apiKey]
		if !ok {
			continue
		}
		var req redisApiRequest
		err = json.Unmarshal(body, &req)
		if err != nil {
			logger.ERROR.Println(err)
			continue
		}
		project, exists := e.app.projectByKey(req.Project)
		if !exists {
			logger.ERROR.Println("no project found with key", req.Project)
			continue
		}

		for _, command := range req.Data {
			_, err := e.app.apiCmd(project, command)
			if err != nil {
				logger.ERROR.Println(err)
			}
		}
	}
}

func (e *RedisEngine) initializePubSub() {
	e.inPubSub = true
	e.psc = redis.PubSubConn{e.pool.Get()}
	defer e.psc.Close()
	defer func() {
		e.inPubSub = false
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
	return e.psc.Subscribe(chID)
}

func (e *RedisEngine) unsubscribe(chID ChannelID) error {
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
	expireAt := time.Now().Unix() + presenceExpireInterval
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

func mapStringByte(result interface{}, err error) (map[string][]byte, error) {
	values, err := redis.Values(result, err)
	if err != nil {
		return nil, err
	}
	if len(values)%2 != 0 {
		return nil, errors.New("mapStringByte expects even number of values result")
	}
	m := make(map[string][]byte, len(values)/2)
	for i := 0; i < len(values); i += 2 {
		key, okKey := values[i].([]byte)
		value, okValue := values[i+1].([]byte)
		if !okKey || !okValue {
			return nil, errors.New("ScanMap key not a bulk string value")
		}
		m[string(key)] = value
	}
	return m, nil
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

func (e *RedisEngine) addHistory(chID ChannelID, message Message, size, lifetime int64) error {
	conn := e.pool.Get()
	defer conn.Close()

	historyKey := e.getHistoryKey(chID)
	messageJson, err := json.Marshal(message)
	if err != nil {
		return err
	}
	conn.Send("MULTI")
	conn.Send("LPUSH", historyKey, messageJson)
	conn.Send("LTRIM", historyKey, 0, size-1)
	conn.Send("EXPIRE", historyKey, lifetime)
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
	history, err := sliceOfMessages(reply, nil)
	return history, err
}
