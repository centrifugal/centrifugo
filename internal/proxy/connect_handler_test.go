package proxy

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/centrifugal/centrifugo/v3/internal/tools"

	"github.com/centrifugal/centrifugo/v3/internal/proxyproto"

	"github.com/centrifugal/centrifugo/v3/internal/rule"
	"github.com/stretchr/testify/require"

	"github.com/centrifugal/centrifuge"
)

type connHandlerTestDepsConfig struct {
	proxyCfg            Config
	connectProxy        *HTTPConnectProxy
	connectProxyHandler *ConnectHandler
	connectEvent        centrifuge.ConnectEvent
}

func newConnHandlerTestDepsConfig(proxyEndpoint string) connHandlerTestDepsConfig {
	proxyCfg := Config{
		HTTPConfig: HTTPConfig{
			Encoder: &proxyproto.JSONEncoder{},
			Decoder: &proxyproto.JSONDecoder{},
		},
	}

	connectProxy, err := NewHTTPConnectProxy(
		proxyEndpoint,
		proxyCfg,
	)
	if err != nil {
		log.Fatalln("could not create http connect proxy: ", err)
	}

	connectProxyHandler := NewConnectHandler(ConnectHandlerConfig{
		Proxy: connectProxy,
	}, rule.NewContainer(rule.DefaultConfig))

	return connHandlerTestDepsConfig{
		proxyCfg: Config{
			HTTPConfig: HTTPConfig{
				Encoder: &proxyproto.JSONEncoder{},
				Decoder: &proxyproto.JSONDecoder{},
			},
		},
		connectProxy:        connectProxy,
		connectProxyHandler: connectProxyHandler,
		connectEvent: centrifuge.ConnectEvent{
			Transport: tools.NewTestTransport(),
		},
	}
}

type proxyHandler struct{}

func (h *proxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	_, _ = w.Write([]byte(`{}`))
}

func TestHandleWithoutResult(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	mux := http.NewServeMux()
	mux.Handle("/proxy", &proxyHandler{})
	server := httptest.NewServer(mux)
	defer server.Close()

	testDepsCfg := newConnHandlerTestDepsConfig(server.URL + "/proxy")
	connHandler := testDepsCfg.connectProxyHandler.Handle(node)

	_, err := connHandler(context.Background(), testDepsCfg.connectEvent)
	require.NoError(t, err)
}

func TestHandleWithResult(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	custData := "test"
	custDataB64 := base64.StdEncoding.EncodeToString([]byte(custData))

	mux := http.NewServeMux()
	mux.HandleFunc("/proxy", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(fmt.Sprintf(`{"result": {"user": "56", "expire_at": 1565436268, "b64data": "%s"}}`, custDataB64)))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	testDepsCfg := newConnHandlerTestDepsConfig(server.URL + "/proxy")
	connHandler := testDepsCfg.connectProxyHandler.Handle(node)

	expectedReply := centrifuge.ConnectReply{
		Credentials: &centrifuge.Credentials{
			UserID:   "56",
			ExpireAt: 1565436268,
		},
		Data: []byte(custData),
	}

	connReply, err := connHandler(context.Background(), testDepsCfg.connectEvent)
	require.NoError(t, err)
	require.Equal(t, connReply, expectedReply)

}

func TestHandleWithInvalidCustomData(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	mux := http.NewServeMux()
	mux.HandleFunc("/proxy", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{"result": {"user": "56", "expire_at": 1565436268, "b64data": "invalid data"}}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	testDepsCfg := newConnHandlerTestDepsConfig(server.URL + "/proxy")
	connHandler := testDepsCfg.connectProxyHandler.Handle(node)
	connReply, err := connHandler(context.Background(), testDepsCfg.connectEvent)
	require.ErrorIs(t, err, centrifuge.ErrorInternal)
	require.Equal(t, connReply, centrifuge.ConnectReply{})

}

func TestHandleWithContextCancel(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	mux := http.NewServeMux()
	mux.Handle("/proxy", &proxyHandler{})
	server := httptest.NewServer(mux)
	defer server.Close()

	testDepsCfg := newConnHandlerTestDepsConfig(server.URL + "/proxy")
	connHandler := testDepsCfg.connectProxyHandler.Handle(node)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	connReply, err := connHandler(ctx, testDepsCfg.connectEvent)
	require.ErrorIs(t, err, centrifuge.DisconnectNormal)
	require.Equal(t, connReply, centrifuge.ConnectReply{})
}

func TestHandleWithoutProxyServerStart(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	testDepsCfg := newConnHandlerTestDepsConfig("/proxy")
	connHandler := testDepsCfg.connectProxyHandler.Handle(node)
	connReply, err := connHandler(context.Background(), testDepsCfg.connectEvent)
	require.ErrorIs(t, err, centrifuge.ErrorInternal)
	require.Equal(t, connReply, centrifuge.ConnectReply{})
}

func TestHandleWithProxyServerCustomDisconnect(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	mux := http.NewServeMux()
	mux.HandleFunc("/proxy", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{"disconnect": {"code": 4000, "reconnect": false, "reason": "custom disconnect"}}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	testDepsCfg := newConnHandlerTestDepsConfig(server.URL + "/proxy")
	connHandler := testDepsCfg.connectProxyHandler.Handle(node)

	connReply, err := connHandler(context.Background(), testDepsCfg.connectEvent)
	expectedErr := centrifuge.Disconnect{
		Code:      4000,
		Reason:    "custom disconnect",
		Reconnect: false,
	}

	require.Equal(t, err.Error(), expectedErr.Error())
	require.Equal(t, connReply, centrifuge.ConnectReply{})
}

func TestHandleWithProxyServerCustomError(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	mux := http.NewServeMux()
	mux.HandleFunc("/proxy", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{"error": {"code": 1000, "message": "custom error"}}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	testDepsCfg := newConnHandlerTestDepsConfig(server.URL + "/proxy")
	connHandler := testDepsCfg.connectProxyHandler.Handle(node)

	connReply, err := connHandler(context.Background(), testDepsCfg.connectEvent)
	expectedErr := centrifuge.Error{
		Code:    1000,
		Message: "custom error",
	}

	require.Equal(t, err.Error(), expectedErr.Error())
	require.Equal(t, connReply, centrifuge.ConnectReply{})
}

func TestHandleWithSubscription(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	mux := http.NewServeMux()
	mux.HandleFunc("/proxy", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{"result": {"user": "56", "channels": ["test_ch"]}}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	testDepsCfg := newConnHandlerTestDepsConfig(server.URL + "/proxy")
	connHandler := testDepsCfg.connectProxyHandler.Handle(node)

	connReply, err := connHandler(context.Background(), testDepsCfg.connectEvent)
	require.NoError(t, err)
	require.NotNil(t, connReply.Subscriptions)
	require.Len(t, connReply.Subscriptions, 1)
	require.Contains(t, connReply.Subscriptions, "test_ch")

}

func TestHandleWithSubscriptionError(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	mux := http.NewServeMux()
	mux.HandleFunc("/proxy", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{"result": {"user": "56", "channels": ["test_ch:test"]}}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	testDepsCfg := newConnHandlerTestDepsConfig(server.URL + "/proxy")
	connHandler := testDepsCfg.connectProxyHandler.Handle(node)

	connReply, err := connHandler(context.Background(), testDepsCfg.connectEvent)
	require.ErrorIs(t, err, centrifuge.ErrorUnknownChannel)
	require.Equal(t, connReply, centrifuge.ConnectReply{})
}
