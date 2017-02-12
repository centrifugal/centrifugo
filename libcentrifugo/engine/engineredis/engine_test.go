package engineredis

import (
	"errors"
	"fmt"
	"math/rand"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/libcentrifugo/config"
	"github.com/centrifugal/centrifugo/libcentrifugo/logger"
	"github.com/centrifugal/centrifugo/libcentrifugo/node"
	"github.com/centrifugal/centrifugo/libcentrifugo/proto"
	"github.com/centrifugal/centrifugo/libcentrifugo/raw"
	"github.com/garyburd/redigo/redis"
	"github.com/stretchr/testify/assert"
)

type testRedisConn struct {
	redis.Conn
}

const (
	testRedisHost         = "127.0.0.1"
	testRedisPort         = "6379"
	testRedisPassword     = ""
	testRedisDB           = "9"
	testRedisURL          = "redis://:@127.0.0.1:6379/9"
	testRedisPoolSize     = 256
	testRedisNumAPIShards = 4
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

func getTestChannelOptions() proto.ChannelOptions {
	return proto.ChannelOptions{
		Watch:           true,
		Publish:         true,
		Presence:        true,
		HistorySize:     1,
		HistoryLifetime: 1,
	}
}

func getTestNamespace(name node.NamespaceKey) node.Namespace {
	return node.Namespace{
		Name:           name,
		ChannelOptions: getTestChannelOptions(),
	}
}

func NewTestConfig() *node.Config {
	c := node.DefaultConfig
	var ns []node.Namespace
	ns = append(ns, getTestNamespace("test"))
	c.Namespaces = ns
	c.Secret = "secret"
	c.ChannelOptions = getTestChannelOptions()
	return c
}

func newTestMessage() *proto.Message {
	return proto.NewMessage(string("channel"), []byte("{}"), "", nil)
}

func NewTestRedisEngine() *RedisEngine {
	return NewTestRedisEngineWithPrefix("centrifugotest")
}

func NewTestRedisEngineWithPrefix(prefix string) *RedisEngine {
	logger.SetStdoutThreshold(logger.LevelNone)
	c := NewTestConfig()
	n := node.New("", c)
	redisConf := &ShardConfig{
		Host:         testRedisHost,
		Port:         testRedisPort,
		Password:     testRedisPassword,
		DB:           testRedisDB,
		PoolSize:     testRedisPoolSize,
		API:          true,
		NumAPIShards: testRedisNumAPIShards,
		Prefix:       prefix,
		ReadTimeout:  100 * time.Second,
	}
	e, _ := New(n, []*ShardConfig{redisConf})
	err := n.Run(&node.RunOptions{Engine: e})
	if err != nil {
		panic(err)
	}
	return e
}

func TestRedisEngine(t *testing.T) {
	c := dial()
	defer c.close()

	e := NewTestRedisEngine()

	err := e.Run()
	assert.Equal(t, nil, err)
	assert.Equal(t, e.Name(), "Redis")

	channels, err := e.Channels()
	assert.Equal(t, nil, err)
	if len(channels) > 0 {
		fmt.Printf("%#v", channels)
	}
	assert.Equal(t, 0, len(channels))

	testMsg := newTestMessage()

	err = <-e.PublishMessage(testMsg, nil)
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, e.Subscribe("channel"))
	// Now we've subscribed...
	err = <-e.PublishMessage(testMsg, nil)
	assert.Equal(t, nil, e.Unsubscribe("channel"))

	// test adding presence
	assert.Equal(t, nil, e.AddPresence("channel", "uid", proto.ClientInfo{}, int(e.node.Config().PresenceExpireInterval.Seconds())))

	// test getting presence
	p, err := e.Presence("channel")
	assert.Equal(t, nil, err)
	assert.Equal(t, 1, len(p))

	// test removing presence
	err = e.RemovePresence("channel", "uid")
	assert.Equal(t, nil, err)

	rawData := raw.Raw([]byte("{}"))
	msg := proto.Message{UID: "test UID", Channel: "channel", Data: rawData}

	// test adding history
	assert.Equal(t, nil, <-e.PublishMessage(&msg, &proto.ChannelOptions{HistorySize: 4, HistoryLifetime: 1, HistoryDropInactive: false}))
	h, err := e.History("channel", 0)
	assert.Equal(t, nil, err)
	assert.Equal(t, 1, len(h))
	assert.Equal(t, h[0].UID, "test UID")

	// test history limit
	assert.Equal(t, nil, <-e.PublishMessage(&msg, &proto.ChannelOptions{HistorySize: 4, HistoryLifetime: 1, HistoryDropInactive: false}))
	assert.Equal(t, nil, <-e.PublishMessage(&msg, &proto.ChannelOptions{HistorySize: 4, HistoryLifetime: 1, HistoryDropInactive: false}))
	assert.Equal(t, nil, <-e.PublishMessage(&msg, &proto.ChannelOptions{HistorySize: 4, HistoryLifetime: 1, HistoryDropInactive: false}))
	h, err = e.History("channel", 2)
	assert.Equal(t, nil, err)
	assert.Equal(t, 2, len(h))

	// test history limit greater than history size
	assert.Equal(t, nil, <-e.PublishMessage(&msg, &proto.ChannelOptions{HistorySize: 1, HistoryLifetime: 1, HistoryDropInactive: false}))
	assert.Equal(t, nil, <-e.PublishMessage(&msg, &proto.ChannelOptions{HistorySize: 1, HistoryLifetime: 1, HistoryDropInactive: false}))
	assert.Equal(t, nil, <-e.PublishMessage(&msg, &proto.ChannelOptions{HistorySize: 1, HistoryLifetime: 1, HistoryDropInactive: false}))
	h, err = e.History("channel", 2)

	// HistoryDropInactive tests - new channel to avoid conflicts with test above
	// 1. add history with DropInactive = true should be a no-op if history is empty
	msg2 := proto.Message{UID: "test UID", Channel: "channel-2"}

	assert.Equal(t, nil, <-e.PublishMessage(&msg2, &proto.ChannelOptions{HistorySize: 2, HistoryLifetime: 5, HistoryDropInactive: true}))
	h, err = e.History("channel-2", 0)
	assert.Equal(t, nil, err)
	assert.Equal(t, 0, len(h))

	// 2. add history with DropInactive = false should always work
	assert.Equal(t, nil, <-e.PublishMessage(&msg2, &proto.ChannelOptions{HistorySize: 2, HistoryLifetime: 5, HistoryDropInactive: false}))
	h, err = e.History("channel-2", 0)
	assert.Equal(t, nil, err)
	assert.Equal(t, 1, len(h))

	// 3. add with DropInactive = true should work immediately since there should be something in history
	// for 5 seconds from above
	assert.Equal(t, nil, <-e.PublishMessage(&msg2, &proto.ChannelOptions{HistorySize: 2, HistoryLifetime: 5, HistoryDropInactive: true}))
	h, err = e.History("channel-2", 0)
	assert.Equal(t, nil, err)
	assert.Equal(t, 2, len(h))

	// test API
	apiKey := "centrifugotest" + "." + "api"
	_, err = c.Conn.Do("LPUSH", apiKey, []byte("{}"))
	assert.Equal(t, nil, err)

	// test API shards
	for i := 0; i < testRedisNumAPIShards; i++ {
		queueKey := fmt.Sprintf("%s.%d", apiKey, i)
		_, err = c.Conn.Do("LPUSH", queueKey, []byte("{}"))
		assert.Equal(t, nil, err)
	}

	// test publishing control message.
	controlMsg := proto.NewControlMessage("uid", "method", []byte("{}"))
	err = <-e.PublishControl(controlMsg)
	assert.Equal(t, nil, err)

	// test publishing admin message.
	adminMessage := proto.NewAdminMessage("method", []byte("{}"))
	err = <-e.PublishAdmin(adminMessage)
	assert.Equal(t, nil, err)

	rawInfoData := raw.Raw([]byte("{}"))
	clientInfo := proto.NewClientInfo(string("1"), string("1"), rawInfoData, rawInfoData)

	// test publishing join message.
	joinMessage := proto.NewJoinMessage(string("test"), *clientInfo)
	err = <-e.PublishJoin(joinMessage, nil)
	assert.Equal(t, nil, err)

	// test publishing leave message.
	leaveMessage := proto.NewLeaveMessage(string("test"), *clientInfo)
	err = <-e.PublishLeave(leaveMessage, nil)
	assert.Equal(t, nil, err)
}

func TestMemoryEngineDropInactive(t *testing.T) {
	c := dial()
	defer c.close()

	e := NewTestRedisEngine()

	conf := e.node.Config()
	conf.HistoryDropInactive = true
	conf.HistoryLifetime = 5
	conf.HistorySize = 2
	e.node.SetConfig(&conf)

	err := e.Run()

	msg := proto.Message{UID: "test UID", Channel: "channel-drop-inactive"}
	opts, _ := e.node.ChannelOpts(msg.Channel)

	assert.Nil(t, <-e.PublishMessage(&msg, &opts))
	h, err := e.History(msg.Channel, 0)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(h))

	e.Unsubscribe(msg.Channel)

	assert.Nil(t, <-e.PublishMessage(&msg, &opts))
	h, err = e.History(msg.Channel, 0)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(h))
}

func TestRedisEngineSubscribeUnsubscribe(t *testing.T) {
	c := dial()
	defer c.close()

	// Custom prefix to not collide with other tests.
	e := NewTestRedisEngineWithPrefix("TestRedisEngineSubscribeUnsubscribe")

	e.Subscribe("1-test")
	e.Subscribe("1-test")
	channels, err := e.Channels()
	assert.Equal(t, nil, err)
	if len(channels) != 1 {
		// Redis PUBSUB CHANNELS command looks like eventual consistent, so sometimes
		// it returns wrong results, sleeping for a while helps in such situations.
		// See https://gist.github.com/FZambia/80a5241e06b4662f7fe89cfaf24072c3
		time.Sleep(500 * time.Millisecond)
		channels, _ := e.Channels()
		assert.Equal(t, 1, len(channels), fmt.Sprintf("%#v", channels))
	}

	e.Unsubscribe("1-test")
	channels, err = e.Channels()
	assert.Equal(t, nil, err)
	if len(channels) != 0 {
		time.Sleep(500 * time.Millisecond)
		channels, _ := e.Channels()
		assert.Equal(t, 0, len(channels), fmt.Sprintf("%#v", channels))
	}

	var wg sync.WaitGroup

	// The same channel in parallel.
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			e.Subscribe("2-test")
			e.Unsubscribe("2-test")
		}()
	}
	wg.Wait()
	channels, err = e.Channels()
	assert.Equal(t, nil, err)

	if len(channels) != 0 {
		time.Sleep(500 * time.Millisecond)
		channels, _ := e.Channels()
		assert.Equal(t, 0, len(channels), fmt.Sprintf("%#v", channels))
	}

	// Different channels in parallel.
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			e.Subscribe("3-test-" + strconv.Itoa(i))
			e.Unsubscribe("3-test-" + strconv.Itoa(i))
		}(i)
	}
	wg.Wait()
	channels, err = e.Channels()
	assert.Equal(t, nil, err)
	if len(channels) != 0 {
		time.Sleep(500 * time.Millisecond)
		channels, _ := e.Channels()
		assert.Equal(t, 0, len(channels), fmt.Sprintf("%#v", channels))
	}

	// The same channel sequential.
	for i := 0; i < 10000; i++ {
		e.Subscribe("4-test")
		e.Unsubscribe("4-test")
	}
	channels, err = e.Channels()
	assert.Equal(t, nil, err)
	if len(channels) != 0 {
		time.Sleep(500 * time.Millisecond)
		channels, _ := e.Channels()
		assert.Equal(t, 0, len(channels), fmt.Sprintf("%#v", channels))
	}

	// Different channels sequential.
	for j := 0; j < 10; j++ {
		for i := 0; i < 10000; i++ {
			e.Subscribe("5-test-" + strconv.Itoa(i))
			e.Unsubscribe("5-test-" + strconv.Itoa(i))
		}
		channels, err = e.Channels()
		assert.Equal(t, nil, err)
		if len(channels) != 0 {
			time.Sleep(500 * time.Millisecond)
			channels, _ := e.Channels()
			assert.Equal(t, 0, len(channels), fmt.Sprintf("%#v", channels))
		}
	}

	// Different channels subscribe only in parallel.
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			e.Subscribe("6-test-" + strconv.Itoa(i))
		}(i)
	}
	wg.Wait()
	channels, err = e.Channels()
	assert.Equal(t, nil, err)
	if len(channels) != 100 {
		time.Sleep(500 * time.Millisecond)
		channels, _ := e.Channels()
		assert.Equal(t, 100, len(channels), fmt.Sprintf("%#v", channels))
	}

	// Different channels unsubscribe only in parallel.
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			e.Unsubscribe("6-test-" + strconv.Itoa(i))
		}(i)
	}
	wg.Wait()
	channels, err = e.Channels()
	assert.Equal(t, nil, err)
	if len(channels) != 0 {
		time.Sleep(500 * time.Millisecond)
		channels, _ := e.Channels()
		assert.Equal(t, 0, len(channels), fmt.Sprintf("%#v", channels))
	}

	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			e.Unsubscribe("7-test-" + strconv.Itoa(i))
			e.Unsubscribe("8-test-" + strconv.Itoa(i))
			e.Subscribe("8-test-" + strconv.Itoa(i))
			e.Unsubscribe("9-test-" + strconv.Itoa(i))
			e.Subscribe("7-test-" + strconv.Itoa(i))
			e.Unsubscribe("8-test-" + strconv.Itoa(i))
			e.Subscribe("9-test-" + strconv.Itoa(i))
			e.Unsubscribe("9-test-" + strconv.Itoa(i))
			e.Unsubscribe("7-test-" + strconv.Itoa(i))
		}(i)
	}
	wg.Wait()
	channels, err = e.Channels()
	assert.Equal(t, nil, err)
	if len(channels) != 0 {
		time.Sleep(500 * time.Millisecond)
		channels, _ := e.Channels()
		assert.Equal(t, 0, len(channels), fmt.Sprintf("%#v", channels))
	}
}

func TestHandleClientMessage(t *testing.T) {
	c := dial()
	defer c.close()

	e := NewTestRedisEngine()

	shard := e.shards[0]

	ch := string("test")
	chID := shard.messageChannelID(ch)
	testMsg := proto.NewMessage(ch, []byte("{\"hello world\": true}"), "", nil)
	byteMessage, _ := testMsg.Marshal() // protobuf
	err := shard.handleRedisClientMessage(chID, byteMessage)
	assert.Equal(t, nil, err)
	rawData := raw.Raw([]byte("{}"))
	info := proto.NewClientInfo(string("1"), string("1"), rawData, rawData)
	testJoinMsg := proto.NewJoinMessage(ch, *info)
	byteJoinMsg, _ := testJoinMsg.Marshal()
	chID = shard.joinChannelID(ch)
	err = shard.handleRedisClientMessage(chID, byteJoinMsg)
	assert.Equal(t, nil, err)
	chID = shard.leaveChannelID(ch)
	testLeaveMsg := proto.NewLeaveMessage(ch, *info)
	byteLeaveMsg, _ := testLeaveMsg.Marshal()
	err = shard.handleRedisClientMessage(chID, byteLeaveMsg)
	assert.Equal(t, nil, err)
}

type testGetter struct {
	data map[string]interface{}
}

func newTestGetter(data map[string]interface{}) config.Getter {
	return &testGetter{
		data: data,
	}
}

func (g *testGetter) Get(key string) interface{} {
	d, ok := g.data[key]
	if ok {
		return d
	}
	return nil
}

func (g *testGetter) GetString(key string) string {
	d, ok := g.data[key]
	if ok {
		return d.(string)
	}
	return ""
}

func (g *testGetter) GetBool(key string) bool {
	d, ok := g.data[key]
	if ok {
		return d.(bool)
	}
	return false
}

func (g *testGetter) GetInt(key string) int {
	d, ok := g.data[key]
	if ok {
		return d.(int)
	}
	return 0
}

func (g *testGetter) IsSet(key string) bool {
	_, ok := g.data[key]
	return ok
}

func (g *testGetter) UnmarshalKey(key string, target interface{}) error {
	return nil
}

func TestShardConfigs(t *testing.T) {
	g := newTestGetter(map[string]interface{}{
		"redis_host":     "127.0.0.1",
		"redis_port":     "6379",
		"redis_url":      "",
		"redis_db":       "0",
		"redis_password": "",
	})
	confs, err := getConfigs(g)
	assert.Equal(t, nil, err)
	assert.Equal(t, 1, len(confs))
	assert.Equal(t, "127.0.0.1", confs[0].Host)
	assert.Equal(t, "6379", confs[0].Port)
	assert.Equal(t, "0", confs[0].DB)

	g = newTestGetter(map[string]interface{}{
		"redis_host":     "127.0.0.1",
		"redis_port":     "6379,6380",
		"redis_url":      "",
		"redis_db":       "0",
		"redis_password": "",
	})
	confs, err = getConfigs(g)
	assert.Equal(t, nil, err)
	assert.Equal(t, 2, len(confs))
	assert.Equal(t, "127.0.0.1", confs[0].Host)
	assert.Equal(t, "6379", confs[0].Port)
	assert.Equal(t, "0", confs[0].DB)
	assert.Equal(t, "127.0.0.1", confs[1].Host)
	assert.Equal(t, "6380", confs[1].Port)
	assert.Equal(t, "0", confs[1].DB)

	g = newTestGetter(map[string]interface{}{
		"redis_host":     "",
		"redis_port":     "",
		"redis_url":      "redis://:pass1@127.0.0.1:6379/0,redis://:pass2@127.0.0.2:6380/1",
		"redis_db":       "",
		"redis_password": "",
	})
	confs, err = getConfigs(g)
	assert.Equal(t, nil, err)
	assert.Equal(t, 2, len(confs))
	assert.Equal(t, "127.0.0.1", confs[0].Host)
	assert.Equal(t, "6379", confs[0].Port)
	assert.Equal(t, "0", confs[0].DB)
	assert.Equal(t, "pass1", confs[0].Password)
	assert.Equal(t, "127.0.0.2", confs[1].Host)
	assert.Equal(t, "6380", confs[1].Port)
	assert.Equal(t, "1", confs[1].DB)
	assert.Equal(t, "pass2", confs[1].Password)

	g = newTestGetter(map[string]interface{}{
		"redis_host":     "127.0.0.1,127.0.0.2",
		"redis_port":     "6379,6380",
		"redis_url":      "",
		"redis_db":       "",
		"redis_password": "",
	})
	confs, err = getConfigs(g)
	assert.Equal(t, nil, err)
	assert.Equal(t, 2, len(confs))
	assert.Equal(t, "127.0.0.1", confs[0].Host)
	assert.Equal(t, "6379", confs[0].Port)
	assert.Equal(t, "127.0.0.2", confs[1].Host)
	assert.Equal(t, "6380", confs[1].Port)

	g = newTestGetter(map[string]interface{}{
		"redis_host":     "127.0.0.1,127.0.0.2",
		"redis_port":     "6379,6380",
		"redis_url":      "",
		"redis_db":       "",
		"redis_password": "",
	})
	confs, err = getConfigs(g)
	assert.Equal(t, nil, err)
	assert.Equal(t, 2, len(confs))
	assert.Equal(t, "127.0.0.1", confs[0].Host)
	assert.Equal(t, "6379", confs[0].Port)
	assert.Equal(t, "127.0.0.2", confs[1].Host)
	assert.Equal(t, "6380", confs[1].Port)

	g = newTestGetter(map[string]interface{}{
		"redis_host":     "127.0.0.1,127.0.0.2",
		"redis_port":     "6379,6380,6381",
		"redis_url":      "",
		"redis_db":       "",
		"redis_password": "",
	})
	_, err = getConfigs(g)
	assert.NotEqual(t, nil, err, "too few hosts here actually")

	g = newTestGetter(map[string]interface{}{
		"redis_master_name": "mymaster,yourmaster",
	})
	_, err = getConfigs(g)
	assert.NotEqual(t, nil, err, "sentinels required here")

	g = newTestGetter(map[string]interface{}{
		"redis_master_name": "mymaster,yourmaster",
		"redis_sentinels":   "127.0.0.1:6380",
	})
	confs, err = getConfigs(g)
	assert.Equal(t, nil, err)
	assert.Equal(t, 2, len(confs))
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
			sameShards += 1
		}
	}
	sameFraction := float64(sameShards) * 100 / float64(len(chans))
	fmt.Printf("Same shards after resharding: %f%%\n", sameFraction)
	assert.True(t, sameFraction > 0.7)
}

func BenchmarkConsistentIndex(b *testing.B) {
	for i := 0; i < b.N; i++ {
		consistentIndex(strconv.Itoa(i), 4)
	}
}

func BenchmarkIndex(b *testing.B) {
	for i := 0; i < b.N; i++ {
		index(strconv.Itoa(i), 4)
	}
}

func BenchmarkPublish(b *testing.B) {
	e := NewTestRedisEngine()
	rawData := raw.Raw([]byte(`{"bench": true}`))
	msg := proto.Message{UID: "test UID", Channel: "channel", Data: rawData}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.PublishMessage(&msg, &proto.ChannelOptions{HistorySize: 0, HistoryLifetime: 0, HistoryDropInactive: false})
	}
}

func BenchmarkPublishWithHistory(b *testing.B) {
	e := NewTestRedisEngine()
	rawData := raw.Raw([]byte(`{"bench": true}`))
	msg := proto.Message{UID: "test UID", Channel: "channel", Data: rawData}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.PublishMessage(&msg, &proto.ChannelOptions{HistorySize: 10, HistoryLifetime: 300, HistoryDropInactive: false})
	}
}

func BenchmarkOpAddPresence(b *testing.B) {
	e := NewTestRedisEngine()
	expire := int(e.node.Config().PresenceExpireInterval.Seconds())
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := e.AddPresence("channel", "uid", proto.ClientInfo{}, expire)
		if err != nil {
			panic(err)
		}
	}
}

func BenchmarkOpAddPresenceParallel(b *testing.B) {
	e := NewTestRedisEngine()
	expire := int(e.node.Config().PresenceExpireInterval.Seconds())
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			err := e.AddPresence("channel", "uid", proto.ClientInfo{}, expire)
			if err != nil {
				panic(err)
			}
		}
	})
}

func BenchmarkOpPresence(b *testing.B) {
	e := NewTestRedisEngine()
	e.AddPresence("channel", "uid", proto.ClientInfo{}, 300)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := e.Presence("channel")
		if err != nil {
			panic(err)
		}
	}
}

func BenchmarkOpPresenceParallel(b *testing.B) {
	e := NewTestRedisEngine()
	e.AddPresence("channel", "uid", proto.ClientInfo{}, 300)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := e.Presence("channel")
			if err != nil {
				panic(err)
			}
		}
	})
}

func BenchmarkOpHistory(b *testing.B) {
	e := NewTestRedisEngine()
	rawData := raw.Raw([]byte("{}"))
	msg := proto.Message{UID: "test UID", Channel: "channel", Data: rawData}
	<-e.PublishMessage(&msg, &proto.ChannelOptions{HistorySize: 4, HistoryLifetime: 300, HistoryDropInactive: false})
	<-e.PublishMessage(&msg, &proto.ChannelOptions{HistorySize: 4, HistoryLifetime: 300, HistoryDropInactive: false})
	<-e.PublishMessage(&msg, &proto.ChannelOptions{HistorySize: 4, HistoryLifetime: 300, HistoryDropInactive: false})
	<-e.PublishMessage(&msg, &proto.ChannelOptions{HistorySize: 4, HistoryLifetime: 300, HistoryDropInactive: false})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := e.History("channel", 0)
		if err != nil {
			panic(err)
		}

	}
}

func BenchmarkOpHistoryParallel(b *testing.B) {
	e := NewTestRedisEngine()
	rawData := raw.Raw([]byte("{}"))
	msg := proto.Message{UID: "test UID", Channel: "channel", Data: rawData}
	<-e.PublishMessage(&msg, &proto.ChannelOptions{HistorySize: 4, HistoryLifetime: 300, HistoryDropInactive: false})
	<-e.PublishMessage(&msg, &proto.ChannelOptions{HistorySize: 4, HistoryLifetime: 300, HistoryDropInactive: false})
	<-e.PublishMessage(&msg, &proto.ChannelOptions{HistorySize: 4, HistoryLifetime: 300, HistoryDropInactive: false})
	<-e.PublishMessage(&msg, &proto.ChannelOptions{HistorySize: 4, HistoryLifetime: 300, HistoryDropInactive: false})
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := e.History("channel", 0)
			if err != nil {
				panic(err)
			}
		}
	})
}
