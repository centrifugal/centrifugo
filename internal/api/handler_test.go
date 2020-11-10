package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/centrifugal/centrifugo/internal/rule"

	"github.com/stretchr/testify/require"
)

func TestAPIHandler(t *testing.T) {
	n := nodeWithMemoryEngine()

	ruleConfig := rule.DefaultConfig
	ruleContainer := rule.NewContainer(ruleConfig)
	apiExecutor := NewExecutor(n, ruleContainer, "test")

	mux := http.NewServeMux()
	mux.Handle("/api", NewHandler(n, apiExecutor, Config{}))
	server := httptest.NewServer(mux)
	defer server.Close()

	// nil body.
	req, _ := http.NewRequest("POST", server.URL+"/api", nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, resp.StatusCode, http.StatusBadRequest)

	// empty body.
	req, _ = http.NewRequest("POST", server.URL+"/api", strings.NewReader(""))
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, resp.StatusCode, http.StatusBadRequest)

	// valid JSON request.
	data := `{"method":"publish","params":{"channel": "test", "data":{}}}`
	req, _ = http.NewRequest("POST", server.URL+"/api", bytes.NewBuffer([]byte(data)))
	req.Header.Add("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, resp.StatusCode, http.StatusOK)

	// request with unknown method.
	data = `{"method":"unknown","params":{"channel": "test", "data":{}}}`
	req, _ = http.NewRequest("POST", server.URL+"/api", bytes.NewBuffer([]byte(data)))
	req.Header.Add("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, resp.StatusCode, http.StatusBadRequest)
}
