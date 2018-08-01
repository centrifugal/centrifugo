// +build integration

package centrifuge

import (
	"errors"
	"fmt"
	"math/rand"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/stretchr/testify/assert"
)

type testRedisConn struct {
	redis.Conn
}

const (
	testRedisHost     = "127.0.0.1"
	testRedisPort     = 6379
	testRedisPassword = ""
	testRedisDB       = 9
	testRedisURL      = "redis://:@127.0.0.1:6379/9"
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
	addr := net.JoinHostPort(testRedisHost, strconv.Itoa(testRedisPort))
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

func newTestRedisEngine() *RedisEngine {
	return NewTestRedisEngineWithPrefix("centrifugotest")
}

func NewTestRedisEngineWithPrefix(prefix string) *RedisEngine {
	n, _ := New(Config{})
	redisConf := RedisShardConfig{
		Host:        testRedisHost,
		Port:        testRedisPort,
		Password:    testRedisPassword,
		DB:          testRedisDB,
		Prefix:      prefix,
		ReadTimeout: 100 * time.Second,
	}
	e, _ := NewRedisEngine(n, RedisEngineConfig{Shards: []RedisShardConfig{redisConf}})
	n.SetEngine(e)
	err := n.Run()
	if err != nil {
		panic(err)
	}
	return e
}

func TestRedisEngine(t *testing.T) {
	c := dial()
	defer c.close()

	e := newTestRedisEngine()

	assert.Equal(t, e.name(), "Redis")

	channels, err := e.channels()
	assert.NoError(t, err)
	assert.Equal(t, 0, len(channels))

	pub := newTestPublication()

	err = <-e.publish("channel", pub, nil)
	assert.NoError(t, <-e.publish("channel", pub, nil))
	assert.NoError(t, e.subscribe("channel"))
	assert.NoError(t, e.unsubscribe("channel"))

	// test adding presence
	assert.NoError(t, e.addPresence("channel", "uid", &ClientInfo{}, 25*time.Second))

	p, err := e.presence("channel")
	assert.NoError(t, err)
	assert.Equal(t, 1, len(p))

	err = e.removePresence("channel", "uid")
	assert.NoError(t, err)

	rawData := Raw([]byte("{}"))
	pub = &Publication{UID: "test UID", Data: rawData}

	// test adding history
	assert.NoError(t, <-e.publish("channel", pub, &ChannelOptions{HistorySize: 4, HistoryLifetime: 1, HistoryDropInactive: false}))
	h, err := e.history("channel", 0)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(h))
	assert.Equal(t, h[0].UID, "test UID")

	// test history limit
	assert.NoError(t, <-e.publish("channel", pub, &ChannelOptions{HistorySize: 4, HistoryLifetime: 1, HistoryDropInactive: false}))
	assert.NoError(t, <-e.publish("channel", pub, &ChannelOptions{HistorySize: 4, HistoryLifetime: 1, HistoryDropInactive: false}))
	assert.NoError(t, <-e.publish("channel", pub, &ChannelOptions{HistorySize: 4, HistoryLifetime: 1, HistoryDropInactive: false}))
	h, err = e.history("channel", 2)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(h))

	// test history limit greater than history size
	assert.NoError(t, <-e.publish("channel", pub, &ChannelOptions{HistorySize: 1, HistoryLifetime: 1, HistoryDropInactive: false}))
	assert.NoError(t, <-e.publish("channel", pub, &ChannelOptions{HistorySize: 1, HistoryLifetime: 1, HistoryDropInactive: false}))
	assert.NoError(t, <-e.publish("channel", pub, &ChannelOptions{HistorySize: 1, HistoryLifetime: 1, HistoryDropInactive: false}))

	// ask all history.
	h, err = e.history("channel", 0)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(h))

	// ask more history than history_size.
	h, err = e.history("channel", 2)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(h))

	// test publishing control message.
	err = <-e.publishControl([]byte(""))
	assert.NoError(t, nil, err)

	// test publishing join message.
	joinMessage := Join{}
	assert.NoError(t, <-e.publishJoin("channel", &joinMessage, nil))

	// test publishing leave message.
	leaveMessage := Leave{}
	assert.NoError(t, <-e.publishLeave("channel", &leaveMessage, nil))
}

func TestRedisEnginePublishHistoryDropInactive(t *testing.T) {

	c := dial()
	defer c.close()

	e := newTestRedisEngine()

	rawData := Raw([]byte("{}"))
	pub := &Publication{UID: "test UID", Data: rawData}

	// 1. add history with DropInactive = true should be a no-op if history is empty
	assert.NoError(t, nil, <-e.publish("channel", pub, &ChannelOptions{HistorySize: 2, HistoryLifetime: 5, HistoryDropInactive: true}))
	h, err := e.history("channel", 0)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(h))

	// 2. add history with DropInactive = false should always work
	assert.NoError(t, nil, <-e.publish("channel", pub, &ChannelOptions{HistorySize: 2, HistoryLifetime: 5, HistoryDropInactive: false}))
	h, err = e.history("channel", 0)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(h))

	// 3. add with DropInactive = true should work immediately since there should be something in history
	// for 5 seconds from above
	assert.NoError(t, nil, <-e.publish("channel", pub, &ChannelOptions{HistorySize: 2, HistoryLifetime: 5, HistoryDropInactive: true}))
	h, err = e.history("channel", 0)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(h))
}

// Test drop inactive edge case - see analogue in Memory Engine tests for more description.
func TestRedisEngineDropInactive(t *testing.T) {
	c := dial()
	defer c.close()

	e := newTestRedisEngine()

	config := e.node.Config()
	config.HistoryDropInactive = true
	config.HistoryLifetime = 5
	config.HistorySize = 2
	e.node.Reload(config)

	pub := newTestPublication()

	opts, _ := e.node.ChannelOpts("channel")

	assert.Nil(t, <-e.publish("channel", pub, &opts))
	h, err := e.history("channel", 0)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(h))

	e.unsubscribe("channel")

	assert.NoError(t, <-e.publish("channel", pub, &opts))
	h, err = e.history("channel", 0)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(h))
}

func TestRedisEngineRecover(t *testing.T) {

	c := dial()
	defer c.close()

	e := newTestRedisEngine()

	rawData := Raw([]byte("{}"))
	pub := &Publication{Data: rawData}

	pub.UID = "1"
	assert.NoError(t, nil, <-e.publish("channel", pub, &ChannelOptions{HistorySize: 10, HistoryLifetime: 2}))
	pub.UID = "2"
	assert.NoError(t, nil, <-e.publish("channel", pub, &ChannelOptions{HistorySize: 10, HistoryLifetime: 2}))
	pub.UID = "3"
	assert.NoError(t, nil, <-e.publish("channel", pub, &ChannelOptions{HistorySize: 10, HistoryLifetime: 2}))
	pub.UID = "4"
	assert.NoError(t, nil, <-e.publish("channel", pub, &ChannelOptions{HistorySize: 10, HistoryLifetime: 2}))
	pub.UID = "5"
	assert.NoError(t, nil, <-e.publish("channel", pub, &ChannelOptions{HistorySize: 10, HistoryLifetime: 2}))

	pubs, recovered, err := e.recoverHistory("channel", "2")
	assert.NoError(t, err)
	assert.True(t, recovered)
	assert.Equal(t, 3, len(pubs))
	assert.Equal(t, "5", pubs[0].UID)
	assert.Equal(t, "4", pubs[1].UID)
	assert.Equal(t, "3", pubs[2].UID)

	pubs, recovered, err = e.recoverHistory("channel", "6")
	assert.NoError(t, err)
	assert.False(t, recovered)
	assert.Equal(t, 5, len(pubs))

	assert.NoError(t, e.removeHistory("channel"))
	pubs, recovered, err = e.recoverHistory("channel", "2")
	assert.NoError(t, err)
	assert.False(t, recovered)
	assert.Equal(t, 0, len(pubs))
}

func TestRedisEngineSubscribeUnsubscribe(t *testing.T) {
	c := dial()
	defer c.close()

	// Custom prefix to not collide with other tests.
	e := NewTestRedisEngineWithPrefix("TestRedisEngineSubscribeUnsubscribe")

	e.subscribe("1-test")
	e.subscribe("1-test")
	channels, err := e.channels()
	assert.Equal(t, nil, err)
	if len(channels) != 1 {
		// Redis PUBSUB CHANNELS command looks like eventual consistent, so sometimes
		// it returns wrong results, sleeping for a while helps in such situations.
		// See https://gist.github.com/FZambia/80a5241e06b4662f7fe89cfaf24072c3
		time.Sleep(500 * time.Millisecond)
		channels, _ := e.channels()
		assert.Equal(t, 1, len(channels), fmt.Sprintf("%#v", channels))
	}

	e.unsubscribe("1-test")
	channels, err = e.channels()
	assert.Equal(t, nil, err)
	if len(channels) != 0 {
		time.Sleep(500 * time.Millisecond)
		channels, _ := e.channels()
		assert.Equal(t, 0, len(channels), fmt.Sprintf("%#v", channels))
	}

	var wg sync.WaitGroup

	// The same channel in parallel.
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			e.subscribe("2-test")
			e.unsubscribe("2-test")
		}()
	}
	wg.Wait()
	channels, err = e.channels()
	assert.Equal(t, nil, err)

	if len(channels) != 0 {
		time.Sleep(500 * time.Millisecond)
		channels, _ := e.channels()
		assert.Equal(t, 0, len(channels), fmt.Sprintf("%#v", channels))
	}

	// Different channels in parallel.
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			e.subscribe("3-test-" + strconv.Itoa(i))
			e.unsubscribe("3-test-" + strconv.Itoa(i))
		}(i)
	}
	wg.Wait()
	channels, err = e.channels()
	assert.Equal(t, nil, err)
	if len(channels) != 0 {
		time.Sleep(500 * time.Millisecond)
		channels, _ := e.channels()
		assert.Equal(t, 0, len(channels), fmt.Sprintf("%#v", channels))
	}

	// The same channel sequential.
	for i := 0; i < 10000; i++ {
		e.subscribe("4-test")
		e.unsubscribe("4-test")
	}
	channels, err = e.channels()
	assert.Equal(t, nil, err)
	if len(channels) != 0 {
		time.Sleep(500 * time.Millisecond)
		channels, _ := e.channels()
		assert.Equal(t, 0, len(channels), fmt.Sprintf("%#v", channels))
	}

	// Different channels sequential.
	for j := 0; j < 10; j++ {
		for i := 0; i < 10000; i++ {
			e.subscribe("5-test-" + strconv.Itoa(i))
			e.unsubscribe("5-test-" + strconv.Itoa(i))
		}
		channels, err = e.channels()
		assert.Equal(t, nil, err)
		if len(channels) != 0 {
			time.Sleep(500 * time.Millisecond)
			channels, _ := e.channels()
			assert.Equal(t, 0, len(channels), fmt.Sprintf("%#v", channels))
		}
	}

	// Different channels subscribe only in parallel.
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			e.subscribe("6-test-" + strconv.Itoa(i))
		}(i)
	}
	wg.Wait()
	channels, err = e.channels()
	assert.Equal(t, nil, err)
	if len(channels) != 100 {
		time.Sleep(500 * time.Millisecond)
		channels, _ := e.channels()
		assert.Equal(t, 100, len(channels), fmt.Sprintf("%#v", channels))
	}

	// Different channels unsubscribe only in parallel.
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			e.unsubscribe("6-test-" + strconv.Itoa(i))
		}(i)
	}
	wg.Wait()
	channels, err = e.channels()
	assert.Equal(t, nil, err)
	if len(channels) != 0 {
		time.Sleep(500 * time.Millisecond)
		channels, _ := e.channels()
		assert.Equal(t, 0, len(channels), fmt.Sprintf("%#v", channels))
	}

	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			e.unsubscribe("7-test-" + strconv.Itoa(i))
			e.unsubscribe("8-test-" + strconv.Itoa(i))
			e.subscribe("8-test-" + strconv.Itoa(i))
			e.unsubscribe("9-test-" + strconv.Itoa(i))
			e.subscribe("7-test-" + strconv.Itoa(i))
			e.unsubscribe("8-test-" + strconv.Itoa(i))
			e.subscribe("9-test-" + strconv.Itoa(i))
			e.unsubscribe("9-test-" + strconv.Itoa(i))
			e.unsubscribe("7-test-" + strconv.Itoa(i))
		}(i)
	}
	wg.Wait()
	channels, err = e.channels()
	assert.Equal(t, nil, err)
	if len(channels) != 0 {
		time.Sleep(500 * time.Millisecond)
		channels, _ := e.channels()
		assert.Equal(t, 0, len(channels), fmt.Sprintf("%#v", channels))
	}
}

var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func randString(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}

// TestConsistentIndex exists to test consistent hashing algorithm we use.
// As we use random in this test we carefully do asserts here.
// At least it can protect us from stupid mistakes to certain degree.
// We just expect +-equal distribution and keeping most of chans on
// the same shard after resharding.
func TestConsistentIndex(t *testing.T) {

	rand.Seed(time.Now().UnixNano())
	numChans := 10000
	numShards := 10
	chans := make([]string, numChans)
	for i := 0; i < numChans; i++ {
		chans[i] = randString(rand.Intn(10) + 1)
	}
	chanToShard := make(map[string]int)
	chanToReshard := make(map[string]int)
	shardToChan := make(map[int][]string)

	for _, ch := range chans {
		shard := consistentIndex(ch, numShards)
		reshard := consistentIndex(ch, numShards+1)
		chanToShard[ch] = shard
		chanToReshard[ch] = reshard

		if _, ok := shardToChan[shard]; !ok {
			shardToChan[shard] = []string{}
		}
		shardChans := shardToChan[shard]
		shardChans = append(shardChans, ch)
		shardToChan[shard] = shardChans
	}

	for shard, shardChans := range shardToChan {
		shardFraction := float64(len(shardChans)) * 100 / float64(len(chans))
		fmt.Printf("Shard %d: %f%%\n", shard, shardFraction)
	}

	sameShards := 0

	// And test resharding.
	for ch, shard := range chanToShard {
		reshard := chanToReshard[ch]
		if shard == reshard {
			sameShards++
		}
	}
	sameFraction := float64(sameShards) * 100 / float64(len(chans))
	fmt.Printf("Same shards after resharding: %f%%\n", sameFraction)
	assert.True(t, sameFraction > 0.7)
}

func BenchmarkRedisEngineConsistentIndex(b *testing.B) {
	for i := 0; i < b.N; i++ {
		consistentIndex(strconv.Itoa(i), 4)
	}
}

func BenchmarkRedisEngineIndex(b *testing.B) {
	for i := 0; i < b.N; i++ {
		index(strconv.Itoa(i), 4)
	}
}

func BenchmarkRedisEnginePublish(b *testing.B) {
	e := newTestRedisEngine()
	rawData := Raw([]byte(`{"bench": true}`))
	pub := &Publication{UID: "test UID", Data: rawData}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		<-e.publish("channel", pub, &ChannelOptions{HistorySize: 0, HistoryLifetime: 0, HistoryDropInactive: false})
	}
}

func BenchmarkRedisEnginePublishParallel(b *testing.B) {
	e := newTestRedisEngine()
	rawData := Raw([]byte(`{"bench": true}`))
	pub := &Publication{UID: "test UID", Data: rawData}
	b.SetParallelism(128)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			<-e.publish("channel", pub, &ChannelOptions{HistorySize: 0, HistoryLifetime: 0, HistoryDropInactive: false})
		}
	})
}

func BenchmarkRedisEnginePublishWithHistory(b *testing.B) {
	e := newTestRedisEngine()
	rawData := Raw([]byte(`{"bench": true}`))
	pub := &Publication{UID: "test-uid", Data: rawData}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		<-e.publish("channel", pub, &ChannelOptions{HistorySize: 100, HistoryLifetime: 100, HistoryDropInactive: false})
	}
}

func BenchmarkRedisEnginePublishWithHistoryParallel(b *testing.B) {
	e := newTestRedisEngine()
	rawData := Raw([]byte(`{"bench": true}`))
	pub := &Publication{UID: "test-uid", Data: rawData}
	b.SetParallelism(128)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			<-e.publish("channel", pub, &ChannelOptions{HistorySize: 100, HistoryLifetime: 100, HistoryDropInactive: false})
		}
	})
}

func BenchmarkRedisEngineAddPresence(b *testing.B) {
	e := newTestRedisEngine()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := e.addPresence("channel", "uid", &ClientInfo{}, 300*time.Second)
		if err != nil {
			panic(err)
		}
	}
}

func BenchmarkRedisEngineAddPresenceParallel(b *testing.B) {
	e := newTestRedisEngine()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			err := e.addPresence("channel", "uid", &ClientInfo{}, 300*time.Second)
			if err != nil {
				panic(err)
			}
		}
	})
}

func BenchmarkRedisEnginePresence(b *testing.B) {
	e := newTestRedisEngine()
	e.addPresence("channel", "uid", &ClientInfo{}, 300*time.Second)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := e.presence("channel")
		if err != nil {
			panic(err)
		}
	}
}

func BenchmarkRedisEnginePresenceParallel(b *testing.B) {
	e := newTestRedisEngine()
	e.addPresence("channel", "uid", &ClientInfo{}, 300*time.Second)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := e.presence("channel")
			if err != nil {
				panic(err)
			}
		}
	})
}

func BenchmarkRedisEngineHistory(b *testing.B) {
	e := newTestRedisEngine()
	rawData := Raw([]byte("{}"))
	pub := &Publication{UID: "test UID", Data: rawData}
	for i := 0; i < 4; i++ {
		<-e.publish("channel", pub, &ChannelOptions{HistorySize: 4, HistoryLifetime: 300, HistoryDropInactive: false})
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := e.history("channel", 0)
		if err != nil {
			panic(err)
		}

	}
}

func BenchmarkRedisEngineHistoryParallel(b *testing.B) {
	e := newTestRedisEngine()
	rawData := Raw([]byte("{}"))
	pub := &Publication{UID: "test-uid", Data: rawData}
	for i := 0; i < 4; i++ {
		<-e.publish("channel", pub, &ChannelOptions{HistorySize: 4, HistoryLifetime: 300, HistoryDropInactive: false})
	}
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := e.history("channel", 0)
			if err != nil {
				panic(err)
			}
		}
	})
}

func BenchmarkRedisEngineHistoryRecoverParallel(b *testing.B) {
	e := newTestRedisEngine()
	rawData := Raw([]byte("{}"))
	numMessages := 100
	for i := 0; i < numMessages; i++ {
		pub := &Publication{UID: "uid" + strconv.Itoa(i), Data: rawData}
		<-e.publish("channel", pub, &ChannelOptions{HistorySize: numMessages, HistoryLifetime: 300, HistoryDropInactive: false})
	}
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _, err := e.recoverHistory("channel", "uid"+strconv.Itoa(numMessages-5))
			if err != nil {
				panic(err)
			}
		}
	})
}
