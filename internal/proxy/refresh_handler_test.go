package proxy

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/v3/internal/proxyproto"
	"github.com/centrifugal/centrifugo/v3/internal/tools"

	"github.com/centrifugal/centrifuge"
	"github.com/stretchr/testify/require"
)

type refreshHandlerTestDepsConfig struct {
	refreshProxyHandler *RefreshHandler
	transport           *tools.TestTransport
}

func newRefreshHandlerTestDepsConfig(proxyEndpoint string) refreshHandlerTestDepsConfig {
	proxyCfg := Config{
		HTTPConfig: HTTPConfig{
			Encoder: &proxyproto.JSONEncoder{},
			Decoder: &proxyproto.JSONDecoder{},
		},
		RefreshEndpoint: proxyEndpoint,
	}

	connectProxy, err := NewHTTPRefreshProxy(
		proxyEndpoint,
		proxyCfg,
	)
	if err != nil {
		log.Fatalln("could not create http refresh proxy: ", err)
	}

	refreshProxyHandler := NewRefreshHandler(RefreshHandlerConfig{
		Proxy: connectProxy,
	})

	return refreshHandlerTestDepsConfig{
		refreshProxyHandler: refreshProxyHandler,
		transport:           tools.NewTestTransport(),
	}
}

func TestHandleRefreshWithCredentials(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	customData := "test"
	customDataB64 := base64.StdEncoding.EncodeToString([]byte(customData))
	expireAt := 1565436268

	mux := http.NewServeMux()
	mux.HandleFunc("/refresh", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(fmt.Sprintf(`{"result": {"expire_at": %d, "b64info": "%s"}}`,
			expireAt,
			customDataB64,
		)))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	testDepsCfg := newRefreshHandlerTestDepsConfig(server.URL + "/refresh")
	refreshHandler := testDepsCfg.refreshProxyHandler.Handle(node)

	client, closeFn, err := centrifuge.NewClient(context.Background(), node, testDepsCfg.transport)
	require.NoError(t, err)
	defer func() { _ = closeFn() }()

	refreshReply, err := refreshHandler(client, centrifuge.RefreshEvent{})
	require.NoError(t, err)
	require.Equal(t, int64(expireAt), refreshReply.ExpireAt)
	require.False(t, refreshReply.Expired)
	require.Equal(t, customData, string(refreshReply.Info))
}

func TestHandleRefreshWithEmptyCredentials(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()
	mux := http.NewServeMux()
	mux.HandleFunc("/refresh", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	testDepsCfg := newRefreshHandlerTestDepsConfig(server.URL + "/refresh")
	refreshHandler := testDepsCfg.refreshProxyHandler.Handle(node)

	client, closeFn, err := centrifuge.NewClient(context.Background(), node, testDepsCfg.transport)
	require.NoError(t, err)
	defer func() { _ = closeFn() }()

	refreshReply, err := refreshHandler(client, centrifuge.RefreshEvent{})
	require.NoError(t, err)
	require.Equal(t, int64(0), refreshReply.ExpireAt)
	require.True(t, refreshReply.Expired)
	require.Equal(t, "", string(refreshReply.Info))
}

func TestHandleRefreshWithExpired(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()
	mux := http.NewServeMux()
	mux.HandleFunc("/refresh", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{"result": {"expired": true}}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	testDepsCfg := newRefreshHandlerTestDepsConfig(server.URL + "/refresh")
	refreshHandler := testDepsCfg.refreshProxyHandler.Handle(node)

	client, closeFn, err := centrifuge.NewClient(context.Background(), node, testDepsCfg.transport)
	require.NoError(t, err)
	defer func() { _ = closeFn() }()

	refreshReply, err := refreshHandler(client, centrifuge.RefreshEvent{})
	require.NoError(t, err)
	require.Equal(t, int64(0), refreshReply.ExpireAt)
	require.True(t, refreshReply.Expired)
	require.Equal(t, "", string(refreshReply.Info))
}

func TestHandleRefreshWithContextCancel(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	mux := http.NewServeMux()
	mux.HandleFunc("/refresh", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	testDepsCfg := newRefreshHandlerTestDepsConfig(server.URL + "/refresh")
	refreshHandler := testDepsCfg.refreshProxyHandler.Handle(node)

	ctx, cancel := context.WithCancel(context.Background())
	client, closeFn, err := centrifuge.NewClient(ctx, node, testDepsCfg.transport)
	require.NoError(t, err)
	defer func() { _ = closeFn() }()

	cancel()
	refreshReply, err := refreshHandler(client, centrifuge.RefreshEvent{})
	require.NoError(t, err)
	require.Equal(t, centrifuge.RefreshReply{}, refreshReply)
}

func TestHandleRefreshWithoutProxyServerStart(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	testDepsCfg := newRefreshHandlerTestDepsConfig("/refresh")
	refreshHandler := testDepsCfg.refreshProxyHandler.Handle(node)

	client, closeFn, err := centrifuge.NewClient(context.Background(), node, testDepsCfg.transport)
	require.NoError(t, err)
	defer func() { _ = closeFn() }()

	refreshReply, err := refreshHandler(client, centrifuge.RefreshEvent{})
	expectedReply := centrifuge.RefreshReply{
		ExpireAt: time.Now().Unix() + 60,
	}
	require.NoError(t, err)
	require.Equal(t, expectedReply, refreshReply)
}

func TestHandleRefreshWithInvalidCustomData(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()
	mux := http.NewServeMux()
	mux.HandleFunc("/refresh", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{"result": {"b64info": "invalid data"}}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	testDepsCfg := newRefreshHandlerTestDepsConfig(server.URL + "/refresh")
	refreshHandler := testDepsCfg.refreshProxyHandler.Handle(node)

	client, closeFn, err := centrifuge.NewClient(context.Background(), node, testDepsCfg.transport)
	require.NoError(t, err)
	defer func() { _ = closeFn() }()

	refreshReply, err := refreshHandler(client, centrifuge.RefreshEvent{})
	require.ErrorIs(t, centrifuge.ErrorInternal, err)
	require.Equal(t, centrifuge.RefreshReply{}, refreshReply)
}
