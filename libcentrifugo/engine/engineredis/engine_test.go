package engineredis

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"testing"
	"time"

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
	testRedisPoolSize     = 13
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
		Prefix:       "centrifugotest",
	}
	e, _ := New(n, []*ShardConfig{redisConf})
	err := n.Run(&node.RunOptions{Engine: e})
	if err != nil {
		panic(err)
	}
	return e.(*RedisEngine)
}

func TestRedisEngine(t *testing.T) {
	c := dial()
	defer c.close()

	e := NewTestRedisEngine()

	err := e.Run()
	assert.Equal(t, nil, err)
	assert.Equal(t, e.Name(), "Redis")

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

/*
func TestRedisChannels(t *testing.T) {
	c := dial()
	defer c.close()
	app := testRedisApp()
	err := app.Run()
	assert.Nil(t, err)
	channels, err := app.engine.channels()
	assert.Equal(t, nil, err)
	assert.Equal(t, 0, len(channels))
	createTestClients(app, 10, 1, nil)
	channels, err = app.engine.channels()
	assert.Equal(t, nil, err)
	assert.Equal(t, 10, len(channels))
}
*/

func TestHandleClientMessage(t *testing.T) {
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

func TestConsistentIndex(t *testing.T) {
	chans := []string{"chan1", "chan2", "chan3", "chan4", "chan5", "chan6", "chan7", "chan8"}
	for _, ch := range chans {
		bucket := consistentIndex(ch, 2)
		assert.True(t, bucket >= 0)
		assert.True(t, bucket < 2)
	}
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
