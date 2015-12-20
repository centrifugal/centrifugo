package libcentrifugo

import (
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/Godeps/_workspace/src/github.com/stretchr/testify/assert"
)

type messageCounter struct {
	sent  chan bool
	n     int64
	total int64
}

type testSession struct {
	counter *messageCounter
	n       int64
	closed  bool
}

func (t *testSession) Send(msg []byte) error {
	atomic.AddInt64(&t.n, 1)
	var val int64
	if t.counter != nil {
		val = atomic.AddInt64(&t.counter.n, 1)
	}
	if t.counter != nil && t.counter.sent != nil && val%t.counter.total == 0 {
		t.counter.sent <- true
	}
	return nil
}

func (t *testSession) Close(status uint32, reason string) error {
	t.closed = true
	return nil
}

func (app *Application) newTestHandler(b *testing.B, s *testSession) *client {
	c, err := newClient(app, s)
	if err != nil {
		b.Fatal(err)
	}
	return c
}

func testApp() *Application {
	c := newTestConfig()
	app, _ := NewApplication(&c)
	app.SetEngine(newTestEngine())
	return app
}

func testMemoryApp() *Application {
	c := newTestConfig()
	app, _ := NewApplication(&c)
	app.SetEngine(NewMemoryEngine(app))
	return app
}

func testRedisApp() *Application {
	c := newTestConfig()
	app, _ := NewApplication(&c)
	app.SetEngine(testRedisEngine(app))
	return app
}

func newTestClient(app *Application) *client {
	s := &testSession{}
	c, _ := newClient(app, s)
	return c
}

func newSynchronizedTestClient(app *Application, counter *messageCounter) *client {
	s := &testSession{counter: counter}
	c, _ := newClient(app, s)
	return c
}

func createTestClients(app *Application, nChannels, nChannelClients int) {
	app.config.Insecure = true
	for i := 0; i < nChannelClients; i++ {
		c := newTestClient(app)
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

func createTestClientsSynchronized(app *Application, nChannels, nChannelClients int, nMessages int, sent chan bool) {
	app.config.Insecure = true
	counter := &messageCounter{total: int64(nMessages), sent: sent}
	for i := 0; i < nChannelClients; i++ {
		c := newSynchronizedTestClient(app, counter)
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
	createTestClients(app, nChannels, nChannelClients)
	return app
}

func testMemoryAppWithClientsSynchronized(nChannels int, nChannelClients int, nMessages int, sent chan bool) *Application {
	app := testMemoryApp()
	createTestClientsSynchronized(app, nChannels, nChannelClients, nMessages, sent)
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
	app := testMemoryApp()
	createTestClients(app, 10, 1)
	data, _ := json.Marshal(map[string]string{"test": "publish"})
	err := app.Publish(Channel("channel-0"), data, ConnID(""), nil)
	assert.Equal(t, nil, err)
}

func TestPublishJoinLeave(t *testing.T) {
	app := testMemoryApp()
	createTestClients(app, 10, 1)
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
	createTestClients(app, 10, 1)
	data, _ := json.Marshal(map[string]string{"test": "publish"})
	err := app.Publish(Channel("channel-0"), data, ConnID(""), nil)
	assert.Equal(t, nil, err)
	app.config.NodeMetricsInterval = 1 * time.Millisecond
	app.updateMetricsOnce()
	assert.Equal(t, int64(1), app.metrics.metrics.NumMsgPublished)
}

func createUsers(users, chanUser, totChannels int) []*testClientConn {
	uC := make([]*testClientConn, users)
	for i := range uC {
		c := newTestUserCC()
		c.UID = UserID(fmt.Sprintf("uid-%d", i))
		c.CID = ConnID(fmt.Sprintf("cid-%d", i))
		c.Channels = make([]Channel, chanUser)
		for j := 0; j < chanUser; j++ {
			c.Channels[j] = Channel(fmt.Sprintf("chan-%d", (j+i*chanUser)%totChannels))
		}
		uC[i] = c
	}
	return uC
}

func BenchmarkSendReceive(b *testing.B) {
	totChannels := 200
	conf := newTestConfig()
	app, _ := NewApplication(&conf)
	app.SetEngine(NewMemoryEngine(app))
	app.config.Insecure = true
	conns := createUsers(50, 10, totChannels)
	for _, c := range conns {
		c.sess = &testSession{}
		cli := app.newTestHandler(b, c.sess)
		cmd := ConnectClientCommand{
			User: c.UID,
		}
		cli.connectCmd(&cmd)
		for _, ch := range c.Channels {
			cmd := SubscribeClientCommand{
				Channel: ch,
			}
			resp, err := cli.subscribeCmd(&cmd)
			if err != nil {
				b.Fatal(err)
			}
			if resp.err != nil {
				b.Fatal(resp.err)
			}
		}
	}
	b.ResetTimer()
	tn := time.Now()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			ch := app.channelID(Channel(fmt.Sprintf("chan-%d", i%totChannels)))
			err := app.clientMsg(ch, []byte("message"))
			if err != nil {
				b.Fatal(err)
			}
			i++
		}
	})
	// TODO: Flush
	dur := time.Since(tn)
	b.StopTimer()
	time.Sleep(time.Second)
	total := 0
	for _, user := range conns {
		total += int(atomic.AddInt64(&user.sess.n, 0))
	}
	b.Logf("Chans:%d, Clnts:%d Msgs:%d Rcvd:%d", app.nChannels(), app.nClients(), b.N, total)
	if dur > time.Millisecond*10 {
		b.Logf("%d messages/sec", total*int(time.Second)/int(dur))
	}

}
