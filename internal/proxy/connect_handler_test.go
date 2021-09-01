package proxy

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"

	"github.com/centrifugal/centrifugo/v3/internal/proxyproto"
	"github.com/centrifugal/centrifugo/v3/internal/rule"
	"github.com/centrifugal/centrifugo/v3/internal/tools"
	"github.com/stretchr/testify/require"

	"github.com/centrifugal/centrifuge"
)

type connHandlerHTTPTestDepsConfig struct {
	connectProxyHandler *ConnectHandler
	connectEvent        centrifuge.ConnectEvent
}

func newConnHandlerHTTPTestDepsConfig(proxyEndpoint string) connHandlerHTTPTestDepsConfig {
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

	return connHandlerHTTPTestDepsConfig{
		connectProxyHandler: connectProxyHandler,
		connectEvent: centrifuge.ConnectEvent{
			Transport: tools.NewTestTransport(),
		},
	}
}

type connHandlerGRPCTestDepsConfig struct {
	connectProxyHandler *ConnectHandler
	connectEvent        centrifuge.ConnectEvent
}

func newConnHandlerGRPCTestDepsConfig(listener *bufconn.Listener) connHandlerGRPCTestDepsConfig {
	proxyCfg := Config{
		ConnectTimeout: 5 * time.Second,
		GRPCConfig: GRPCConfig{
			TestDialer: func(ctx context.Context, s string) (net.Conn, error) {
				return listener.Dial()
			},
		},
	}

	connectProxy, err := NewGRPCConnectProxy(
		listener.Addr().String(),
		proxyCfg,
	)
	if err != nil {
		log.Fatalln("could not create grpc connect proxy: ", err)
	}

	connectProxyHandler := NewConnectHandler(ConnectHandlerConfig{
		Proxy: connectProxy,
	}, rule.NewContainer(rule.DefaultConfig))

	return connHandlerGRPCTestDepsConfig{
		connectProxyHandler: connectProxyHandler,
		connectEvent: centrifuge.ConnectEvent{
			Transport: tools.NewTestTransport(),
		},
	}
}

type grpcConnHandleTestCase struct {
	cfg    connHandlerGRPCTestDepsConfig
	node   *centrifuge.Node
	server *grpc.Server
}

func newConnHandleGRPCTestCase(proxyGRPCServer proxyproto.CentrifugoProxyServer) grpcConnHandleTestCase {
	node := tools.NodeWithMemoryEngineNoHandlers()

	listener := bufconn.Listen(1024)
	server := grpc.NewServer()
	proxyproto.RegisterCentrifugoProxyServer(server, proxyGRPCServer)

	go func() {
		if err := server.Serve(listener); err != nil {
			log.Fatalf("GRPC server exited with error: %v", err)
		}
	}()

	grpcTestDepsCfg := newConnHandlerGRPCTestDepsConfig(listener)

	return grpcConnHandleTestCase{
		cfg:    grpcTestDepsCfg,
		node:   node,
		server: server,
	}
}

func teardownConnHandleGRPCTestCase(c grpcConnHandleTestCase) {
	defer func() { _ = c.node.Shutdown(context.Background()) }()
	c.server.Stop()
}

type httpConnTestCase struct {
	cfg    connHandlerHTTPTestDepsConfig
	node   *centrifuge.Node
	server *httptest.Server
	mux    *http.ServeMux
}

func newConnHandleHTTPTestCase(endpoint string) httpConnTestCase {
	node := tools.NodeWithMemoryEngineNoHandlers()

	mux := http.NewServeMux()
	server := httptest.NewServer(mux)

	httpTestDepsCfg := newConnHandlerHTTPTestDepsConfig(server.URL + endpoint)

	return httpConnTestCase{
		cfg:    httpTestDepsCfg,
		node:   node,
		server: server,
		mux:    mux,
	}
}

func teardownConnHandleHTTPTestCase(c httpConnTestCase) {
	defer func() { _ = c.node.Shutdown(context.Background()) }()
	c.server.Close()
}

type connHandleTestCase struct {
	connectProxyHandler *ConnectHandler
	protocol            string
}

func newConnHandleTestCases(httpTestCase httpConnTestCase, grpcTestCase grpcConnHandleTestCase) []connHandleTestCase {
	return []connHandleTestCase{
		{
			connectProxyHandler: grpcTestCase.cfg.connectProxyHandler,
			protocol:            "grpc",
		},
		{
			connectProxyHandler: httpTestCase.cfg.connectProxyHandler,
			protocol:            "http",
		},
	}
}

func invokeTestConnectHandler(ctx context.Context, httpTestCase httpConnTestCase, grpcTestCase grpcConnHandleTestCase, currentCase connHandleTestCase) (reply centrifuge.ConnectReply, err error) {
	var connHandler centrifuge.ConnectingHandler

	if currentCase.protocol == "http" {
		connHandler = currentCase.connectProxyHandler.Handle(httpTestCase.node)
		reply, err = connHandler(ctx, httpTestCase.cfg.connectEvent)
	} else if currentCase.protocol == "grpc" {
		connHandler = currentCase.connectProxyHandler.Handle(grpcTestCase.node)
		reply, err = connHandler(ctx, grpcTestCase.cfg.connectEvent)
	}

	return reply, err
}

func TestHandleConnectWithEmptyReply(t *testing.T) {
	grpcTestCase := newConnHandleGRPCTestCase(proxyproto.NewProxyGRPCTestServer("", proxyproto.ProxyGRPCTestServerOptions{}))
	defer teardownConnHandleGRPCTestCase(grpcTestCase)

	httpTestCase := newConnHandleHTTPTestCase("/proxy")
	httpTestCase.mux.HandleFunc("/proxy", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	})
	defer teardownConnHandleHTTPTestCase(httpTestCase)

	cases := newConnHandleTestCases(httpTestCase, grpcTestCase)
	for _, c := range cases {
		reply, err := invokeTestConnectHandler(context.Background(), httpTestCase, grpcTestCase, c)
		require.NoError(t, err, c.protocol)
		require.Equal(t, centrifuge.ConnectReply{}, reply, c.protocol)
	}
}

func TestHandleConnectWithResult(t *testing.T) {
	customData := "test"
	customDataB64 := base64.StdEncoding.EncodeToString([]byte(customData))

	opts := proxyproto.ProxyGRPCTestServerOptions{
		User:     "56",
		ExpireAt: 1565436268,
		B64Data:  customDataB64,
	}
	grpcTestCase := newConnHandleGRPCTestCase(proxyproto.NewProxyGRPCTestServer("result", opts))
	defer teardownConnHandleGRPCTestCase(grpcTestCase)

	httpTestCase := newConnHandleHTTPTestCase("/proxy")
	httpTestCase.mux.HandleFunc("/proxy", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(fmt.Sprintf(`{"result": {"user": "56", "expire_at": 1565436268, "b64data": "%s"}}`, customDataB64)))
	})
	defer teardownConnHandleHTTPTestCase(httpTestCase)

	cases := newConnHandleTestCases(httpTestCase, grpcTestCase)
	for _, c := range cases {
		expectedReply := centrifuge.ConnectReply{
			Credentials: &centrifuge.Credentials{
				UserID:   "56",
				ExpireAt: 1565436268,
			},
			Data: []byte(customData),
		}

		reply, err := invokeTestConnectHandler(context.Background(), httpTestCase, grpcTestCase, c)
		require.NoError(t, err, c.protocol)
		require.Equal(t, expectedReply, reply, c.protocol)
	}
}

func TestHandleConnectWithInvalidCustomData(t *testing.T) {
	opts := proxyproto.ProxyGRPCTestServerOptions{
		User:     "56",
		ExpireAt: 1565436268,
		B64Data:  "invalid data",
	}
	grpcTestCase := newConnHandleGRPCTestCase(proxyproto.NewProxyGRPCTestServer("result", opts))
	defer teardownConnHandleGRPCTestCase(grpcTestCase)

	httpTestCase := newConnHandleHTTPTestCase("/proxy")
	httpTestCase.mux.HandleFunc("/proxy", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{"result": {"user": "56", "expire_at": 1565436268, "b64data": "invalid data"}}`))
	})
	defer teardownConnHandleHTTPTestCase(httpTestCase)

	cases := newConnHandleTestCases(httpTestCase, grpcTestCase)
	for _, c := range cases {
		reply, err := invokeTestConnectHandler(context.Background(), httpTestCase, grpcTestCase, c)
		require.ErrorIs(t, centrifuge.ErrorInternal, err, c.protocol)
		require.Equal(t, centrifuge.ConnectReply{}, reply, c.protocol)
	}
}

func TestHandleConnectWithContextCancel(t *testing.T) {
	httpTestCase := newConnHandleHTTPTestCase("/proxy")
	httpTestCase.mux.HandleFunc("/proxy", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cases := newConnHandleTestCases(httpTestCase, grpcConnHandleTestCase{})
	for _, c := range cases {
		// @TODO add test
		if c.protocol == "grpc" {
			continue
		}

		reply, err := invokeTestConnectHandler(ctx, httpTestCase, grpcConnHandleTestCase{}, c)
		require.ErrorIs(t, centrifuge.DisconnectNormal, err, c.protocol)
		require.Equal(t, centrifuge.ConnectReply{}, reply, c.protocol)
	}
}

func TestHandleConnectWithoutProxyServerStart(t *testing.T) {
	grpcTestCase := newConnHandleGRPCTestCase(&proxyproto.ProxyGRPCTestServer{})
	teardownConnHandleGRPCTestCase(grpcTestCase)

	httpTestCase := newConnHandleHTTPTestCase("/proxy")
	teardownConnHandleHTTPTestCase(httpTestCase)

	cases := newConnHandleTestCases(httpTestCase, grpcTestCase)
	for _, c := range cases {
		reply, err := invokeTestConnectHandler(context.Background(), httpTestCase, grpcTestCase, c)
		require.ErrorIs(t, centrifuge.ErrorInternal, err, c.protocol)
		require.Equal(t, centrifuge.ConnectReply{}, reply, c.protocol)
	}
}

func TestHandleConnectWithProxyServerCustomDisconnect(t *testing.T) {
	grpcTestCase := newConnHandleGRPCTestCase(proxyproto.NewProxyGRPCTestServer("custom disconnect", proxyproto.ProxyGRPCTestServerOptions{}))
	defer teardownConnHandleGRPCTestCase(grpcTestCase)

	httpTestCase := newConnHandleHTTPTestCase("/proxy")
	httpTestCase.mux.HandleFunc("/proxy", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{"disconnect": {"code": 4000, "reconnect": false, "reason": "custom disconnect"}}`))
	})
	defer teardownConnHandleHTTPTestCase(httpTestCase)

	cases := newConnHandleTestCases(httpTestCase, grpcTestCase)
	for _, c := range cases {
		expectedErr := centrifuge.Disconnect{
			Code:      4000,
			Reason:    "custom disconnect",
			Reconnect: false,
		}

		reply, err := invokeTestConnectHandler(context.Background(), httpTestCase, grpcTestCase, c)
		require.NotNil(t, err, c.protocol)
		require.Equal(t, expectedErr.Error(), err.Error(), c.protocol)
		require.Equal(t, centrifuge.ConnectReply{}, reply, c.protocol)
	}
}

func TestHandleConnectWithProxyServerCustomError(t *testing.T) {
	grpcTestCase := newConnHandleGRPCTestCase(proxyproto.NewProxyGRPCTestServer("custom error", proxyproto.ProxyGRPCTestServerOptions{}))
	defer teardownConnHandleGRPCTestCase(grpcTestCase)

	httpTestCase := newConnHandleHTTPTestCase("/proxy")
	httpTestCase.mux.HandleFunc("/proxy", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{"error": {"code": 1000, "message": "custom error"}}`))
	})
	defer teardownConnHandleHTTPTestCase(httpTestCase)

	cases := newConnHandleTestCases(httpTestCase, grpcTestCase)
	for _, c := range cases {
		expectedErr := centrifuge.Error{
			Code:    1000,
			Message: "custom error",
		}

		reply, err := invokeTestConnectHandler(context.Background(), httpTestCase, grpcTestCase, c)
		require.NotNil(t, err, c.protocol)
		require.Equal(t, expectedErr.Error(), err.Error(), c.protocol)
		require.Equal(t, centrifuge.ConnectReply{}, reply, c.protocol)
	}
}

func TestHandleConnectWithSubscription(t *testing.T) {
	opts := proxyproto.ProxyGRPCTestServerOptions{
		User:     "56",
		Channels: []string{"test_ch"},
	}
	grpcTestCase := newConnHandleGRPCTestCase(proxyproto.NewProxyGRPCTestServer("subscription", opts))
	defer teardownConnHandleGRPCTestCase(grpcTestCase)

	httpTestCase := newConnHandleHTTPTestCase("/proxy")
	httpTestCase.mux.HandleFunc("/proxy", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{"result": {"user": "56", "channels": ["test_ch"]}}`))
	})
	defer teardownConnHandleHTTPTestCase(httpTestCase)

	cases := newConnHandleTestCases(httpTestCase, grpcTestCase)
	for _, c := range cases {
		reply, err := invokeTestConnectHandler(context.Background(), httpTestCase, grpcTestCase, c)
		require.NoError(t, err, c.protocol)
		require.NotNil(t, reply.Subscriptions, c.protocol)
		require.Len(t, reply.Subscriptions, 1, c.protocol)
		require.Contains(t, reply.Subscriptions, "test_ch", c.protocol)
	}

}

func TestHandleConnectWithSubscriptionError(t *testing.T) {
	opts := proxyproto.ProxyGRPCTestServerOptions{
		User:     "56",
		Channels: []string{"test_ch:test"},
	}
	grpcTestCase := newConnHandleGRPCTestCase(proxyproto.NewProxyGRPCTestServer("subscription error", opts))
	defer teardownConnHandleGRPCTestCase(grpcTestCase)

	httpTestCase := newConnHandleHTTPTestCase("/proxy")
	httpTestCase.mux.HandleFunc("/proxy", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{"result": {"user": "56", "channels": ["test_ch:test"]}}`))
	})
	defer teardownConnHandleHTTPTestCase(httpTestCase)

	cases := newConnHandleTestCases(httpTestCase, grpcTestCase)
	for _, c := range cases {
		reply, err := invokeTestConnectHandler(context.Background(), httpTestCase, grpcTestCase, c)
		require.ErrorIs(t, centrifuge.ErrorUnknownChannel, err, c.protocol)
		require.Equal(t, centrifuge.ConnectReply{}, reply, c.protocol)
	}
}
