package libcentrifugo

import (
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/Godeps/_workspace/src/github.com/nu7hatch/gouuid"
	"github.com/centrifugal/centrifugo/Godeps/_workspace/src/github.com/stretchr/testify/assert"
	"github.com/centrifugal/centrifugo/libcentrifugo/stringqueue"
)

type testSession struct {
	n      int64
	closed bool
}

func (t *testSession) Send(msg string) error {
	atomic.AddInt64(&t.n, 1)
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

func newTestConfig() Config {
	return *DefaultConfig
}

func testApp() *Application {
	c := newTestConfig()
	app, _ := NewApplication(&c)
	app.SetEngine(newTestEngine())
	app.SetStructure(getTestStructure())
	return app
}

func newTestClient(app *Application) *client {
	uid, _ := uuid.NewV4()
	s := &testSession{}
	c := client{
		UID:       ConnID(uid.String()),
		app:       app,
		sess:      s,
		messages:  stringqueue.New(),
		closeChan: make(chan struct{}),
	}
	return &c
}

func createTestClients(app *Application, nChannels, nChannelClients int) {
	app.config.Insecure = true
	for i := 0; i < nChannelClients; i++ {
		c := newTestClient(app)
		cmd := connectClientCommand{
			Project: ProjectKey("test1"),
			User:    UserID(fmt.Sprintf("user-%d", i)),
		}
		resp, err := c.connectCmd(&cmd)
		if err != nil {
			panic(err)
		}
		if resp.err != nil {
			panic(resp.err)
		}
		for j := 0; j < nChannels; j++ {
			cmd := subscribeClientCommand{
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

func testAppWithClients(nChannels, nChannelClients int) *Application {
	app := testApp()
	createTestClients(app, nChannels, nChannelClients)
	return app
}

func TestProjectByKey(t *testing.T) {
	app := testApp()
	p, found := app.projectByKey("nonexistent")
	assert.Equal(t, found, false)
	p, found = app.projectByKey("test1")
	assert.Equal(t, found, true)
	assert.Equal(t, p.Name, ProjectKey("test1"))
}

func TestChannelID(t *testing.T) {
	app := testApp()
	p, _ := app.projectByKey("test1")
	chID := app.channelID(p.Name, "channel")
	assert.Equal(t, chID, ChannelID(defaultChannelPrefix+".test1.channel"))
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

func createUsers(users, chanUser, totChannels int) []*testClientConn {
	uC := make([]*testClientConn, users)
	for i := range uC {
		c := newTestUserCC()
		c.Uid = UserID(fmt.Sprintf("uid-%d", i))
		c.Cid = ConnID(fmt.Sprintf("cid-%d", i))
		c.PK = ProjectKey("test1")
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
	app.SetStructure(getTestStructure())
	app.config.Insecure = true
	pk := ProjectKey("test1")
	conns := createUsers(50, 10, totChannels)
	for _, c := range conns {
		c.sess = &testSession{}
		cli := app.newTestHandler(b, c.sess)
		cmd := connectClientCommand{
			Project: c.PK,
			User:    c.Uid,
		}
		cli.connectCmd(&cmd)
		for _, ch := range c.Channels {
			cmd := subscribeClientCommand{
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
			ch := app.channelID(pk, Channel(fmt.Sprintf("chan-%d", i%totChannels)))
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
