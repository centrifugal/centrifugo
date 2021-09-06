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
	"github.com/centrifugal/centrifugo/v3/internal/tools"

	"github.com/centrifugal/centrifuge"
	"github.com/stretchr/testify/require"
)

type rpcHandlerTestDepsConfig struct {
	rpcProxyHandler *RPCHandler
	rpcCEvent       centrifuge.RPCEvent
	transport       *tools.TestTransport
}

func newRPCHandlerTestDepsConfig(proxyEndpoint string) rpcHandlerTestDepsConfig {
	proxyCfg := Config{
		HTTPConfig: HTTPConfig{
			Encoder: &proxyproto.JSONEncoder{},
			Decoder: &proxyproto.JSONDecoder{},
		},
	}

	rpcProxy, err := NewHTTPRPCProxy(
		proxyEndpoint,
		proxyCfg,
	)
	if err != nil {
		log.Fatalln("could not create http rpc proxy: ", err)
	}

	rpcProxyHandler := NewRPCHandler(RPCHandlerConfig{
		Proxy: rpcProxy,
	})

	return rpcHandlerTestDepsConfig{
		rpcProxyHandler: rpcProxyHandler,
		rpcCEvent:       centrifuge.RPCEvent{},
		transport:       tools.NewTestTransport(),
	}
}

func TestHandleRPCWithResult(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	clientData := `{"field":"some data"}`
	mux := http.NewServeMux()
	mux.HandleFunc("/rpc", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(fmt.Sprintf(`{"result": {"data": %s}}`, clientData)))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	testDepsCfg := newRPCHandlerTestDepsConfig(server.URL + "/rpc")
	rpcHandler := testDepsCfg.rpcProxyHandler.Handle(node)

	client, closeFn, err := centrifuge.NewClient(context.Background(), node, testDepsCfg.transport)
	require.NoError(t, err)
	defer func() { _ = closeFn() }()

	reply, err := rpcHandler(client, testDepsCfg.rpcCEvent)
	require.NoError(t, err)
	require.Equal(t, []byte(clientData), reply.Data)
}

func TestHandleRPCWithContextCancel(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	clientData := `{"field":"some data"}`
	mux := http.NewServeMux()
	mux.HandleFunc("/rpc", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(fmt.Sprintf(`{"result": {"data": %s}}`, clientData)))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	testDepsCfg := newRPCHandlerTestDepsConfig(server.URL + "/rpc")
	rpcHandler := testDepsCfg.rpcProxyHandler.Handle(node)

	ctx, cancel := context.WithCancel(context.Background())
	client, closeFn, err := centrifuge.NewClient(ctx, node, testDepsCfg.transport)
	require.NoError(t, err)
	defer func() { _ = closeFn() }()

	cancel()
	reply, err := rpcHandler(client, testDepsCfg.rpcCEvent)
	require.ErrorIs(t, centrifuge.DisconnectNormal, err)
	require.Equal(t, centrifuge.RPCReply{}, reply)
}

func TestHandleRPCWithoutProxyServerStart(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	testDepsCfg := newRPCHandlerTestDepsConfig("/rpc")
	rpcHandler := testDepsCfg.rpcProxyHandler.Handle(node)

	client, closeFn, err := centrifuge.NewClient(context.Background(), node, testDepsCfg.transport)
	require.NoError(t, err)
	defer func() { _ = closeFn() }()

	reply, err := rpcHandler(client, testDepsCfg.rpcCEvent)
	require.ErrorIs(t, centrifuge.ErrorInternal, err)
	require.Equal(t, centrifuge.RPCReply{}, reply)
}

func TestHandleRPCWithProxyServerCustomDisconnect(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	mux := http.NewServeMux()
	mux.HandleFunc("/rpc", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{"disconnect": {"code": 4000, "reconnect": false, "reason": "custom disconnect"}}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	testDepsCfg := newRPCHandlerTestDepsConfig(server.URL + "/rpc")
	rpcHandler := testDepsCfg.rpcProxyHandler.Handle(node)

	client, closeFn, err := centrifuge.NewClient(context.Background(), node, testDepsCfg.transport)
	require.NoError(t, err)
	defer func() { _ = closeFn() }()

	reply, err := rpcHandler(client, testDepsCfg.rpcCEvent)
	expectedErr := centrifuge.Disconnect{
		Code:      4000,
		Reason:    "custom disconnect",
		Reconnect: false,
	}
	require.NotNil(t, err)
	require.Equal(t, expectedErr.Error(), err.Error())
	require.Equal(t, centrifuge.RPCReply{}, reply)
}

func TestHandleRPCWithProxyServerCustomError(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	mux := http.NewServeMux()
	mux.HandleFunc("/rpc", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{"error": {"code": 1000, "message": "custom error"}}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	testDepsCfg := newRPCHandlerTestDepsConfig(server.URL + "/rpc")
	rpcHandler := testDepsCfg.rpcProxyHandler.Handle(node)

	client, closeFn, err := centrifuge.NewClient(context.Background(), node, testDepsCfg.transport)
	require.NoError(t, err)
	defer func() { _ = closeFn() }()

	reply, err := rpcHandler(client, testDepsCfg.rpcCEvent)
	expectedErr := centrifuge.Error{
		Code:    1000,
		Message: "custom error",
	}
	require.NotNil(t, err)
	require.Equal(t, expectedErr.Error(), err.Error())
	require.Equal(t, centrifuge.RPCReply{}, reply)
}

func TestHandleRPCWithValidCustomData(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	customData := "test"
	customDataB64 := base64.StdEncoding.EncodeToString([]byte(customData))

	mux := http.NewServeMux()
	mux.HandleFunc("/rpc", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(fmt.Sprintf(`{"result": {"b64data": "%s"}}`, customDataB64)))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	testDepsCfg := newRPCHandlerTestDepsConfig(server.URL + "/rpc")
	rpcHandler := testDepsCfg.rpcProxyHandler.Handle(node)

	client, closeFn, err := centrifuge.NewClient(context.Background(), node, testDepsCfg.transport)
	require.NoError(t, err)
	defer func() { _ = closeFn() }()

	reply, err := rpcHandler(client, testDepsCfg.rpcCEvent)
	require.NoError(t, err)
	require.Equal(t, []byte(customData), reply.Data)
}

func TestHandleRPCWithInvalidCustomData(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()
	mux := http.NewServeMux()
	mux.HandleFunc("/rpc", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{"result": {"b64data": "invalid data"}}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	testDepsCfg := newRPCHandlerTestDepsConfig(server.URL + "/rpc")
	rpcHandler := testDepsCfg.rpcProxyHandler.Handle(node)

	client, closeFn, err := centrifuge.NewClient(context.Background(), node, testDepsCfg.transport)
	require.NoError(t, err)
	defer func() { _ = closeFn() }()

	reply, err := rpcHandler(client, testDepsCfg.rpcCEvent)
	require.ErrorIs(t, centrifuge.ErrorInternal, err)
	require.Equal(t, centrifuge.RPCReply{}, reply)
}
