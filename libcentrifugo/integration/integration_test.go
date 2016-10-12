package integration

import (
	"encoding/json"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/libcentrifugo/engine/enginememory"
	//"github.com/centrifugal/centrifugo/libcentrifugo/engine/engineredis"
	"github.com/centrifugal/centrifugo/libcentrifugo/node"
	"github.com/centrifugal/centrifugo/libcentrifugo/proto"
	"github.com/stretchr/testify/assert"
)

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

func newTestConfig() node.Config {
	c := *node.DefaultConfig
	var ns []node.Namespace
	ns = append(ns, getTestNamespace("test"))
	c.Namespaces = ns
	c.Secret = "secret"
	c.ChannelOptions = getTestChannelOptions()
	return c
}

type testSession struct {
	sink   chan []byte
	closed bool
}

func (t *testSession) Send(msg []byte) error {
	if t.sink != nil {
		t.sink <- msg
	}
	return nil
}

func (t *testSession) Close(status uint32, reason string) error {
	t.closed = true
	return nil
}

func testMemoryNode() *node.Node {
	return testMemoryNodeWithConfig(nil)
}

func testMemoryNodeWithConfig(c *node.Config) *node.Node {
	if c == nil {
		conf := newTestConfig()
		c = &conf
	}
	n := node.New(c)
	engn, _ := enginememory.NewMemoryEngine(n, nil)
	n.Run(&node.RunOptions{Engine: engn})
	return n
}

func testMemoryNodeWithClients(nChannels int, nChannelClients int) *node.Node {
	n := testMemoryNode()
	createTestClients(n, nChannels, nChannelClients, nil)
	return n
}

func newTestClient(n *node.Node, sess node.Session) node.ClientConn {
	c, _ := n.NewClient(sess, nil)
	return c
}

func createTestClients(n *node.Node, nChannels, nChannelClients int, sink chan []byte) {
	config := n.Config()
	config.Insecure = true
	n.SetConfig(&config)
	for i := 0; i < nChannelClients; i++ {
		sess := &testSession{}
		if sink != nil {
			sess.sink = sink
		}
		c := newTestClient(n, sess)
		cmd := proto.ConnectClientCommand{
			User: proto.UserID(fmt.Sprintf("user-%d", i)),
		}
		msg, _ := json.Marshal(cmd)
		err := c.Handle(msg)
		if err != nil {
			panic(err)
		}
		//if resp.(*proto.ClientConnectResponse).ResponseError.Err != nil {
		//	panic(resp.(*proto.ClientConnectResponse).ResponseError.Err)
		//}
		for j := 0; j < nChannels; j++ {
			cmd := proto.SubscribeClientCommand{
				Channel: proto.Channel(fmt.Sprintf("channel-%d", j)),
			}
			msg, _ := json.Marshal(cmd)
			err := c.Handle(msg)
			if err != nil {
				panic(err)
			}
			//if resp.(*proto.ClientSubscribeResponse).ResponseError.Err != nil {
			//	panic(resp.(*proto.ClientSubscribeResponse).ResponseError.Err)
			//}
		}
	}
}

// BenchmarkPubSubMessageReceive allows to estimate how many new messages we can convert to client JSON messages.
func BenchmarkPubSubMessageReceive(b *testing.B) {
	app := testMemoryNode()

	// create one client so clientMsg really marshal into client response JSON.
	c, _ := app.NewClient(&testSession{}, nil)

	messagePoolSize := 1000

	messagePool := make([][]byte, messagePoolSize)

	for i := 0; i < len(messagePool); i++ {
		channel := proto.Channel("test" + strconv.Itoa(i))
		// subscribe client to channel so we need to encode message to JSON
		app.ClientHub().AddSub(channel, c)
		// add message to pool so we have messages for different channels.
		testMsg := proto.NewMessage(channel, []byte("{\"hello world\": true}"), "", nil)
		byteMessage, _ := testMsg.Marshal() // protobuf
		messagePool[i] = byteMessage
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var msg proto.Message
		err := msg.Unmarshal(messagePool[i%len(messagePool)]) // unmarshal from protobuf
		if err != nil {
			panic(err)
		}
		err = app.ClientMsg(proto.Channel("test"+strconv.Itoa(i%len(messagePool))), &msg)
		if err != nil {
			panic(err)
		}
	}
}

// BenchmarkClientMsg allows to measue performance of marshaling messages into client response JSON.
func BenchmarkClientMsg(b *testing.B) {
	app := testMemoryNode()
	// create one client so clientMsg really marshal into client response JSON.
	c, _ := app.NewClient(&testSession{}, nil)
	messagePoolSize := 1000
	messagePool := make([]*proto.Message, messagePoolSize)

	for i := 0; i < len(messagePool); i++ {
		channel := proto.Channel("test" + strconv.Itoa(i))
		// subscribe client to channel so we need to encode message to JSON
		app.ClientHub().AddSub(channel, c)
		// add message to pool so we have messages for different channels.
		testMsg := proto.NewMessage(channel, []byte("{\"hello world\": true}"), "", nil)
		messagePool[i] = testMsg
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := app.ClientMsg(proto.Channel("test"+strconv.Itoa(i%len(messagePool))), messagePool[i%len(messagePool)])
		if err != nil {
			panic(err)
		}
	}
}

// BenchmarkEngineMessageUnmarshal shows how fast we can decode messages coming from engine PUB/SUB.
func BenchmarkEngineMessageUnmarshal(b *testing.B) {
	messagePoolSize := 1000
	messagePool := make([][]byte, messagePoolSize)

	for i := 0; i < len(messagePool); i++ {
		channel := proto.Channel("test" + strconv.Itoa(i))
		// add message to pool so we have messages for different channels.
		testMsg := proto.NewMessage(channel, []byte("{\"hello world\": true}"), "", nil)
		byteMessage, _ := testMsg.Marshal() // protobuf
		messagePool[i] = byteMessage
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var msg proto.Message
		err := msg.Unmarshal(messagePool[i%len(messagePool)]) // unmarshal from protobuf
		if err != nil {
			panic(err)
		}
	}
}

// BenchmarkReceiveBroadcast measures how fast we can broadcast messages received
// from engine into client channels in case of reasonably large different channel
// amount.
func BenchmarkReceiveBroadcast(b *testing.B) {
	nChannels := 1000
	nClients := 1000
	nCommands := 10000
	nMessages := nCommands * nClients
	sink := make(chan []byte, nMessages)
	app := testMemoryNode()
	// Use very large initial capacity so that queue resizes do not affect benchmark.
	config := app.Config()
	config.ClientQueueInitialCapacity = 4000
	config.ClientChannelLimit = 1000
	app.SetConfig(&config)
	createTestClients(app, nChannels, nClients, sink)

	type received struct {
		ch   proto.Channel
		data proto.Message
	}

	var inputData []received

	for i := 0; i < nCommands; i++ {
		suffix := i % nChannels
		ch := proto.Channel(fmt.Sprintf("channel-%d", suffix))
		msg := proto.NewMessage(ch, []byte("{}"), "", nil)
		inputData = append(inputData, received{ch, *msg})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {

		done := make(chan struct{})

		go func() {
			count := 0
			for {
				select {
				case <-sink:
					count++
				}
				if count == nMessages {
					close(done)
					return
				}
			}
		}()

		go func() {
			for _, item := range inputData {
				app.ClientMsg(item.ch, &item.data)
			}
		}()

		<-done
	}
	b.StopTimer()
}

func TestPublish(t *testing.T) {
	// Custom config
	c := newTestConfig()

	// Set custom options for default namespace
	c.ChannelOptions.HistoryLifetime = 10
	c.ChannelOptions.HistorySize = 2
	c.ChannelOptions.HistoryDropInactive = true

	app := testMemoryNodeWithConfig(&c)
	createTestClients(app, 10, 1, nil)
	data, _ := json.Marshal(map[string]string{"test": "publish"})
	err := app.Publish(proto.Channel("channel-0"), data, proto.ConnID(""), nil)
	assert.Nil(t, err)

	// Check publish to subscribed channels did result in saved history
	hist, err := app.History(proto.Channel("channel-0"))
	assert.Nil(t, err)
	assert.Equal(t, 1, len(hist))

	// Publishing to a channel no one is subscribed to should be a no-op
	err = app.Publish(proto.Channel("some-other-channel"), data, proto.ConnID(""), nil)
	assert.Nil(t, err)

	hist, err = app.History(proto.Channel("some-other-channel"))
	assert.Nil(t, err)
	assert.Equal(t, 0, len(hist))
}
