package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAPIHandler(t *testing.T) {
	n := nodeWithMemoryEngine()

	mux := http.NewServeMux()
	mux.Handle("/api", NewHandler(n, Config{}))
	server := httptest.NewServer(mux)
	defer server.Close()

	// nil body.
	req, _ := http.NewRequest("POST", server.URL+"/api", nil)
	resp, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, resp.StatusCode, http.StatusBadRequest)

	// empty body.
	req, _ = http.NewRequest("POST", server.URL+"/api", strings.NewReader(""))
	resp, err = http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, resp.StatusCode, http.StatusBadRequest)

	// valid JSON request.
	data := `{"method":"publish","params":{"channel": "test", "data":{}}}`
	req, _ = http.NewRequest("POST", server.URL+"/api", bytes.NewBuffer([]byte(data)))
	req.Header.Add("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, resp.StatusCode, http.StatusOK)

	// request with unknown method.
	data = `{"method":"unknown","params":{"channel": "test", "data":{}}}`
	req, _ = http.NewRequest("POST", server.URL+"/api", bytes.NewBuffer([]byte(data)))
	req.Header.Add("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, resp.StatusCode, http.StatusBadRequest)
}
