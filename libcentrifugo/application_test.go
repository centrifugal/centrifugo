package libcentrifugo

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/Godeps/_workspace/src/github.com/stretchr/testify/assert"
)

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

func testApp() *Application {
	c := newTestConfig()
	app, _ := NewApplication(&c)
	app.SetEngine(newTestEngine())
	return app
}

func testMemoryApp() *Application {
	return testMemoryAppWithConfig(nil)
}

func testMemoryAppWithConfig(c *Config) *Application {
	if c == nil {
		conf := newTestConfig()
		c = &conf
	}
	app, _ := NewApplication(c)
	app.SetEngine(NewMemoryEngine(app))
	return app
}

func testRedisApp() *Application {
	return testRedisAppWithConfig(nil)
}

func testRedisAppWithConfig(c *Config) *Application {
	if c == nil {
		conf := newTestConfig()
		c = &conf
	}
	app, _ := NewApplication(c)
	app.SetEngine(testRedisEngine(app))
	return app
}

func newTestClient(app *Application, sess session) *client {
	c, _ := newClient(app, sess)
	return c
}

func createTestClients(app *Application, nChannels, nChannelClients int, sink chan []byte) {
	app.config.Insecure = true
	for i := 0; i < nChannelClients; i++ {
		sess := &testSession{}
		if sink != nil {
			sess.sink = sink
		}
		c := newTestClient(app, sess)
		cmd := ConnectClientCommand{
			User: UserID(fmt.Sprintf("user-%d", i)),
		}
		resp, err := c.connectCmd(&cmd)
		if err != nil {
			panic(err)
		}
		if resp.err != nil {
			panic(resp.err)
		}
		for j := 0; j < nChannels; j++ {
			cmd := SubscribeClientCommand{
				Channel: Channel(fmt.Sprintf("channel-%d", j)),
			}
			resp, err = c.subscribeCmd(&cmd)
			if err != nil {
				panic(err)
			}
			if resp.err != nil {
				panic(resp.err)
			}
		}
	}
}

func testMemoryAppWithClients(nChannels int, nChannelClients int) *Application {
	app := testMemoryApp()
	createTestClients(app, nChannels, nChannelClients, nil)
	return app
}

func TestChannelID(t *testing.T) {
	app := testApp()
	chID := app.channelID("channel")
	assert.Equal(t, chID, ChannelID(defaultChannelPrefix+".channel.channel"))
}

func TestUserAllowed(t *testing.T) {
	app := testApp()
	assert.Equal(t, true, app.userAllowed("channel#1", "1"))
	assert.Equal(t, true, app.userAllowed("channel", "1"))
	assert.Equal(t, false, app.userAllowed("channel#1", "2"))
	assert.Equal(t, true, app.userAllowed("channel#1,2", "1"))
	assert.Equal(t, true, app.userAllowed("channel#1,2", "2"))
	assert.Equal(t, false, app.userAllowed("channel#1,2", "3"))
}

func TestSetConfig(t *testing.T) {
	app := testApp()
	c := newTestConfig()
	app.SetConfig(&c)
}

func TestAdminAuthToken(t *testing.T) {
	app := testApp()
	// first without secret set
	err := app.checkAdminAuthToken("")
	assert.Equal(t, ErrUnauthorized, err)

	// no web secret set
	token, err := app.adminAuthToken()
	if err == nil {
		println(app.config.WebSecret)
		println(token)
	}
	assert.Equal(t, ErrInternalServerError, err)

	app.Lock()
	app.config.WebSecret = "secret"
	app.Unlock()

	err = app.checkAdminAuthToken("")
	assert.Equal(t, ErrUnauthorized, err)

	token, err = app.adminAuthToken()
	assert.Equal(t, nil, err)
	assert.True(t, len(token) > 0)
	err = app.checkAdminAuthToken(token)
	assert.Equal(t, nil, err)

}

func TestClientAllowed(t *testing.T) {
	app := testApp()
	assert.Equal(t, true, app.clientAllowed("channel&67330d48-f668-4916-758b-f4eb1dd5b41d", ConnID("67330d48-f668-4916-758b-f4eb1dd5b41d")))
	assert.Equal(t, true, app.clientAllowed("channel", ConnID("67330d48-f668-4916-758b-f4eb1dd5b41d")))
	assert.Equal(t, false, app.clientAllowed("channel&long-client-id", ConnID("wrong-client-id")))
}

func TestNamespaceKey(t *testing.T) {
	app := testApp()
	assert.Equal(t, NamespaceKey("ns"), app.namespaceKey("ns:channel"))
	assert.Equal(t, NamespaceKey(""), app.namespaceKey("channel"))
	assert.Equal(t, NamespaceKey("ns"), app.namespaceKey("ns:channel:opa"))
	assert.Equal(t, NamespaceKey("ns"), app.namespaceKey("ns::channel"))
}

func BenchmarkNamespaceKey(b *testing.B) {
	app := testApp()
	ch := Channel("test")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		app.namespaceKey(ch)
	}
}

func testPingControlCmd(uid string) []byte {
	params := json.RawMessage([]byte("{}"))
	cmd := controlCommand{
		UID:    uid,
		Method: "ping",
		Params: &params,
	}
	cmdBytes, _ := json.Marshal(cmd)
	return cmdBytes
}

func testUnsubscribeControlCmd(uid string) []byte {
	params := json.RawMessage([]byte("{}"))
	cmd := controlCommand{
		UID:    uid,
		Method: "unsubscribe",
		Params: &params,
	}
	cmdBytes, _ := json.Marshal(cmd)
	return cmdBytes
}

func testDisconnectControlCmd(uid string) []byte {
	params := json.RawMessage([]byte("{}"))
	cmd := controlCommand{
		UID:    uid,
		Method: "disconnect",
		Params: &params,
	}
	cmdBytes, _ := json.Marshal(cmd)
	return cmdBytes
}

func testWrongControlCmd(uid string) []byte {
	params := json.RawMessage([]byte("{}"))
	cmd := controlCommand{
		UID:    uid,
		Method: "wrong",
		Params: &params,
	}
	cmdBytes, _ := json.Marshal(cmd)
	return cmdBytes
}

func TestPublish(t *testing.T) {
	// Custom config
	c := newTestConfig()

	// Set custom options for default namespace
	c.ChannelOptions.HistoryLifetime = 10
	c.ChannelOptions.HistorySize = 2
	c.ChannelOptions.HistoryDropInactive = true

	app := testMemoryAppWithConfig(&c)
	createTestClients(app, 10, 1, nil)
	data, _ := json.Marshal(map[string]string{"test": "publish"})
	err := app.Publish(Channel("channel-0"), data, ConnID(""), nil)
	assert.Nil(t, err)

	// Check publish to subscribed channels did result in saved history
	hist, err := app.History(Channel("channel-0"))
	assert.Nil(t, err)
	assert.Equal(t, 1, len(hist))

	// Publishing to a channel no one is subscribed to should be a no-op
	err = app.Publish(Channel("some-other-channel"), data, ConnID(""), nil)
	assert.Nil(t, err)

	hist, err = app.History(Channel("some-other-channel"))
	assert.Nil(t, err)
	assert.Equal(t, 0, len(hist))

}

func TestPublishJoinLeave(t *testing.T) {
	app := testMemoryApp()
	createTestClients(app, 10, 1, nil)
	err := app.pubJoinLeave(Channel("channel-0"), "join", ClientInfo{})
	assert.Equal(t, nil, err)
}

func TestControlMessages(t *testing.T) {
	app := testApp()
	app.Run()
	// command from this node
	cmd := testPingControlCmd(app.uid)
	err := app.controlMsg(cmd)
	assert.Equal(t, nil, err)
	cmd = testPingControlCmd("another_node")
	err = app.controlMsg(cmd)
	assert.Equal(t, nil, err)
	err = app.controlMsg(testWrongControlCmd("another node"))
	assert.Equal(t, ErrInvalidMessage, err)
	err = app.controlMsg(testUnsubscribeControlCmd("another node"))
	assert.Equal(t, nil, err)
	err = app.controlMsg(testDisconnectControlCmd("another node"))
	assert.Equal(t, nil, err)
}

func TestUpdateMetrics(t *testing.T) {
	app := testMemoryApp()
	createTestClients(app, 10, 1, nil)
	data, _ := json.Marshal(map[string]string{"test": "publish"})
	err := app.Publish(Channel("channel-0"), data, ConnID(""), nil)
	assert.Equal(t, nil, err)
	app.config.NodeMetricsInterval = 1 * time.Millisecond
	app.updateMetricsOnce()
	assert.Equal(t, int64(1), app.metrics.metrics.NumMsgPublished)
}
