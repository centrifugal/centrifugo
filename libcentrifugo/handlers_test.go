package libcentrifugo

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/libcentrifugo/auth"
	"github.com/elazarl/go-bindata-assetfs"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
)

func TestDefaultMux(t *testing.T) {
	app := testApp()
	mux := DefaultMux(app, DefaultMuxOptions)
	server := httptest.NewServer(mux)
	defer server.Close()
	resp, err := http.Get(server.URL + "/connection/info")
	assert.Equal(t, nil, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	app.Shutdown()
	resp, err = http.Get(server.URL + "/connection/info")
	assert.Equal(t, nil, err)
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
}

func TestMuxWithDebugFlag(t *testing.T) {
	app := testApp()
	opts := DefaultMuxOptions
	opts.HandlerFlags |= HandlerDebug
	mux := DefaultMux(app, opts)
	server := httptest.NewServer(mux)
	defer server.Close()
	resp, err := http.Get(server.URL + "/debug/pprof")
	assert.Equal(t, nil, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

type testFileSystem struct{}

func (fs *testFileSystem) Open(name string) (http.File, error) {
	file := assetfs.NewAssetFile(name, []byte("it works"), time.Now())
	return file, nil
}

func TestMuxAdminWeb404(t *testing.T) {
	app := testApp()
	opts := DefaultMuxOptions
	mux := DefaultMux(app, opts)
	server := httptest.NewServer(mux)
	defer server.Close()
	resp, err := http.Get(server.URL + "/")
	assert.Equal(t, nil, err)
	assert.Equal(t, resp.StatusCode, http.StatusNotFound)
}

func TestMuxAdminWeb(t *testing.T) {
	app := testApp()
	opts := DefaultMuxOptions
	opts.Web = true
	opts.Admin = true
	opts.WebFS = &testFileSystem{}
	opts.HandlerFlags |= HandlerAdmin
	mux := DefaultMux(app, opts)
	server := httptest.NewServer(mux)
	defer server.Close()
	resp, err := http.Get(server.URL + "/path/")
	assert.Equal(t, nil, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	assert.Equal(t, nil, err)
	assert.Equal(t, "it works", string(body))
}

func TestHandlerFlagString(t *testing.T) {
	var flag HandlerFlag
	flag |= HandlerAPI
	s := flag.String()
	assert.True(t, strings.Contains(s, handlerText[HandlerAPI]))
	assert.False(t, strings.Contains(s, handlerText[HandlerRawWS]))
}

func TestRawWsHandler(t *testing.T) {
	app := testApp()
	mux := DefaultMux(app, DefaultMuxOptions)
	server := httptest.NewServer(mux)
	defer server.Close()
	url := "ws" + server.URL[4:]
	conn, resp, err := websocket.DefaultDialer.Dial(url+"/connection/websocket", nil)
	conn.Close()
	assert.Equal(t, nil, err)
	assert.NotEqual(t, nil, conn)
	assert.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)

	app.Shutdown()
	_, resp, err = websocket.DefaultDialer.Dial(url+"/connection/websocket", nil)
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
}

func TestAdminWebsocketHandlerNotFound(t *testing.T) {
	app := testApp()
	opts := DefaultMuxOptions
	opts.Web = true
	mux := DefaultMux(app, opts)
	server := httptest.NewServer(mux)
	defer server.Close()
	url := "ws" + server.URL[4:]
	_, resp, _ := websocket.DefaultDialer.Dial(url+"/socket", nil)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestAdminWebsocketHandler(t *testing.T) {
	app := testApp()
	app.config.Admin = true // admin websocket available only if option enabled.
	opts := DefaultMuxOptions
	opts.Admin = true
	mux := DefaultMux(app, opts)
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
	app := testApp()
	opts := DefaultMuxOptions
	mux := DefaultMux(app, opts)
	server := httptest.NewServer(mux)
	defer server.Close()
	url := "ws" + server.URL[4:]
	conn, resp, err := websocket.DefaultDialer.Dial(url+"/connection/220/fi0pbfvm/websocket", nil)
	assert.Equal(t, nil, err)
	assert.NotEqual(t, nil, conn)
	_, p, err := conn.ReadMessage()
	// open frame of SockJS protocol
	assert.Equal(t, "o", string(p))
	conn.Close()
	assert.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)
}

func TestRawWSHandler(t *testing.T) {
	app := testApp()
	opts := DefaultMuxOptions
	mux := DefaultMux(app, opts)
	server := httptest.NewServer(mux)
	defer server.Close()
	url := "ws" + server.URL[4:]
	conn, resp, err := websocket.DefaultDialer.Dial(url+"/connection/websocket", nil)
	assert.Equal(t, nil, err)
	data := map[string]interface{}{
		"method": "ping",
		"params": pingBody{Data: "hello"},
	}
	conn.WriteJSON(data)
	var response clientResponse
	conn.ReadJSON(&response)
	assert.NotEqual(t, nil, response)
	// connect message should be sent first, so we get disconnect
	assert.Equal(t, "disconnect", response.Method)
	conn.Close()
	assert.NotEqual(t, nil, conn)
	assert.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)
}

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

func TestAPIHandler(t *testing.T) {
	app := testApp()
	mux := DefaultMux(app, DefaultMuxOptions)
	server := httptest.NewServer(mux)
	defer server.Close()

	// nil body
	rec := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", server.URL+"/api/test", nil)
	app.APIHandler(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	// empty body
	rec = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", server.URL+"/api/test", strings.NewReader(""))
	app.APIHandler(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	// wrong sign
	values := url.Values{}
	values.Set("sign", "wrong")
	values.Add("data", "data")
	rec = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", server.URL+"/api/", strings.NewReader(values.Encode()))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Add("Content-Length", strconv.Itoa(len(values.Encode())))
	app.APIHandler(rec, req)
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
	app.APIHandler(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	// valid JSON request
	rec = httptest.NewRecorder()
	data = "{\"method\":\"publish\",\"params\":{\"channel\": \"test\", \"data\":{}}}"
	sign = auth.GenerateApiSign("secret", []byte(data))
	req, _ = http.NewRequest("POST", server.URL+"/api/test1", bytes.NewBuffer([]byte(data)))
	req.Header.Add("X-API-Sign", sign)
	req.Header.Add("Content-Type", "application/json")
	app.APIHandler(rec, req)
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
	app.APIHandler(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestAuthHandler(t *testing.T) {
	app := testApp()

	// no web secret in config
	r := httptest.NewRecorder()
	values := url.Values{}
	values.Set("password", "pass")
	req, _ := http.NewRequest("POST", "/auth/", strings.NewReader(values.Encode()))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	app.AuthHandler(r, req)
	assert.Equal(t, http.StatusBadRequest, r.Code)

	app.Lock()
	app.config.AdminPassword = "password"
	app.config.AdminSecret = "secret"
	app.Unlock()

	// wrong password
	r = httptest.NewRecorder()
	values.Set("password", "wrong_pass")
	req, _ = http.NewRequest("POST", "/auth/", strings.NewReader(values.Encode()))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	app.AuthHandler(r, req)
	assert.Equal(t, http.StatusBadRequest, r.Code)
	body, _ := ioutil.ReadAll(r.Body)
	assert.Equal(t, false, strings.Contains(string(body), "token"))

	// correct password
	r = httptest.NewRecorder()
	values.Set("password", "password")
	req, _ = http.NewRequest("POST", "/auth/", strings.NewReader(values.Encode()))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	app.AuthHandler(r, req)
	assert.Equal(t, http.StatusOK, r.Code)
	body, _ = ioutil.ReadAll(r.Body)
	assert.Equal(t, true, strings.Contains(string(body), "token"))
}
