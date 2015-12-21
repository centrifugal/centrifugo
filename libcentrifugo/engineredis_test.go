package libcentrifugo

import (
	"errors"
	"net"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/Godeps/_workspace/src/github.com/garyburd/redigo/redis"
	"github.com/centrifugal/centrifugo/Godeps/_workspace/src/github.com/stretchr/testify/assert"
)

type testRedisConn struct {
	redis.Conn
}

const (
	testRedisHost     = "127.0.0.1"
	testRedisPort     = "6379"
	testRedisPassword = ""
	testRedisDB       = "9"
	testRedisURL      = "redis://:@127.0.0.1:6379/9"
	testRedisPoolSize = 5
)

func (t testRedisConn) close() error {
	_, err := t.Conn.Do("SELECT", testRedisDB)
	if err != nil {
		return nil
	}
	_, err = t.Conn.Do("FLUSHDB")
	if err != nil {
		return err
	}
	return t.Conn.Close()
}

// Get connection to Redis, select database and if that database not empty
// then panic to prevent existing data corruption.
func dial() testRedisConn {
	addr := net.JoinHostPort(testRedisHost, testRedisPort)
	c, err := redis.DialTimeout("tcp", addr, 0, 1*time.Second, 1*time.Second)
	if err != nil {
		panic(err)
	}

	_, err = c.Do("SELECT", testRedisDB)
	if err != nil {
		c.Close()
		panic(err)
	}

	n, err := redis.Int(c.Do("DBSIZE"))
	if err != nil {
		c.Close()
		panic(err)
	}

	if n != 0 {
		c.Close()
		panic(errors.New("database is not empty, test can not continue"))
	}

	return testRedisConn{c}
}

func testRedisEngine(app *Application) *RedisEngine {
	e := NewRedisEngine(app, testRedisHost, testRedisPort, testRedisPassword, testRedisDB, testRedisURL, true, testRedisPoolSize)
	return e
}

func TestRedisEngine(t *testing.T) {
	c := dial()
	defer c.close()
	app := testApp()
	e := testRedisEngine(app)
	err := e.run()
	assert.Equal(t, nil, err)
	app.SetEngine(e)
	assert.Equal(t, e.name(), "Redis")

	hasActiveSubscribers, err := e.publish(ChannelID("channel"), []byte("{}"))
	assert.False(t, hasActiveSubscribers)
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, e.subscribe(ChannelID("channel")))
	// Now we've subscribed...
	hasActiveSubscribers, err = e.publish(ChannelID("channel"), []byte("{}"))
	assert.True(t, hasActiveSubscribers)
	assert.Equal(t, nil, e.unsubscribe(ChannelID("channel")))

	// Now we've unsubscribed again..
	hasActiveSubscribers, err = e.publish(ChannelID("channel"), []byte("{}"))
	assert.False(t, hasActiveSubscribers)

	// test adding presence
	assert.Equal(t, nil, e.addPresence(ChannelID("channel"), "uid", ClientInfo{}))

	// test getting presence
	p, err := e.presence(ChannelID("channel"))
	assert.Equal(t, nil, err)
	assert.Equal(t, 1, len(p))

	// test removing presence
	err = e.removePresence(ChannelID("channel"), "uid")
	assert.Equal(t, nil, err)

	// test adding history
	assert.Equal(t, nil, e.addHistory(ChannelID("channel"), Message{}, addHistoryOpts{4, 1}))
	h, err := e.history(ChannelID("channel"), historyOpts{})
	assert.Equal(t, nil, err)
	assert.Equal(t, 1, len(h))

	// test history limit
	assert.Equal(t, nil, e.addHistory(ChannelID("channel"), Message{}, addHistoryOpts{4, 1}))
	assert.Equal(t, nil, e.addHistory(ChannelID("channel"), Message{}, addHistoryOpts{4, 1}))
	assert.Equal(t, nil, e.addHistory(ChannelID("channel"), Message{}, addHistoryOpts{4, 1}))
	h, err = e.history(ChannelID("channel"), historyOpts{Limit: 2})
	assert.Equal(t, nil, err)
	assert.Equal(t, 2, len(h))

	// test history limit greater than history size
	assert.Equal(t, nil, e.addHistory(ChannelID("channel"), Message{}, addHistoryOpts{1, 1}))
	assert.Equal(t, nil, e.addHistory(ChannelID("channel"), Message{}, addHistoryOpts{1, 1}))
	assert.Equal(t, nil, e.addHistory(ChannelID("channel"), Message{}, addHistoryOpts{1, 1}))
	h, err = e.history(ChannelID("channel"), historyOpts{Limit: 2})

	// OnlySaveIfActive tests
	// 1. add history with OnlySaveIfActive = true should be a no-op if history is empty
	assert.Equal(t, nil, e.addHistory(ChannelID("channel"), Message{}, addHistoryOpts{2, 5, true}))
	h, err = e.history(ChannelID("channel"))
	assert.Equal(t, nil, err)
	assert.Equal(t, 0, len(h))

	// 2. add history with OnlySaveIfActive = false should always work
	assert.Equal(t, nil, e.addHistory(ChannelID("channel"), Message{}, addHistoryOpts{2, 5, false}))
	h, err = e.history(ChannelID("channel"))
	assert.Equal(t, nil, err)
	assert.Equal(t, 1, len(h))

	// 3. add with OnlySaveIfActive = true should work immediately since there should be something in history
	// for 5 seconds from above
	assert.Equal(t, nil, e.addHistory(ChannelID("channel"), Message{}, addHistoryOpts{2, 5, true}))
	h, err = e.history(ChannelID("channel"))
	assert.Equal(t, nil, err)
	assert.Equal(t, 2, len(h))

	// test API
	apiKey := e.app.config.ChannelPrefix + "." + "api"
	_, err = c.Conn.Do("LPUSH", apiKey, []byte("{}"))
	assert.Equal(t, nil, err)
}

func TestRedisChannels(t *testing.T) {
	c := dial()
	defer c.close()
	app := testRedisApp()
	channels, err := app.engine.channels()
	assert.Equal(t, nil, err)
	assert.Equal(t, 0, len(channels))
	createTestClients(app, 10, 1)
	channels, err = app.engine.channels()
	assert.Equal(t, nil, err)
	assert.Equal(t, 10, len(channels))
}
