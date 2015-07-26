package libcentrifugo

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/centrifugal/centrifugo/libcentrifugo/auth"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
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
