package proxy

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/config"
	"github.com/centrifugal/centrifugo/v6/internal/configtypes"
	"github.com/centrifugal/centrifugo/v6/internal/tools"

	"github.com/centrifugal/centrifuge"
	"github.com/stretchr/testify/require"
)

type grpcRPCHandleTestCase struct {
	*tools.CommonGRPCProxyTestCase
	rpcProxyHandler *RPCHandler
}

func newRPCHandlerGRPCTestCase(ctx context.Context, proxyGRPCServer proxyGRPCTestServer) grpcRPCHandleTestCase {
	commonProxyTestCase := tools.NewCommonGRPCProxyTestCase(ctx, proxyGRPCServer)

	rpcProxy, err := NewGRPCRPCProxy("default", getTestGrpcProxy(commonProxyTestCase))
	if err != nil {
		log.Fatalln("could not create grpc rpc proxy: ", err)
	}

	rpcProxyHandler := NewRPCHandler(RPCHandlerConfig{
		Proxies: map[string]RPCProxy{
			"test": rpcProxy,
		},
	})

	return grpcRPCHandleTestCase{commonProxyTestCase, rpcProxyHandler}
}

type httpRPCHandleTestCase struct {
	*tools.CommonHTTPProxyTestCase
	rpcProxyHandler *RPCHandler
}

func newRPCHandlerHTTPTestCase(ctx context.Context, endpoint string) httpRPCHandleTestCase {
	commonProxyTestCase := tools.NewCommonHTTPProxyTestCase(ctx)

	rpcProxy, err := NewHTTPRPCProxy(getTestHttpProxy(commonProxyTestCase, endpoint))
	if err != nil {
		log.Fatalln("could not create http rpc proxy: ", err)
	}

	rpcProxyHandler := NewRPCHandler(RPCHandlerConfig{
		Proxies: map[string]RPCProxy{
			"test": rpcProxy,
		},
	})

	return httpRPCHandleTestCase{commonProxyTestCase, rpcProxyHandler}
}

type rpcHandleTestCase struct {
	rpcProxyHandler *RPCHandler
	protocol        string
	node            *centrifuge.Node
	client          *centrifuge.Client
}

func (c rpcHandleTestCase) invokeHandle() (reply centrifuge.RPCReply, err error) {
	rpcHandler := c.rpcProxyHandler.Handle()
	cfg := config.DefaultConfig()
	cfg.RPC = configtypes.RPC{
		WithoutNamespace: configtypes.RpcOptions{
			ProxyName:    "test",
			ProxyEnabled: true,
		},
	}
	cfg.Proxies = configtypes.NamedProxies{
		{
			Name: "test",
			Proxy: configtypes.Proxy{
				Endpoint: "https://example.com/rpc",
				Timeout:  configtypes.Duration(time.Second),
			},
		},
	}
	cfgContainer, err := config.NewContainer(cfg)
	if err != nil {
		return centrifuge.RPCReply{}, err
	}
	reply, err = rpcHandler(c.client, centrifuge.RPCEvent{}, cfgContainer, PerCallData{})

	return reply, err
}

func newRPCHandlerTestCases(httpTestCase httpRPCHandleTestCase, grpcTestCase grpcRPCHandleTestCase) []rpcHandleTestCase {
	return []rpcHandleTestCase{
		{
			rpcProxyHandler: grpcTestCase.rpcProxyHandler,
			node:            grpcTestCase.Node,
			client:          grpcTestCase.Client,
			protocol:        "grpc",
		},
		{
			rpcProxyHandler: httpTestCase.rpcProxyHandler,
			node:            httpTestCase.Node,
			client:          httpTestCase.Client,
			protocol:        "http",
		},
	}
}

func TestHandleRPCWithResult(t *testing.T) {
	clientData := `{"field":"some data"}`
	opts := proxyGRPCTestServerOptions{Data: []byte(clientData)}

	grpcTestCase := newRPCHandlerGRPCTestCase(context.Background(), newProxyGRPCTestServer("result", opts))
	defer grpcTestCase.Teardown()

	httpTestCase := newRPCHandlerHTTPTestCase(context.Background(), "/rpc")
	httpTestCase.Mux.HandleFunc("/rpc", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(fmt.Sprintf(`{"result": {"data": %s}}`, clientData)))
	})
	defer httpTestCase.Teardown()

	cases := newRPCHandlerTestCases(httpTestCase, grpcTestCase)
	for _, c := range cases {
		reply, err := c.invokeHandle()
		require.NoError(t, err, c.protocol)
		require.Equal(t, []byte(clientData), reply.Data)
	}
}

func TestHandleRPCWithContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	clientData := `{"field":"some data"}`
	opts := proxyGRPCTestServerOptions{Data: []byte(clientData)}

	grpcTestCase := newRPCHandlerGRPCTestCase(ctx, newProxyGRPCTestServer("result", opts))
	defer grpcTestCase.Teardown()

	httpTestCase := newRPCHandlerHTTPTestCase(ctx, "/rpc")
	httpTestCase.Mux.HandleFunc("/rpc", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(fmt.Sprintf(`{"result": {"data": %s}}`, clientData)))
	})
	defer httpTestCase.Teardown()

	cases := newRPCHandlerTestCases(httpTestCase, grpcTestCase)
	for _, c := range cases {
		reply, err := c.invokeHandle()
		require.ErrorIs(t, err, centrifuge.DisconnectConnectionClosed, c.protocol)
		require.Equal(t, centrifuge.RPCReply{}, reply, c.protocol)
	}
}

func TestHandleRPCWithoutProxyServerStart(t *testing.T) {
	grpcTestCase := newRPCHandlerGRPCTestCase(context.Background(), proxyGRPCTestServer{})
	grpcTestCase.Teardown()

	httpTestCase := newRPCHandlerHTTPTestCase(context.Background(), "/rpc")
	httpTestCase.Teardown()

	cases := newRPCHandlerTestCases(httpTestCase, grpcTestCase)
	for _, c := range cases {
		reply, err := c.invokeHandle()
		require.Error(t, err, c.protocol)
		require.Equal(t, centrifuge.RPCReply{}, reply)
	}
}

func TestHandleRPCWithProxyServerCustomDisconnect(t *testing.T) {
	grpcTestCase := newRPCHandlerGRPCTestCase(context.Background(), newProxyGRPCTestServer("custom disconnect", proxyGRPCTestServerOptions{}))
	defer grpcTestCase.Teardown()

	httpTestCase := newRPCHandlerHTTPTestCase(context.Background(), "/rpc")
	httpTestCase.Mux.HandleFunc("/rpc", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{"disconnect": {"code": 4000, "reconnect": false, "reason": "custom disconnect"}}`))
	})
	defer httpTestCase.Teardown()

	expectedErr := centrifuge.Disconnect{
		Code:   4000,
		Reason: "custom disconnect",
	}

	cases := newRPCHandlerTestCases(httpTestCase, grpcTestCase)
	for _, c := range cases {
		reply, err := c.invokeHandle()
		require.NotNil(t, err, c.protocol)
		require.Equal(t, expectedErr.Error(), err.Error(), c.protocol)
		require.Equal(t, centrifuge.RPCReply{}, reply, c.protocol)
	}
}

func TestHandleRPCWithProxyServerCustomError(t *testing.T) {
	grpcTestCase := newRPCHandlerGRPCTestCase(context.Background(), newProxyGRPCTestServer("custom error", proxyGRPCTestServerOptions{}))
	defer grpcTestCase.Teardown()

	httpTestCase := newRPCHandlerHTTPTestCase(context.Background(), "/rpc")
	httpTestCase.Mux.HandleFunc("/rpc", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{"error": {"code": 1000, "message": "custom error"}}`))
	})
	defer httpTestCase.Teardown()

	expectedErr := centrifuge.Error{
		Code:    1000,
		Message: "custom error",
	}

	cases := newRPCHandlerTestCases(httpTestCase, grpcTestCase)
	for _, c := range cases {
		reply, err := c.invokeHandle()
		require.NotNil(t, err, c.protocol)
		require.Equal(t, expectedErr.Error(), err.Error(), c.protocol)
		require.Equal(t, centrifuge.RPCReply{}, reply, c.protocol)
	}
}

func TestHandleRPCWithValidCustomData(t *testing.T) {
	customData := `test`
	customDataB64 := base64.StdEncoding.EncodeToString([]byte(customData))
	opts := proxyGRPCTestServerOptions{B64Data: customDataB64}

	grpcTestCase := newRPCHandlerGRPCTestCase(context.Background(), newProxyGRPCTestServer("custom data", opts))
	defer grpcTestCase.Teardown()

	httpTestCase := newRPCHandlerHTTPTestCase(context.Background(), "/rpc")
	httpTestCase.Mux.HandleFunc("/rpc", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(fmt.Sprintf(`{"result": {"b64data": "%s"}}`, customDataB64)))
	})
	defer httpTestCase.Teardown()

	cases := newRPCHandlerTestCases(httpTestCase, grpcTestCase)
	for _, c := range cases {
		reply, err := c.invokeHandle()
		require.NoError(t, err, c.protocol)
		require.Equal(t, []byte(customData), reply.Data)
	}
}

func TestHandleRPCWithInvalidCustomData(t *testing.T) {
	opts := proxyGRPCTestServerOptions{
		B64Data: "invalid data",
	}
	grpcTestCase := newRPCHandlerGRPCTestCase(context.Background(), newProxyGRPCTestServer("custom data", opts))
	defer grpcTestCase.Teardown()

	httpTestCase := newRPCHandlerHTTPTestCase(context.Background(), "/rpc")
	httpTestCase.Mux.HandleFunc("/rpc", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{"result": {"b64data": "invalid data"}}`))
	})
	defer httpTestCase.Teardown()

	cases := newRPCHandlerTestCases(httpTestCase, grpcTestCase)
	for _, c := range cases {
		reply, err := c.invokeHandle()
		require.ErrorIs(t, err, centrifuge.ErrorInternal, c.protocol)
		require.Equal(t, centrifuge.RPCReply{}, reply, c.protocol)
	}
}
