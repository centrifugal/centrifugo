package main

import (
	"time"

	"github.com/centrifugal/centrifugo/logger"

	"github.com/garyburd/redigo/redis"
)

type redisEngine struct {
	app  *application
	pool *redis.Pool
	psc  redis.PubSubConn
}

func newRedisEngine(app *application, host, port, password, db, url string, api bool) *redisEngine {
	server := host + ":" + port
	pool := newPool(server, password, db)
	conn := pool.Get()
	psc := redis.PubSubConn{conn}
	return &redisEngine{
		app:  app,
		pool: pool,
		psc:  psc,
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
	e.psc.Subscribe(e.app.adminChannel)
	e.psc.Subscribe(e.app.controlChannel)
	go func() {
		for {
			switch n := e.psc.Receive().(type) {
			case redis.Message:
				e.app.handleMessage(n.Channel, string(n.Data))
			case redis.PMessage:
			case redis.Subscription:
			case error:
				logger.ERROR.Printf("error: %v\n", n)
			}
		}
	}()
	return nil
}

func (e *redisEngine) publish(channel, message string) error {
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

func (e *redisEngine) addPresence(channel, uid string, info interface{}) error {
	return nil
}

func (e *redisEngine) removePresence(channel, uid string) error {
	return nil
}

func (e *redisEngine) getPresence(channel string) (map[string]interface{}, error) {
	return map[string]interface{}{}, nil
}

func (e *redisEngine) addHistoryMessage(channel string, message interface{}) error {
	return nil
}

func (e *redisEngine) getHistory(channel string) ([]interface{}, error) {
	return []interface{}{}, nil
}
