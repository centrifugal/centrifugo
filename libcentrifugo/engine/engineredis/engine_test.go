package engineredis

import (
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/libcentrifugo/engine"
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

func testRedisEngine(n *node.Node) engine.Engine {
	redisConf := &RedisEngineConfig{
		Host:         testRedisHost,
		Port:         testRedisPort,
		Password:     testRedisPassword,
		DB:           testRedisDB,
		URL:          testRedisURL,
		PoolSize:     testRedisPoolSize,
		API:          true,
		NumAPIShards: testRedisNumAPIShards,
	}
	e, _ := NewRedisEngine(n, redisConf)
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

	testMsg := newTestMessage()

	err = <-e.publishMessage(Channel("channel"), testMsg, nil)
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, e.subscribe(Channel("channel")))
	// Now we've subscribed...
	err = <-e.publishMessage(Channel("channel"), testMsg, nil)
	assert.Equal(t, nil, e.unsubscribe(Channel("channel")))

	// test adding presence
	assert.Equal(t, nil, e.addPresence(Channel("channel"), "uid", ClientInfo{}))

	// test getting presence
	p, err := e.presence(Channel("channel"))
	assert.Equal(t, nil, err)
	assert.Equal(t, 1, len(p))

	// test removing presence
	err = e.removePresence(Channel("channel"), "uid")
	assert.Equal(t, nil, err)

	rawData := raw.Raw([]byte("{}"))
	msg := Message{UID: "test UID", Data: &rawData}

	// test adding history
	assert.Equal(t, nil, <-e.publishMessage(Channel("channel"), &msg, &ChannelOptions{HistorySize: 4, HistoryLifetime: 1, HistoryDropInactive: false}))
	h, err := e.history(Channel("channel"), 0)
	assert.Equal(t, nil, err)
	assert.Equal(t, 1, len(h))
	assert.Equal(t, h[0].UID, "test UID")

	// test history limit
	assert.Equal(t, nil, <-e.publishMessage(Channel("channel"), &msg, &ChannelOptions{HistorySize: 4, HistoryLifetime: 1, HistoryDropInactive: false}))
	assert.Equal(t, nil, <-e.publishMessage(Channel("channel"), &msg, &ChannelOptions{HistorySize: 4, HistoryLifetime: 1, HistoryDropInactive: false}))
	assert.Equal(t, nil, <-e.publishMessage(Channel("channel"), &msg, &ChannelOptions{HistorySize: 4, HistoryLifetime: 1, HistoryDropInactive: false}))
	h, err = e.history(Channel("channel"), 2)
	assert.Equal(t, nil, err)
	assert.Equal(t, 2, len(h))

	// test history limit greater than history size
	assert.Equal(t, nil, <-e.publishMessage(Channel("channel"), &msg, &ChannelOptions{HistorySize: 1, HistoryLifetime: 1, HistoryDropInactive: false}))
	assert.Equal(t, nil, <-e.publishMessage(Channel("channel"), &msg, &ChannelOptions{HistorySize: 1, HistoryLifetime: 1, HistoryDropInactive: false}))
	assert.Equal(t, nil, <-e.publishMessage(Channel("channel"), &msg, &ChannelOptions{HistorySize: 1, HistoryLifetime: 1, HistoryDropInactive: false}))
	h, err = e.history(Channel("channel"), 2)

	// HistoryDropInactive tests - new channel to avoid conflicts with test above
	// 1. add history with DropInactive = true should be a no-op if history is empty
	assert.Equal(t, nil, <-e.publishMessage(Channel("channel-2"), &msg, &ChannelOptions{HistorySize: 2, HistoryLifetime: 5, HistoryDropInactive: true}))
	h, err = e.history(Channel("channel-2"), 0)
	assert.Equal(t, nil, err)
	assert.Equal(t, 0, len(h))

	// 2. add history with DropInactive = false should always work
	assert.Equal(t, nil, <-e.publishMessage(Channel("channel-2"), &msg, &ChannelOptions{HistorySize: 2, HistoryLifetime: 5, HistoryDropInactive: false}))
	h, err = e.history(Channel("channel-2"), 0)
	assert.Equal(t, nil, err)
	assert.Equal(t, 1, len(h))

	// 3. add with DropInactive = true should work immediately since there should be something in history
	// for 5 seconds from above
	assert.Equal(t, nil, <-e.publishMessage(Channel("channel-2"), &msg, &ChannelOptions{HistorySize: 2, HistoryLifetime: 5, HistoryDropInactive: true}))
	h, err = e.history(Channel("channel-2"), 0)
	assert.Equal(t, nil, err)
	assert.Equal(t, 2, len(h))

	// test API
	apiKey := e.app.config.ChannelPrefix + "." + "api"
	_, err = c.Conn.Do("LPUSH", apiKey, []byte("{}"))
	assert.Equal(t, nil, err)

	// test API shards
	for i := 0; i < testRedisNumAPIShards; i++ {
		queueKey := fmt.Sprintf("%s.%d", apiKey, i)
		_, err = c.Conn.Do("LPUSH", queueKey, []byte("{}"))
		assert.Equal(t, nil, err)
	}

	// test publishing control message.
	controlMsg := newControlMessage("uid", "method", []byte("{}"))
	err = <-e.publishControl(controlMsg)
	assert.Equal(t, nil, err)

	// test publishing admin message.
	adminMessage := newAdminMessage("method", []byte("{}"))
	err = <-e.publishAdmin(adminMessage)
	assert.Equal(t, nil, err)

	rawInfoData := raw.Raw([]byte("{}"))
	clientInfo := newClientInfo(UserID("1"), ConnID("1"), &rawInfoData, &rawInfoData)

	// test publishing join message.
	joinMessage := newJoinMessage(Channel("test"), *clientInfo)
	err = <-e.publishJoin(Channel("test"), joinMessage)
	assert.Equal(t, nil, err)

	// test publishing leave message.
	leaveMessage := newLeaveMessage(Channel("test"), *clientInfo)
	err = <-e.publishLeave(Channel("test"), leaveMessage)
	assert.Equal(t, nil, err)
}

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

func TestHandleClientMessage(t *testing.T) {
	app := testApp()
	e := testRedisEngine(app)
	ch := Channel("test")
	chID := e.messageChannelID(ch)
	testMsg := newMessage(ch, []byte("{\"hello world\": true}"), "", nil)
	byteMessage, _ := testMsg.Marshal() // protobuf
	err := e.handleRedisClientMessage(chID, byteMessage)
	assert.Equal(t, nil, err)
	rawData := raw.Raw([]byte("{}"))
	info := newClientInfo(UserID("1"), ConnID("1"), &rawData, &rawData)
	testJoinMsg := newJoinMessage(ch, *info)
	byteJoinMsg, _ := testJoinMsg.Marshal()
	chID = e.joinChannelID(ch)
	err = e.handleRedisClientMessage(chID, byteJoinMsg)
	assert.Equal(t, nil, err)
	chID = e.leaveChannelID(ch)
	testLeaveMsg := newLeaveMessage(ch, *info)
	byteLeaveMsg, _ := testLeaveMsg.Marshal()
	err = e.handleRedisClientMessage(chID, byteLeaveMsg)
	assert.Equal(t, nil, err)
}

func TestEngineEncodeDecode(t *testing.T) {
	message := proto.NewMessage(Channel("encode_decode_test"), []byte("{}"), "", nil)
	byteMessage, err := encodeEngineClientMessage(message)
	assert.Equal(t, nil, err)
	assert.True(t, bytes.Contains(byteMessage, []byte("encode_decode_test")))

	decodedMessage, err := decodeEngineClientMessage(byteMessage)
	assert.Equal(t, nil, err)
	assert.Equal(t, "encode_decode_test", decodedMessage.Channel)

	joinMessage := newJoinMessage(Channel("encode_decode_test"), ClientInfo{})
	byteMessage, err = encodeEngineJoinMessage(joinMessage)
	assert.Equal(t, nil, err)
	assert.True(t, bytes.Contains(byteMessage, []byte("encode_decode_test")))

	decodedJoinMessage, err := decodeEngineJoinMessage(byteMessage)
	assert.Equal(t, nil, err)
	assert.Equal(t, "encode_decode_test", decodedJoinMessage.Channel)

	leaveMessage := newLeaveMessage(Channel("encode_decode_test"), ClientInfo{})
	byteMessage, err = encodeEngineLeaveMessage(leaveMessage)
	assert.Equal(t, nil, err)
	assert.True(t, bytes.Contains(byteMessage, []byte("encode_decode_test")))

	decodedLeaveMessage, err := decodeEngineLeaveMessage(byteMessage)
	assert.Equal(t, nil, err)
	assert.Equal(t, "encode_decode_test", decodedLeaveMessage.Channel)

	controlMessage := newControlMessage("test_encode_decode_uid", "ping", []byte("{}"))
	byteMessage, err = encodeEngineControlMessage(controlMessage)
	assert.Equal(t, nil, err)
	assert.True(t, bytes.Contains(byteMessage, []byte("test_encode_decode_uid")))

	decodedControlMessage, err := decodeEngineControlMessage(byteMessage)
	assert.Equal(t, nil, err)
	assert.Equal(t, "test_encode_decode_uid", decodedControlMessage.UID)

	adminMessage := newAdminMessage("test_encode_decode", []byte("{}"))
	byteMessage, err = encodeEngineAdminMessage(adminMessage)
	assert.Equal(t, nil, err)
	assert.True(t, bytes.Contains(byteMessage, []byte("test_encode_decode")))

	decodedAdminMessage, err := decodeEngineAdminMessage(byteMessage)
	assert.Equal(t, nil, err)
	assert.Equal(t, "test_encode_decode", decodedAdminMessage.Method)
}
