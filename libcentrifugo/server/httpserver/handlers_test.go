package httpserver

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/centrifugal/centrifugo/libcentrifugo/auth"
	"github.com/centrifugal/centrifugo/libcentrifugo/node"
	"github.com/centrifugal/centrifugo/libcentrifugo/proto"
	"github.com/centrifugal/centrifugo/libcentrifugo/server"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
)

type TestEngine struct{}

func NewTestEngine() *TestEngine {
	return &TestEngine{}
}

func (e *TestEngine) Name() string {
	return "test engine"
}

func (e *TestEngine) Run() error {
	return nil
}

func (e *TestEngine) Shutdown() error {
	return nil
}

func (e *TestEngine) PublishMessage(message *proto.Message, opts *proto.ChannelOptions) <-chan error {
	eChan := make(chan error, 1)
	eChan <- nil
	return eChan
}

func (e *TestEngine) PublishJoin(message *proto.JoinMessage, opts *proto.ChannelOptions) <-chan error {
	eChan := make(chan error, 1)
	eChan <- nil
	return eChan
}

func (e *TestEngine) PublishLeave(message *proto.LeaveMessage, opts *proto.ChannelOptions) <-chan error {
	eChan := make(chan error, 1)
	eChan <- nil
	return eChan
}

func (e *TestEngine) PublishAdmin(message *proto.AdminMessage) <-chan error {
	eChan := make(chan error, 1)
	eChan <- nil
	return eChan
}

func (e *TestEngine) PublishControl(message *proto.ControlMessage) <-chan error {
	eChan := make(chan error, 1)
	eChan <- nil
	return eChan
}

func (e *TestEngine) Subscribe(ch string) error {
	return nil
}

func (e *TestEngine) Unsubscribe(ch string) error {
	return nil
}

func (e *TestEngine) AddPresence(ch string, uid string, info proto.ClientInfo, expire int) error {
	return nil
}

func (e *TestEngine) RemovePresence(ch string, uid string) error {
	return nil
}

func (e *TestEngine) Presence(ch string) (map[string]proto.ClientInfo, error) {
	return map[string]proto.ClientInfo{}, nil
}

func (e *TestEngine) History(ch string, limit int) ([]proto.Message, error) {
	return []proto.Message{}, nil
}

func (e *TestEngine) Channels() ([]string, error) {
	return []string{}, nil
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

func NewTestHTTPServer() *HTTPServer {

	c := NewTestConfig()
	n := node.New("", c)

	s := &HTTPServer{
		node:       n,
		config:     &Config{},
		shutdownCh: make(chan struct{}),
	}

	err := n.Run(&node.RunOptions{Engine: NewTestEngine(), Servers: map[string]server.Server{"http": s}})
	if err != nil {
		panic(err)
	}
	return s
}

func TestDefaultMux(t *testing.T) {
	s := NewTestHTTPServer()
	mux := DefaultMux(s, DefaultMuxOptions)
	server := httptest.NewServer(mux)
	defer server.Close()
	resp, err := http.Get(server.URL + "/connection/info")
	assert.Equal(t, nil, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	s.node.Shutdown()
	resp, err = http.Get(server.URL + "/connection/info")
	assert.Equal(t, nil, err)
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
}

func TestMuxWithDebugFlag(t *testing.T) {
	s := NewTestHTTPServer()
	opts := DefaultMuxOptions
	opts.HandlerFlags |= HandlerDebug
	mux := DefaultMux(s, opts)
	server := httptest.NewServer(mux)
	defer server.Close()
	resp, err := http.Get(server.URL + "/debug/pprof")
	assert.Equal(t, nil, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestMuxAdminWeb404(t *testing.T) {
	s := NewTestHTTPServer()
	opts := DefaultMuxOptions
	mux := DefaultMux(s, opts)
	server := httptest.NewServer(mux)
	defer server.Close()
	resp, err := http.Get(server.URL + "/")
	assert.Equal(t, nil, err)
	assert.Equal(t, resp.StatusCode, http.StatusNotFound)
}

func TestHandlerFlagString(t *testing.T) {
	var flag HandlerFlag
	flag |= HandlerAPI
	s := flag.String()
	assert.True(t, strings.Contains(s, handlerText[HandlerAPI]))
	assert.False(t, strings.Contains(s, handlerText[HandlerRawWS]))
}

func TestRawWsHandler(t *testing.T) {
	s := NewTestHTTPServer()
	mux := DefaultMux(s, DefaultMuxOptions)
	server := httptest.NewServer(mux)
	defer server.Close()
	url := "ws" + server.URL[4:]
	conn, resp, err := websocket.DefaultDialer.Dial(url+"/connection/websocket", nil)
	conn.Close()
	assert.Equal(t, nil, err)
	assert.NotEqual(t, nil, conn)
	assert.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)

	s.node.Shutdown()
	_, resp, _ = websocket.DefaultDialer.Dial(url+"/connection/websocket", nil)
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
}

func TestAdminWebsocketHandlerNotFound(t *testing.T) {
	s := NewTestHTTPServer()
	opts := DefaultMuxOptions
	opts.Web = true
	mux := DefaultMux(s, opts)
	server := httptest.NewServer(mux)
	defer server.Close()
	url := "ws" + server.URL[4:]
	_, resp, _ := websocket.DefaultDialer.Dial(url+"/socket", nil)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestAdminWebsocketHandler(t *testing.T) {
	s := NewTestHTTPServer()
	conf := s.node.Config()
	conf.Admin = true // admin websocket available only if option enabled.
	s.node.SetConfig(&conf)
	opts := DefaultMuxOptions
	opts.Admin = true
	mux := DefaultMux(s, opts)
	server := httptest.NewServer(mux)
	defer server.Close()
	url := "ws" + server.URL[4:]
	conn, resp, err := websocket.DefaultDialer.Dial(url+"/socket", nil)
	data := map[string]interface{}{
		"method": "ping",
		"params": map[string]string{},
	}
	conn.WriteJSON(data)
	var response interface{}
	conn.ReadJSON(&response)
	conn.Close()
	assert.Equal(t, nil, err)
	assert.NotEqual(t, nil, conn)
	assert.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)

}

func TestSockJSHandler(t *testing.T) {
	s := NewTestHTTPServer()
	opts := DefaultMuxOptions
	mux := DefaultMux(s, opts)
	server := httptest.NewServer(mux)
	defer server.Close()
	url := "ws" + server.URL[4:]
	conn, resp, err := websocket.DefaultDialer.Dial(url+"/connection/220/fi0pbfvm/websocket", nil)
	assert.Equal(t, nil, err)
	assert.NotEqual(t, nil, conn)
	_, p, err := conn.ReadMessage()
	assert.Equal(t, nil, err)
	// open frame of SockJS protocol
	assert.Equal(t, "o", string(p))
	conn.Close()
	assert.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)
}

func TestRawWSHandler(t *testing.T) {
	opts := DefaultMuxOptions
	s := NewTestHTTPServer()
	mux := DefaultMux(s, opts)
	server := httptest.NewServer(mux)
	defer server.Close()
	url := "ws" + server.URL[4:]
	conn, resp, err := websocket.DefaultDialer.Dial(url+"/connection/websocket", nil)
	assert.Equal(t, nil, err)
	assert.NotEqual(t, nil, conn)
	assert.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)
}

/*
func BenchmarkAPIHandler(b *testing.B) {
	nChannels := 1
	nClients := 1000
	nCommands := 1000
	nMessages := nClients * nCommands
	sink := make(chan []byte, nMessages)
	app := testMemoryApp()

	// Use very large initial capacity so that queue resizes do not affect benchmark.
	app.config.ClientQueueInitialCapacity = 1000

	createTestClients(app, nChannels, nClients, sink)
	b.Logf("num channels: %v, num clients: %v, num unique clients %v, num commands: %v", app.clients.nChannels(), app.clients.nClients(), app.clients.nUniqueClients(), nCommands)
	jsonData := getNPublishJSON("channel-0", nCommands)
	sign := auth.GenerateApiSign("secret", jsonData)
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
		rec := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/test1", bytes.NewBuffer(jsonData))
		req.Header.Add("X-API-Sign", sign)
		req.Header.Add("Content-Type", "application/json")
		app.APIHandler(rec, req)
		<-done
	}
	b.StopTimer()
}
*/

func TestAPIHandler(t *testing.T) {
	s := NewTestHTTPServer()
	mux := DefaultMux(s, DefaultMuxOptions)
	server := httptest.NewServer(mux)
	defer server.Close()

	// nil body
	rec := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", server.URL+"/api/test", nil)
	s.APIHandler(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	// empty body
	rec = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", server.URL+"/api/test", strings.NewReader(""))
	s.APIHandler(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	// wrong sign
	values := url.Values{}
	values.Set("sign", "wrong")
	values.Add("data", "data")
	rec = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", server.URL+"/api/", strings.NewReader(values.Encode()))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Add("Content-Length", strconv.Itoa(len(values.Encode())))
	s.APIHandler(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)

	// valid form urlencoded request
	rec = httptest.NewRecorder()
	values = url.Values{}
	data := "{\"method\":\"publish\",\"params\":{\"channel\": \"test\", \"data\":{}}}"
	sign := auth.GenerateApiSign("secret", []byte(data))
	values.Set("sign", sign)
	values.Add("data", data)
	req, _ = http.NewRequest("POST", server.URL+"/api/test1", strings.NewReader(values.Encode()))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Add("Content-Length", strconv.Itoa(len(values.Encode())))
	s.APIHandler(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	// valid JSON request
	rec = httptest.NewRecorder()
	data = "{\"method\":\"publish\",\"params\":{\"channel\": \"test\", \"data\":{}}}"
	sign = auth.GenerateApiSign("secret", []byte(data))
	req, _ = http.NewRequest("POST", server.URL+"/api/test1", bytes.NewBuffer([]byte(data)))
	req.Header.Add("X-API-Sign", sign)
	req.Header.Add("Content-Type", "application/json")
	s.APIHandler(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	// request with unknown method
	rec = httptest.NewRecorder()
	values = url.Values{}
	data = "{\"method\":\"unknown\",\"params\":{\"channel\": \"test\", \"data\":{}}}"
	sign = auth.GenerateApiSign("secret", []byte(data))
	values.Set("sign", sign)
	values.Add("data", data)
	req, _ = http.NewRequest("POST", server.URL+"/api/test1", strings.NewReader(values.Encode()))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Add("Content-Length", strconv.Itoa(len(values.Encode())))
	s.APIHandler(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestAuthHandler(t *testing.T) {
	s := NewTestHTTPServer()

	// no web secret in config
	r := httptest.NewRecorder()
	values := url.Values{}
	values.Set("password", "pass")
	req, _ := http.NewRequest("POST", "/auth/", strings.NewReader(values.Encode()))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	s.AuthHandler(r, req)
	assert.Equal(t, http.StatusBadRequest, r.Code)

	conf := s.node.Config()
	conf.AdminPassword = "password"
	conf.AdminSecret = "secret"
	s.node.SetConfig(&conf)

	// wrong password
	r = httptest.NewRecorder()
	values.Set("password", "wrong_pass")
	req, _ = http.NewRequest("POST", "/auth/", strings.NewReader(values.Encode()))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	s.AuthHandler(r, req)
	assert.Equal(t, http.StatusBadRequest, r.Code)
	body, _ := ioutil.ReadAll(r.Body)
	assert.Equal(t, false, strings.Contains(string(body), "token"))

	// correct password
	r = httptest.NewRecorder()
	values.Set("password", "password")
	req, _ = http.NewRequest("POST", "/auth/", strings.NewReader(values.Encode()))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	s.AuthHandler(r, req)
	assert.Equal(t, http.StatusOK, r.Code)
	body, _ = ioutil.ReadAll(r.Body)
	assert.Equal(t, true, strings.Contains(string(body), "token"))
}
