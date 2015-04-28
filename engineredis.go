package main

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/centrifugal/centrifugo/logger"

	"github.com/garyburd/redigo/redis"
)

// redisEngine uses Redis datastructures and PUB/SUB to manage Centrifuge logic.
// This engine allows to scale Centrifuge - you can run several Centrifuge instances
// connected to the same Redis and load balance clients between instances.
type redisEngine struct {
	app       *application
	pool      *redis.Pool
	psc       redis.PubSubConn
	connected bool
}

func newRedisEngine(app *application, host, port, password, db, url string, api bool) *redisEngine {
	server := host + ":" + port
	pool := newPool(server, password, db)
	return &redisEngine{
		app:  app,
		pool: pool,
	}
}

func newPool(server, password, db string) *redis.Pool {
	return &redis.Pool{
		MaxIdle:     3,
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

func (e *redisEngine) getName() string {
	return "Redis"
}

func (e *redisEngine) initialize() error {
	go e.initializePubSub()
	go e.checkConnectionStatus()
	return nil
}

func (e *redisEngine) checkConnectionStatus() {
	for {
		time.Sleep(time.Second)
		if e.connected {
			continue
		}
		go e.initializePubSub()
	}
}

func (e *redisEngine) initializePubSub() {
	e.connected = true
	e.psc = redis.PubSubConn{e.pool.Get()}
	defer e.psc.Close()
	err := e.psc.Subscribe(e.app.adminChannel)
	if err != nil {
		e.connected = false
		e.psc.Close()
		return
	}
	err = e.psc.Subscribe(e.app.controlChannel)
	if err != nil {
		e.connected = false
		e.psc.Close()
		return
	}
	for _, channel := range e.app.clientSubscriptionHub.getChannels() {
		err = e.psc.Subscribe(channel)
		if err != nil {
			e.connected = false
			e.psc.Close()
			return
		}
	}
	for {
		switch n := e.psc.Receive().(type) {
		case redis.Message:
			e.app.handleMessage(n.Channel, n.Data)
		case redis.Subscription:
		case error:
			logger.ERROR.Printf("error: %v\n", n)
			e.psc.Close()
			e.connected = false
			return
		}
	}
}

func (e *redisEngine) publish(channel string, message []byte) error {
	conn := e.pool.Get()
	defer conn.Close()
	_, err := conn.Do("PUBLISH", channel, message)
	return err
}

func (e *redisEngine) subscribe(channel string) error {
	return e.psc.Subscribe(channel)
}

func (e *redisEngine) unsubscribe(channel string) error {
	return e.psc.Unsubscribe(channel)
}

func (e *redisEngine) getHashKey(channel string) string {
	return e.app.channelPrefix + ".presence.hash." + channel
}

func (e *redisEngine) getSetKey(channel string) string {
	return e.app.channelPrefix + ".presence.set." + channel
}

func (e *redisEngine) addPresence(channel, uid string, info interface{}) error {
	conn := e.pool.Get()
	defer conn.Close()
	infoJson, err := json.Marshal(info)
	if err != nil {
		return err
	}
	expireAt := time.Now().Unix() + e.app.presenceExpireInterval
	hashKey := e.getHashKey(channel)
	setKey := e.getSetKey(channel)
	conn.Send("MULTI")
	conn.Send("ZADD", setKey, expireAt, uid)
	conn.Send("HSET", hashKey, uid, infoJson)
	_, err = conn.Do("EXEC")
	return err
}

func (e *redisEngine) removePresence(channel, uid string) error {
	conn := e.pool.Get()
	defer conn.Close()
	hashKey := e.getHashKey(channel)
	setKey := e.getSetKey(channel)
	conn.Send("MULTI")
	conn.Send("HDEL", hashKey, uid)
	conn.Send("ZREM", setKey, uid)
	_, err := conn.Do("EXEC")
	return err
}

func mapStringInterface(result interface{}, err error) (map[string]interface{}, error) {
	values, err := redis.Values(result, err)
	if err != nil {
		return nil, err
	}
	if len(values)%2 != 0 {
		return nil, errors.New("mapStringInterface expects even number of values result")
	}
	m := make(map[string]interface{}, len(values)/2)
	for i := 0; i < len(values); i += 2 {
		key, okKey := values[i].([]byte)
		value, okValue := values[i+1].([]byte)
		if !okKey || !okValue {
			return nil, errors.New("ScanMap key not a bulk string value")
		}
		var f interface{}
		err = json.Unmarshal(value, &f)
		if err != nil {
			return nil, errors.New("can not unmarshal value to interface")
		}
		m[string(key)] = f
	}
	return m, nil
}

func (e *redisEngine) getPresence(channel string) (map[string]interface{}, error) {
	conn := e.pool.Get()
	defer conn.Close()
	now := time.Now().Unix()
	hashKey := e.getHashKey(channel)
	setKey := e.getSetKey(channel)
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
	presence, err := mapStringInterface(reply, nil)
	return presence, err
}

func (e *redisEngine) addHistoryMessage(channel string, message interface{}, size, lifetime int64) error {
	conn := e.pool.Get()
	defer conn.Close()
	return nil
}

func (e *redisEngine) getHistory(channel string) ([]interface{}, error) {
	conn := e.pool.Get()
	defer conn.Close()
	return []interface{}{}, nil
}
