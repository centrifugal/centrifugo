package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/proxyproto"

	"github.com/centrifugal/centrifuge"
	"github.com/stretchr/testify/require"
)

func newTestSharedPollHTTPProxy(t *testing.T, handler http.HandlerFunc) (*SharedPollRefreshHandler, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	proxy, err := NewHTTPSharedPollRefreshProxy(Config{
		Endpoint: server.URL,
		Timeout:  5000000000, // 5s in Duration.
	})
	require.NoError(t, err)
	h := NewSharedPollRefreshHandler(SharedPollRefreshHandlerConfig{
		Proxy: proxy,
	})
	return h, server
}

func TestRefreshProxy_HTTP_BasicRequest(t *testing.T) {
	var receivedBody []byte
	handler, server := newTestSharedPollHTTPProxy(t, func(w http.ResponseWriter, r *http.Request) {
		var err error
		receivedBody, err = io.ReadAll(r.Body)
		require.NoError(t, err)
		_, _ = w.Write([]byte(`{"result": {"items": []}}`))
	})
	defer server.Close()

	node, _ := centrifuge.New(centrifuge.Config{})
	_ = node.Run()
	defer func() { _ = node.Shutdown(context.Background()) }()

	fn := handler.Handle(node)
	_, err := fn(context.Background(), centrifuge.SharedPollEvent{
		Channel: "test:channel",
		Items: []centrifuge.SharedPollItem{
			{Key: "key1"},
			{Key: "key2"},
		},
	})
	require.NoError(t, err)

	var req proxyproto.SharedPollRefreshRequest
	err = json.Unmarshal(receivedBody, &req)
	require.NoError(t, err)
	require.Equal(t, "test:channel", req.Channel)
	require.Len(t, req.Items, 2)
	require.Equal(t, "key1", req.Items[0].Key)
	require.Equal(t, "key2", req.Items[1].Key)
}

func TestRefreshProxy_HTTP_ResponseParsing(t *testing.T) {
	handler, server := newTestSharedPollHTTPProxy(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"result": {"items": [
			{"key": "k1", "data": {"votes": 42}, "version": 5},
			{"key": "k2", "data": {"votes": 10}, "version": 3}
		]}}`))
	})
	defer server.Close()

	node, _ := centrifuge.New(centrifuge.Config{})
	_ = node.Run()
	defer func() { _ = node.Shutdown(context.Background()) }()

	fn := handler.Handle(node)
	result, err := fn(context.Background(), centrifuge.SharedPollEvent{
		Channel: "ch",
		Items:   []centrifuge.SharedPollItem{{Key: "k1"}, {Key: "k2"}},
	})
	require.NoError(t, err)
	require.Len(t, result.Items, 2)
	require.Equal(t, "k1", result.Items[0].Key)
	require.Equal(t, uint64(5), result.Items[0].Version)
	require.Contains(t, string(result.Items[0].Data), "42")
	require.Equal(t, "k2", result.Items[1].Key)
	require.Equal(t, uint64(3), result.Items[1].Version)
}

func TestRefreshProxy_HTTP_RemovedItem(t *testing.T) {
	handler, server := newTestSharedPollHTTPProxy(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"result": {"items": [{"key": "k1", "removed": true}]}}`))
	})
	defer server.Close()

	node, _ := centrifuge.New(centrifuge.Config{})
	_ = node.Run()
	defer func() { _ = node.Shutdown(context.Background()) }()

	fn := handler.Handle(node)
	result, err := fn(context.Background(), centrifuge.SharedPollEvent{
		Channel: "ch",
		Items:   []centrifuge.SharedPollItem{{Key: "k1"}},
	})
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	require.True(t, result.Items[0].Removed)
	require.Equal(t, "k1", result.Items[0].Key)
}

func TestRefreshProxy_HTTP_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
	}))
	t.Cleanup(server.Close)

	proxy, err := NewHTTPSharedPollRefreshProxy(Config{
		Endpoint: server.URL,
		Timeout:  100000000, // 100ms.
	})
	require.NoError(t, err)
	h := NewSharedPollRefreshHandler(SharedPollRefreshHandlerConfig{Proxy: proxy})

	node, _ := centrifuge.New(centrifuge.Config{})
	_ = node.Run()
	defer func() { _ = node.Shutdown(context.Background()) }()

	fn := h.Handle(node)
	_, err = fn(context.Background(), centrifuge.SharedPollEvent{
		Channel: "ch",
		Items:   []centrifuge.SharedPollItem{{Key: "k1"}},
	})
	require.Error(t, err)
}

func TestRefreshProxy_HTTP_ErrorResponse(t *testing.T) {
	handler, server := newTestSharedPollHTTPProxy(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`Internal Server Error`))
	})
	defer server.Close()

	node, _ := centrifuge.New(centrifuge.Config{})
	_ = node.Run()
	defer func() { _ = node.Shutdown(context.Background()) }()

	fn := handler.Handle(node)
	_, err := fn(context.Background(), centrifuge.SharedPollEvent{
		Channel: "ch",
		Items:   []centrifuge.SharedPollItem{{Key: "k1"}},
	})
	require.Error(t, err)
}

func TestRefreshProxy_HTTP_EmptyResponse(t *testing.T) {
	handler, server := newTestSharedPollHTTPProxy(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"result": {"items": []}}`))
	})
	defer server.Close()

	node, _ := centrifuge.New(centrifuge.Config{})
	_ = node.Run()
	defer func() { _ = node.Shutdown(context.Background()) }()

	fn := handler.Handle(node)
	result, err := fn(context.Background(), centrifuge.SharedPollEvent{
		Channel: "ch",
		Items:   []centrifuge.SharedPollItem{{Key: "k1"}},
	})
	require.NoError(t, err)
	require.Empty(t, result.Items)
}

func TestRefreshProxy_HTTP_DiffMode(t *testing.T) {
	var receivedBody []byte
	handler, server := newTestSharedPollHTTPProxy(t, func(w http.ResponseWriter, r *http.Request) {
		var err error
		receivedBody, err = io.ReadAll(r.Body)
		require.NoError(t, err)
		_, _ = w.Write([]byte(`{"result": {"items": [{"key": "k1", "data": {"v": 1}, "version": 6}]}}`))
	})
	defer server.Close()

	node, _ := centrifuge.New(centrifuge.Config{})
	_ = node.Run()
	defer func() { _ = node.Shutdown(context.Background()) }()

	fn := handler.Handle(node)
	_, err := fn(context.Background(), centrifuge.SharedPollEvent{
		Channel: "ch",
		Items: []centrifuge.SharedPollItem{
			{Key: "k1", Version: 5},
			{Key: "k2", Version: 10},
		},
	})
	require.NoError(t, err)

	var req proxyproto.SharedPollRefreshRequest
	err = json.Unmarshal(receivedBody, &req)
	require.NoError(t, err)
	require.Len(t, req.Items, 2)
	require.Equal(t, uint64(5), req.Items[0].Version)
	require.Equal(t, uint64(10), req.Items[1].Version)
}

func TestRefreshProxy_HTTP_ProxyErrorField(t *testing.T) {
	handler, server := newTestSharedPollHTTPProxy(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"error": {"code": 100, "message": "custom error"}}`))
	})
	defer server.Close()

	node, _ := centrifuge.New(centrifuge.Config{})
	_ = node.Run()
	defer func() { _ = node.Shutdown(context.Background()) }()

	fn := handler.Handle(node)
	_, err := fn(context.Background(), centrifuge.SharedPollEvent{
		Channel: "ch",
		Items:   []centrifuge.SharedPollItem{{Key: "k1"}},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "custom error")
}

func TestRefreshProxy_HTTP_NullResult(t *testing.T) {
	handler, server := newTestSharedPollHTTPProxy(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"result": null}`))
	})
	defer server.Close()

	node, _ := centrifuge.New(centrifuge.Config{})
	_ = node.Run()
	defer func() { _ = node.Shutdown(context.Background()) }()

	fn := handler.Handle(node)
	result, err := fn(context.Background(), centrifuge.SharedPollEvent{
		Channel: "ch",
		Items:   []centrifuge.SharedPollItem{{Key: "k1"}},
	})
	require.NoError(t, err)
	require.Empty(t, result.Items)
}

func TestRefreshProxy_HTTP_LargeResponse(t *testing.T) {
	handler, server := newTestSharedPollHTTPProxy(t, func(w http.ResponseWriter, r *http.Request) {
		items := make([]string, 100)
		for i := range items {
			items[i] = fmt.Sprintf(`{"key": "k%d", "data": {"i": %d}, "version": %d}`, i, i, i+1)
		}
		resp := `{"result": {"items": [` + joinStrings(items) + `]}}`
		_, _ = w.Write([]byte(resp))
	})
	defer server.Close()

	node, _ := centrifuge.New(centrifuge.Config{})
	_ = node.Run()
	defer func() { _ = node.Shutdown(context.Background()) }()

	fn := handler.Handle(node)
	requestItems := make([]centrifuge.SharedPollItem, 100)
	for i := range requestItems {
		requestItems[i] = centrifuge.SharedPollItem{Key: fmt.Sprintf("k%d", i)}
	}
	result, err := fn(context.Background(), centrifuge.SharedPollEvent{
		Channel: "ch",
		Items:   requestItems,
	})
	require.NoError(t, err)
	require.Len(t, result.Items, 100)
	require.Equal(t, "k0", result.Items[0].Key)
	require.Equal(t, uint64(1), result.Items[0].Version)
	require.Equal(t, "k99", result.Items[99].Key)
	require.Equal(t, uint64(100), result.Items[99].Version)
}

func joinStrings(ss []string) string {
	result := ""
	for i, s := range ss {
		if i > 0 {
			result += ", "
		}
		result += s
	}
	return result
}
