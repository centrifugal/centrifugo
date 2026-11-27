package api

import (
	"bytes"
	"context"
	"fmt"
	"io"
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
	_ = resp.Body.Close()

	// empty body.
	req, _ = http.NewRequest("POST", server.URL+"/api", strings.NewReader(""))
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, resp.StatusCode, http.StatusBadRequest)
	_ = resp.Body.Close()

	// valid JSON request.
	data := `{"method":"publish","params":{"channel": "test", "data":{}}}`
	req, _ = http.NewRequest("POST", server.URL+"/api", bytes.NewBuffer([]byte(data)))
	req.Header.Add("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, resp.StatusCode, http.StatusOK)
	_ = resp.Body.Close()

	// valid JSON request to special method route
	data = `{"channel": "test", "data":{}}`
	req, _ = http.NewRequest("POST", server.URL+"/api/publish", bytes.NewBuffer([]byte(data)))
	req.Header.Add("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, resp.StatusCode, http.StatusOK)
	_ = resp.Body.Close()

	// request with unknown method.
	data = `{"method":"unknown","params":{"channel": "test", "data":{}}}`
	req, _ = http.NewRequest("POST", server.URL+"/api", bytes.NewBuffer([]byte(data)))
	req.Header.Add("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, resp.StatusCode, http.StatusBadRequest)
	_ = resp.Body.Close()
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

type countingReader struct {
	remaining int
	readBytes int
}

func (r *countingReader) Read(p []byte) (int, error) {
	if r.remaining <= 0 {
		return 0, io.EOF
	}
	n := len(p)
	if n > r.remaining {
		n = r.remaining
	}
	for i := 0; i < n; i++ {
		p[i] = 'a'
	}
	r.remaining -= n
	r.readBytes += n
	return n, nil
}

func TestAPIHandlerMaxRequestBodySize(t *testing.T) {
	n := nodeWithMemoryEngine()
	defer func() { _ = n.Shutdown(context.Background()) }()

	cfg := config.DefaultConfig()
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)
	apiExecutor := NewExecutor(n, cfgContainer, &testSurveyCaller{}, ExecutorConfig{Protocol: "test", UseOpenTelemetry: false})

	const maxLimit = 128

	mux := http.NewServeMux()
	apiHandler := NewHandler(n, apiExecutor, Config{MaxRequestBodySize: maxLimit})
	mux.Handle("/api", apiHandler.OldRoute())
	for path, handler := range apiHandler.Routes() {
		mux.Handle("/api"+path, handler)
	}

	server := httptest.NewServer(mux)
	defer server.Close()

	routes := []struct {
		path    string
		payload string
	}{
		{path: "/api/publish", payload: `{"channel":"test","data":{}}`},
		{path: "/api/batch", payload: `{"commands":[{"publish":{"channel":"test","data":{}}}]}`},
		{path: "/api/broadcast", payload: `{"channels":["test"],"data":{}}`},
		{path: "/api/history", payload: `{"channel":"test"}`},
		{path: "/api", payload: `{"method":"publish","params":{"channel":"test","data":{}}}`},
	}

	oversized := fmt.Sprintf(`{"method":"publish","params":{"channel":"test","data":"%s"}}`, strings.Repeat("x", maxLimit*4))

	for _, tc := range routes {
		t.Run("within limit "+tc.path, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, server.URL+tc.path, strings.NewReader(tc.payload))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()
			require.Equal(t, http.StatusOK, resp.StatusCode)
		})

		t.Run("over limit "+tc.path, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, server.URL+tc.path, strings.NewReader(oversized))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()
			require.Equal(t, http.StatusRequestEntityTooLarge, resp.StatusCode)
		})
	}

	t.Run("stops reading oversized streaming body early", func(t *testing.T) {
		cr := &countingReader{remaining: 1024 * 1024}
		req := httptest.NewRequest(http.MethodPost, "/api/publish", cr)
		rec := httptest.NewRecorder()

		apiHandler.handlePublish(rec, req)

		require.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
		require.LessOrEqual(t, cr.readBytes, maxLimit+1)
	})

	for _, maxSize := range []int{0, -1} {
		t.Run(fmt.Sprintf("non-positive limit %d uses default", maxSize), func(t *testing.T) {
			withDefault := NewHandler(n, apiExecutor, Config{MaxRequestBodySize: maxSize})

			okReq := httptest.NewRequest(http.MethodPost, "/api/publish", strings.NewReader(`{"channel":"test","data":{}}`))
			okRec := httptest.NewRecorder()
			withDefault.handlePublish(okRec, okReq)
			require.Equal(t, http.StatusOK, okRec.Code)

			cr := &countingReader{remaining: DefaultMaxRequestBodySize + 1024}
			overReq := httptest.NewRequest(http.MethodPost, "/api/publish", cr)
			overRec := httptest.NewRecorder()
			withDefault.handlePublish(overRec, overReq)
			require.Equal(t, http.StatusRequestEntityTooLarge, overRec.Code)
			require.LessOrEqual(t, cr.readBytes, DefaultMaxRequestBodySize+1)
		})
	}
}
