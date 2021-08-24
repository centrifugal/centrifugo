package proxy

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/centrifugal/centrifugo/v3/internal/rule"

	"github.com/centrifugal/centrifuge"
	"github.com/stretchr/testify/require"

	"github.com/centrifugal/centrifugo/v3/internal/proxyproto"
	"github.com/centrifugal/centrifugo/v3/internal/tools"
)

type subscribeHandlerTestDepsConfig struct {
	subscribeProxyHandler *SubscribeHandler
	transport             *tools.TestTransport
}

func newSubscribeHandlerTestDepsConfig(proxyEndpoint string) subscribeHandlerTestDepsConfig {
	proxyCfg := Config{
		HTTPConfig: HTTPConfig{
			Encoder: &proxyproto.JSONEncoder{},
			Decoder: &proxyproto.JSONDecoder{},
		},
		SubscribeEndpoint: proxyEndpoint,
	}

	connectProxy, err := NewHTTPSubscribeProxy(
		proxyEndpoint,
		proxyCfg,
	)
	if err != nil {
		log.Fatalln("could not create http subscribe proxy: ", err)
	}

	subscribeProxyHandler := NewSubscribeHandler(SubscribeHandlerConfig{
		Proxy: connectProxy,
	})

	return subscribeHandlerTestDepsConfig{
		subscribeProxyHandler: subscribeProxyHandler,
		transport:             tools.NewTestTransport(),
	}
}

func TestHandleSubscribeWithResult(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	custData := "test"
	custDataB64 := base64.StdEncoding.EncodeToString([]byte(custData))

	mux := http.NewServeMux()
	mux.HandleFunc("/subscribe", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(fmt.Sprintf(`{"result": {"b64info": "%s"}}`, custDataB64)))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	testDepsCfg := newSubscribeHandlerTestDepsConfig(server.URL + "/subscribe")
	subscribeHandler := testDepsCfg.subscribeProxyHandler.Handle(node)

	client, closeFn, err := centrifuge.NewClient(context.Background(), node, testDepsCfg.transport)
	require.NoError(t, err)
	defer func() { _ = closeFn() }()

	chOpts := rule.ChannelOptions{
		Presence:  true,
		JoinLeave: true,
		Recover:   true,
	}
	subscribeReply, err := subscribeHandler(client, centrifuge.SubscribeEvent{}, chOpts)
	expectedSubscribeOpts := centrifuge.SubscribeOptions{
		ChannelInfo: []byte(custData),
		Presence:    true,
		JoinLeave:   true,
		Recover:     true,
	}

	require.Equal(t, expectedSubscribeOpts, subscribeReply.Options)
	require.True(t, subscribeReply.ClientSideRefresh)
	require.NoError(t, err)
}

func TestHandleSubscribeWithContextCancel(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	mux := http.NewServeMux()
	mux.HandleFunc("/subscribe", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	testDepsCfg := newSubscribeHandlerTestDepsConfig(server.URL + "/subscribe")
	subscribeHandler := testDepsCfg.subscribeProxyHandler.Handle(node)

	ctx, cancel := context.WithCancel(context.Background())
	client, closeFn, err := centrifuge.NewClient(ctx, node, testDepsCfg.transport)
	require.NoError(t, err)
	defer func() { _ = closeFn() }()

	cancel()
	subscribeReply, err := subscribeHandler(client, centrifuge.SubscribeEvent{}, rule.ChannelOptions{})
	require.NoError(t, err)
	require.Equal(t, centrifuge.SubscribeReply{}, subscribeReply)
}

func TestHandleSubscribeWithoutProxyServerStart(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	testDepsCfg := newSubscribeHandlerTestDepsConfig("/subscribe")
	subscribeHandler := testDepsCfg.subscribeProxyHandler.Handle(node)

	client, closeFn, err := centrifuge.NewClient(context.Background(), node, testDepsCfg.transport)
	require.NoError(t, err)
	defer func() { _ = closeFn() }()

	subscribeReply, err := subscribeHandler(client, centrifuge.SubscribeEvent{}, rule.ChannelOptions{})
	require.ErrorIs(t, centrifuge.ErrorInternal, err)
	require.Equal(t, centrifuge.SubscribeReply{}, subscribeReply)
}

func TestHandleSubscribeWithProxyServerCustomDisconnect(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	mux := http.NewServeMux()
	mux.HandleFunc("/subscribe", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{"disconnect": {"code": 4000, "reconnect": false, "reason": "custom disconnect"}}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	testDepsCfg := newSubscribeHandlerTestDepsConfig(server.URL + "/subscribe")
	subscribeHandler := testDepsCfg.subscribeProxyHandler.Handle(node)

	client, closeFn, err := centrifuge.NewClient(context.Background(), node, testDepsCfg.transport)
	require.NoError(t, err)
	defer func() { _ = closeFn() }()

	subscribeReply, err := subscribeHandler(client, centrifuge.SubscribeEvent{}, rule.ChannelOptions{})
	expectedErr := centrifuge.Disconnect{
		Code:      4000,
		Reason:    "custom disconnect",
		Reconnect: false,
	}

	require.Equal(t, expectedErr.Error(), err.Error())
	require.Equal(t, centrifuge.SubscribeReply{}, subscribeReply)
}

func TestHandleSubscribeWithProxyServerCustomError(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	mux := http.NewServeMux()
	mux.HandleFunc("/subscribe", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{"error": {"code": 1000, "message": "custom error"}}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	testDepsCfg := newSubscribeHandlerTestDepsConfig(server.URL + "/subscribe")
	subscribeHandler := testDepsCfg.subscribeProxyHandler.Handle(node)

	client, closeFn, err := centrifuge.NewClient(context.Background(), node, testDepsCfg.transport)
	require.NoError(t, err)
	defer func() { _ = closeFn() }()

	subscribeReply, err := subscribeHandler(client, centrifuge.SubscribeEvent{}, rule.ChannelOptions{})
	expectedErr := centrifuge.Error{
		Code:    1000,
		Message: "custom error",
	}

	require.Equal(t, expectedErr.Error(), err.Error())
	require.Equal(t, centrifuge.SubscribeReply{}, subscribeReply)
}

func TestHandleSubscribeWithInvalidCustomData(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()
	mux := http.NewServeMux()
	mux.HandleFunc("/subscribe", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{"result": {"b64info": "invalid data"}}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	testDepsCfg := newSubscribeHandlerTestDepsConfig(server.URL + "/subscribe")
	subscribeHandler := testDepsCfg.subscribeProxyHandler.Handle(node)

	client, closeFn, err := centrifuge.NewClient(context.Background(), node, testDepsCfg.transport)
	require.NoError(t, err)
	defer func() { _ = closeFn() }()

	subscribeReply, err := subscribeHandler(client, centrifuge.SubscribeEvent{}, rule.ChannelOptions{})
	require.ErrorIs(t, centrifuge.ErrorInternal, err)
	require.Equal(t, centrifuge.SubscribeReply{}, subscribeReply)
}
