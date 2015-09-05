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

	"github.com/centrifugal/centrifugo/Godeps/_workspace/src/github.com/gorilla/websocket"
	"github.com/centrifugal/centrifugo/Godeps/_workspace/src/github.com/stretchr/testify/assert"
	"github.com/centrifugal/centrifugo/libcentrifugo/auth"
)

func TestDefaultMux(t *testing.T) {
	app := testApp()
	mux := DefaultMux(app, "", "path/to/web", "sockjs url")
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

func TestRawWsHandler(t *testing.T) {
	app := testApp()
	mux := DefaultMux(app, "", "path/to/web", "sockjs url")
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

func TestAdminWebsocketHandler(t *testing.T) {
	app := testApp()
	mux := DefaultMux(app, "", "path/to/web", "sockjs url")
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

func BenchmarkAPIHandler(b *testing.B) {
	app := testAppWithClients(1, 100)
	data := "{\"method\":\"publish\",\"params\":{\"channel\": \"test\", \"data\":{}}}"
	sign := auth.GenerateApiSign("secret", "test1", []byte(data))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/test1", bytes.NewBuffer([]byte(data)))
		req.Header.Add("X-API-Sign", sign)
		req.Header.Add("Content-Type", "application/json")
		app.APIHandler(rec, req)
	}
}

func TestAPIHandler(t *testing.T) {
	app := testApp()
	mux := DefaultMux(app, "", "path/to/web", "sockjs url")
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

	// project not found
	values := url.Values{}
	values.Set("sign", "wrong")
	values.Add("data", "data")
	rec = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", server.URL+"/api/wrong_project", strings.NewReader(values.Encode()))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Add("Content-Length", strconv.Itoa(len(values.Encode())))
	app.APIHandler(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)

	// wrong sign
	rec = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", server.URL+"/api/test1", strings.NewReader(values.Encode()))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Add("Content-Length", strconv.Itoa(len(values.Encode())))
	app.APIHandler(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)

	// valid form urlencoded request
	rec = httptest.NewRecorder()
	values = url.Values{}
	data := "{\"method\":\"publish\",\"params\":{\"channel\": \"test\", \"data\":{}}}"
	sign := auth.GenerateApiSign("secret", "test1", []byte(data))
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
	sign = auth.GenerateApiSign("secret", "test1", []byte(data))
	req, _ = http.NewRequest("POST", server.URL+"/api/test1", bytes.NewBuffer([]byte(data)))
	req.Header.Add("X-API-Sign", sign)
	req.Header.Add("Content-Type", "application/json")
	app.APIHandler(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	// request with unknown method
	rec = httptest.NewRecorder()
	values = url.Values{}
	data = "{\"method\":\"unknown\",\"params\":{\"channel\": \"test\", \"data\":{}}}"
	sign = auth.GenerateApiSign("secret", "test1", []byte(data))
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
	r := httptest.NewRecorder()

	// no web secret in config
	values := url.Values{}
	values.Set("password", "pass")
	req, _ := http.NewRequest("POST", "/auth/", strings.NewReader(values.Encode()))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	app.AuthHandler(r, req)
	assert.Equal(t, http.StatusBadRequest, r.Code)

	c := newTestConfig()
	c.WebPassword = "password"
	c.WebSecret = "secret"
	app.SetConfig(c)

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

func TestInfoHandler(t *testing.T) {
	app := testApp()
	r := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/info/", nil)
	app.InfoHandler(r, req)
	body, _ := ioutil.ReadAll(r.Body)
	assert.Equal(t, true, strings.Contains(string(body), "nodes"))
	assert.Equal(t, true, strings.Contains(string(body), "node_name"))
	assert.Equal(t, true, strings.Contains(string(body), "version"))
	assert.Equal(t, true, strings.Contains(string(body), "structure"))
	assert.Equal(t, true, strings.Contains(string(body), "engine"))
}

func TestActionHandler(t *testing.T) {
	app := testApp()

	r := httptest.NewRecorder()
	values := url.Values{}
	values.Set("project", "test")
	values.Set("method", "publish")
	req, _ := http.NewRequest("POST", "/action/", strings.NewReader(values.Encode()))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	app.ActionHandler(r, req)
	assert.Equal(t, http.StatusBadRequest, r.Code)

	r = httptest.NewRecorder()
	values = url.Values{}
	values.Set("project", "test1")
	values.Set("method", "wrong_method")
	req, _ = http.NewRequest("POST", "/action/", strings.NewReader(values.Encode()))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	app.ActionHandler(r, req)
	assert.Equal(t, http.StatusBadRequest, r.Code)

	r = httptest.NewRecorder()
	values = url.Values{}
	values.Set("project", "test1")
	values.Set("method", "publish")
	values.Set("channel", "test")
	values.Set("data", "")
	req, _ = http.NewRequest("POST", "/action/", strings.NewReader(values.Encode()))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	app.ActionHandler(r, req)
	assert.Equal(t, http.StatusBadRequest, r.Code)

	r = httptest.NewRecorder()
	values = url.Values{}
	values.Set("project", "test1")
	values.Set("method", "publish")
	values.Set("channel", "test")
	values.Set("data", "{}")
	req, _ = http.NewRequest("POST", "/action/", strings.NewReader(values.Encode()))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	app.ActionHandler(r, req)
	assert.Equal(t, http.StatusOK, r.Code)

	r = httptest.NewRecorder()
	values = url.Values{}
	values.Set("project", "test1")
	values.Set("method", "unsubscribe")
	values.Set("user", "test")
	values.Set("channel", "test")
	req, _ = http.NewRequest("POST", "/action/", strings.NewReader(values.Encode()))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	app.ActionHandler(r, req)
	assert.Equal(t, http.StatusOK, r.Code)

	r = httptest.NewRecorder()
	values = url.Values{}
	values.Set("project", "test1")
	values.Set("method", "disconnect")
	values.Set("user", "test")
	req, _ = http.NewRequest("POST", "/action/", strings.NewReader(values.Encode()))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	app.ActionHandler(r, req)
	assert.Equal(t, http.StatusOK, r.Code)

	r = httptest.NewRecorder()
	values = url.Values{}
	values.Set("project", "test1")
	values.Set("method", "presence")
	values.Set("channel", "test")
	req, _ = http.NewRequest("POST", "/action/", strings.NewReader(values.Encode()))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	app.ActionHandler(r, req)
	assert.Equal(t, http.StatusOK, r.Code)

	r = httptest.NewRecorder()
	values = url.Values{}
	values.Set("project", "test1")
	values.Set("method", "history")
	values.Set("channel", "test")
	req, _ = http.NewRequest("POST", "/action/", strings.NewReader(values.Encode()))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	app.ActionHandler(r, req)
	assert.Equal(t, http.StatusOK, r.Code)
}
