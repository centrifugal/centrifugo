package api

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/centrifugal/centrifugo/v6/internal/config"

	"github.com/stretchr/testify/require"
)

func TestAPIHandler(t *testing.T) {
	n := nodeWithMemoryEngine()
	defer func() { _ = n.Shutdown(context.Background()) }()

	cfg := config.DefaultConfig()
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)
	apiExecutor := NewExecutor(n, cfgContainer, &testSurveyCaller{}, ExecutorConfig{Protocol: "test", UseOpenTelemetry: false})

	mux := http.NewServeMux()
	apiHandler := NewHandler(n, apiExecutor, Config{})
	mux.Handle("/api", apiHandler.OldRoute())
	for path, handler := range apiHandler.Routes() {
		handlePath := "/api" + path
		mux.Handle(handlePath, handler)
	}

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

	// valid JSON request to special method route
	data = `{"channel": "test", "data":{}}`
	req, _ = http.NewRequest("POST", server.URL+"/api/publish", bytes.NewBuffer([]byte(data)))
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

func BenchmarkAPIHandler(b *testing.B) {
	n := nodeWithMemoryEngine()
	defer func() { _ = n.Shutdown(context.Background()) }()

	cfg := config.DefaultConfig()
	cfg.Client.Insecure = true
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(b, err)

	handler := NewHandler(n, NewExecutor(n, cfgContainer, nil, ExecutorConfig{
		Protocol:         "http",
		UseOpenTelemetry: false,
	}), Config{})

	payload := []byte(`{"method": "publish", "params": {"channel": "index", "data": 1}}`)

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			request, _ := http.NewRequest(http.MethodPost, "/api", bytes.NewReader(payload))
			recorder := httptest.NewRecorder()
			handler.handlePublish(recorder, request)
			if recorder.Code != http.StatusOK {
				b.Fatalf("unexpected status code %d", recorder.Code)
			}
		}
	})
}
