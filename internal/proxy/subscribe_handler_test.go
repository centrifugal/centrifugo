package proxy

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/centrifugal/centrifugo/v3/internal/proxyproto"
	"github.com/centrifugal/centrifugo/v3/internal/rule"
	"github.com/centrifugal/centrifugo/v3/internal/tools"

	"github.com/centrifugal/centrifuge"
	"github.com/stretchr/testify/require"
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

	customData := "test"
	customDataB64 := base64.StdEncoding.EncodeToString([]byte(customData))

	mux := http.NewServeMux()
	mux.HandleFunc("/subscribe", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(fmt.Sprintf(`{"result": {"b64info": "%s"}}`, customDataB64)))
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
		Position:  true,
	}
	subscribeReply, err := subscribeHandler(client, centrifuge.SubscribeEvent{}, chOpts)
	expectedSubscribeOpts := centrifuge.SubscribeOptions{
		ChannelInfo: []byte(customData),
		Presence:    true,
		JoinLeave:   true,
		Recover:     true,
		Position:    true,
	}
	require.NoError(t, err)
	require.Equal(t, expectedSubscribeOpts, subscribeReply.Options)
	require.True(t, subscribeReply.ClientSideRefresh)
	require.NoError(t, err)
}

func TestHandleSubscribeWithOverride(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	customData := "test"
	customDataB64 := base64.StdEncoding.EncodeToString([]byte(customData))

	mux := http.NewServeMux()
	mux.HandleFunc("/subscribe", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(fmt.Sprintf(`{"result": {"b64info": "%s", "override": {"join_leave": {"value": false}, "presence": {"value": true}, "position": {"value": true}, "recover": {"value": true}}}}`, customDataB64)))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	testDepsCfg := newSubscribeHandlerTestDepsConfig(server.URL + "/subscribe")
	subscribeHandler := testDepsCfg.subscribeProxyHandler.Handle(node)

	client, closeFn, err := centrifuge.NewClient(context.Background(), node, testDepsCfg.transport)
	require.NoError(t, err)
	defer func() { _ = closeFn() }()

	chOpts := rule.ChannelOptions{
		Presence:  false,
		JoinLeave: true,
		Recover:   false,
		Position:  false,
	}
	subscribeReply, err := subscribeHandler(client, centrifuge.SubscribeEvent{}, chOpts)
	expectedSubscribeOpts := centrifuge.SubscribeOptions{
		ChannelInfo: []byte(customData),
		Presence:    true,
		JoinLeave:   false,
		Position:    true,
		Recover:     true,
	}
	require.NoError(t, err)
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
	require.ErrorIs(t, centrifuge.DisconnectNormal, err)
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
	require.NotNil(t, err)
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
	require.NotNil(t, err)
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
