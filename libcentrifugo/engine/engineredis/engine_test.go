package engineredis

import (
	"bytes"
	"errors"
	"fmt"
	"net"
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
	testRedisPoolSize     = 5
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
	return proto.NewMessage(proto.Channel("test"), []byte("{}"), "", nil)
}

func NewTestRedisEngine() *RedisEngine {
	c := NewTestConfig()
	n := node.New("", c)
	redisConf := &RedisEngineConfig{
		Host:         testRedisHost,
		Port:         testRedisPort,
		Password:     testRedisPassword,
		DB:           testRedisDB,
		URL:          testRedisURL,
		PoolSize:     testRedisPoolSize,
		API:          true,
		NumAPIShards: testRedisNumAPIShards,
		Prefix:       "centrifugotest",
	}
	e, _ := NewRedisEngine(n, redisConf)
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

	err = <-e.PublishMessage(proto.Channel("channel"), testMsg, nil)
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, e.Subscribe(proto.Channel("channel")))
	// Now we've subscribed...
	err = <-e.PublishMessage(proto.Channel("channel"), testMsg, nil)
	assert.Equal(t, nil, e.Unsubscribe(proto.Channel("channel")))

	// test adding presence
	assert.Equal(t, nil, e.AddPresence(proto.Channel("channel"), "uid", proto.ClientInfo{}, int(e.node.Config().PresenceExpireInterval.Seconds())))

	// test getting presence
	p, err := e.Presence(proto.Channel("channel"))
	assert.Equal(t, nil, err)
	assert.Equal(t, 1, len(p))

	// test removing presence
	err = e.RemovePresence(proto.Channel("channel"), "uid")
	assert.Equal(t, nil, err)

	rawData := raw.Raw([]byte("{}"))
	msg := proto.Message{UID: "test UID", Data: &rawData}

	// test adding history
	assert.Equal(t, nil, <-e.PublishMessage(proto.Channel("channel"), &msg, &proto.ChannelOptions{HistorySize: 4, HistoryLifetime: 1, HistoryDropInactive: false}))
	h, err := e.History(proto.Channel("channel"), 0)
	assert.Equal(t, nil, err)
	assert.Equal(t, 1, len(h))
	assert.Equal(t, h[0].UID, "test UID")

	// test history limit
	assert.Equal(t, nil, <-e.PublishMessage(proto.Channel("channel"), &msg, &proto.ChannelOptions{HistorySize: 4, HistoryLifetime: 1, HistoryDropInactive: false}))
	assert.Equal(t, nil, <-e.PublishMessage(proto.Channel("channel"), &msg, &proto.ChannelOptions{HistorySize: 4, HistoryLifetime: 1, HistoryDropInactive: false}))
	assert.Equal(t, nil, <-e.PublishMessage(proto.Channel("channel"), &msg, &proto.ChannelOptions{HistorySize: 4, HistoryLifetime: 1, HistoryDropInactive: false}))
	h, err = e.History(proto.Channel("channel"), 2)
	assert.Equal(t, nil, err)
	assert.Equal(t, 2, len(h))

	// test history limit greater than history size
	assert.Equal(t, nil, <-e.PublishMessage(proto.Channel("channel"), &msg, &proto.ChannelOptions{HistorySize: 1, HistoryLifetime: 1, HistoryDropInactive: false}))
	assert.Equal(t, nil, <-e.PublishMessage(proto.Channel("channel"), &msg, &proto.ChannelOptions{HistorySize: 1, HistoryLifetime: 1, HistoryDropInactive: false}))
	assert.Equal(t, nil, <-e.PublishMessage(proto.Channel("channel"), &msg, &proto.ChannelOptions{HistorySize: 1, HistoryLifetime: 1, HistoryDropInactive: false}))
	h, err = e.History(proto.Channel("channel"), 2)

	// HistoryDropInactive tests - new channel to avoid conflicts with test above
	// 1. add history with DropInactive = true should be a no-op if history is empty
	assert.Equal(t, nil, <-e.PublishMessage(proto.Channel("channel-2"), &msg, &proto.ChannelOptions{HistorySize: 2, HistoryLifetime: 5, HistoryDropInactive: true}))
	h, err = e.History(proto.Channel("channel-2"), 0)
	assert.Equal(t, nil, err)
	assert.Equal(t, 0, len(h))

	// 2. add history with DropInactive = false should always work
	assert.Equal(t, nil, <-e.PublishMessage(proto.Channel("channel-2"), &msg, &proto.ChannelOptions{HistorySize: 2, HistoryLifetime: 5, HistoryDropInactive: false}))
	h, err = e.History(proto.Channel("channel-2"), 0)
	assert.Equal(t, nil, err)
	assert.Equal(t, 1, len(h))

	// 3. add with DropInactive = true should work immediately since there should be something in history
	// for 5 seconds from above
	assert.Equal(t, nil, <-e.PublishMessage(proto.Channel("channel-2"), &msg, &proto.ChannelOptions{HistorySize: 2, HistoryLifetime: 5, HistoryDropInactive: true}))
	h, err = e.History(proto.Channel("channel-2"), 0)
	assert.Equal(t, nil, err)
	assert.Equal(t, 2, len(h))

	// test API
	apiKey := e.config.Prefix + "." + "api"
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
	clientInfo := proto.NewClientInfo(proto.UserID("1"), proto.ConnID("1"), &rawInfoData, &rawInfoData)

	// test publishing join message.
	joinMessage := proto.NewJoinMessage(proto.Channel("test"), *clientInfo)
	err = <-e.PublishJoin(proto.Channel("test"), joinMessage)
	assert.Equal(t, nil, err)

	// test publishing leave message.
	leaveMessage := proto.NewLeaveMessage(proto.Channel("test"), *clientInfo)
	err = <-e.PublishLeave(proto.Channel("test"), leaveMessage)
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

	ch := proto.Channel("test")
	chID := e.messageChannelID(ch)
	testMsg := proto.NewMessage(ch, []byte("{\"hello world\": true}"), "", nil)
	byteMessage, _ := testMsg.Marshal() // protobuf
	err := e.handleRedisClientMessage(chID, byteMessage)
	assert.Equal(t, nil, err)
	rawData := raw.Raw([]byte("{}"))
	info := proto.NewClientInfo(proto.UserID("1"), proto.ConnID("1"), &rawData, &rawData)
	testJoinMsg := proto.NewJoinMessage(ch, *info)
	byteJoinMsg, _ := testJoinMsg.Marshal()
	chID = e.joinChannelID(ch)
	err = e.handleRedisClientMessage(chID, byteJoinMsg)
	assert.Equal(t, nil, err)
	chID = e.leaveChannelID(ch)
	testLeaveMsg := proto.NewLeaveMessage(ch, *info)
	byteLeaveMsg, _ := testLeaveMsg.Marshal()
	err = e.handleRedisClientMessage(chID, byteLeaveMsg)
	assert.Equal(t, nil, err)
}

func TestEngineEncodeDecode(t *testing.T) {
	message := proto.NewMessage(proto.Channel("encode_decode_test"), []byte("{}"), "", nil)
	byteMessage, err := encodeEngineClientMessage(message)
	assert.Equal(t, nil, err)
	assert.True(t, bytes.Contains(byteMessage, []byte("encode_decode_test")))

	decodedMessage, err := decodeEngineClientMessage(byteMessage)
	assert.Equal(t, nil, err)
	assert.Equal(t, "encode_decode_test", decodedMessage.Channel)

	joinMessage := proto.NewJoinMessage(proto.Channel("encode_decode_test"), proto.ClientInfo{})
	byteMessage, err = encodeEngineJoinMessage(joinMessage)
	assert.Equal(t, nil, err)
	assert.True(t, bytes.Contains(byteMessage, []byte("encode_decode_test")))

	decodedJoinMessage, err := decodeEngineJoinMessage(byteMessage)
	assert.Equal(t, nil, err)
	assert.Equal(t, "encode_decode_test", decodedJoinMessage.Channel)

	leaveMessage := proto.NewLeaveMessage(proto.Channel("encode_decode_test"), proto.ClientInfo{})
	byteMessage, err = encodeEngineLeaveMessage(leaveMessage)
	assert.Equal(t, nil, err)
	assert.True(t, bytes.Contains(byteMessage, []byte("encode_decode_test")))

	decodedLeaveMessage, err := decodeEngineLeaveMessage(byteMessage)
	assert.Equal(t, nil, err)
	assert.Equal(t, "encode_decode_test", decodedLeaveMessage.Channel)

	controlMessage := proto.NewControlMessage("test_encode_decode_uid", "ping", []byte("{}"))
	byteMessage, err = encodeEngineControlMessage(controlMessage)
	assert.Equal(t, nil, err)
	assert.True(t, bytes.Contains(byteMessage, []byte("test_encode_decode_uid")))

	decodedControlMessage, err := decodeEngineControlMessage(byteMessage)
	assert.Equal(t, nil, err)
	assert.Equal(t, "test_encode_decode_uid", decodedControlMessage.UID)

	adminMessage := proto.NewAdminMessage("test_encode_decode", []byte("{}"))
	byteMessage, err = encodeEngineAdminMessage(adminMessage)
	assert.Equal(t, nil, err)
	assert.True(t, bytes.Contains(byteMessage, []byte("test_encode_decode")))

	decodedAdminMessage, err := decodeEngineAdminMessage(byteMessage)
	assert.Equal(t, nil, err)
	assert.Equal(t, "test_encode_decode", decodedAdminMessage.Method)
}
