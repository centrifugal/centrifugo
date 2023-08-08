package proxy

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/v5/internal/rule"
	"github.com/centrifugal/centrifugo/v5/internal/tools"

	"github.com/centrifugal/centrifuge"
	"github.com/stretchr/testify/require"
)

type grpcConnHandleTestCase struct {
	*tools.CommonGRPCProxyTestCase
	connectProxyHandler *ConnectHandler
}

func getTestGrpcProxy(commonProxyTestCase *tools.CommonGRPCProxyTestCase) Proxy {
	return Proxy{
		Endpoint: commonProxyTestCase.Listener.Addr().String(),
		Timeout:  tools.Duration(5 * time.Second),
		testGrpcDialer: func(ctx context.Context, s string) (net.Conn, error) {
			return commonProxyTestCase.Listener.Dial()
		},
	}
}

func getTestHttpProxy(commonProxyTestCase *tools.CommonHTTPProxyTestCase, endpoint string) Proxy {
	return Proxy{
		Endpoint: commonProxyTestCase.Server.URL + endpoint,
		Timeout:  tools.Duration(5 * time.Second),
		StaticHttpHeaders: map[string]string{
			"X-Test": "test",
		},
	}
}

func newConnHandleGRPCTestCase(ctx context.Context, proxyGRPCServer proxyGRPCTestServer) grpcConnHandleTestCase {
	commonProxyTestCase := tools.NewCommonGRPCProxyTestCase(ctx, proxyGRPCServer)

	connectProxy, err := NewGRPCConnectProxy(getTestGrpcProxy(commonProxyTestCase))
	if err != nil {
		log.Fatalln("could not create grpc connect proxy: ", err)
	}

	ruleContainer, err := rule.NewContainer(rule.DefaultConfig)
	if err != nil {
		panic(err)
	}

	connectProxyHandler := NewConnectHandler(ConnectHandlerConfig{
		Proxy: connectProxy,
	}, ruleContainer)

	return grpcConnHandleTestCase{commonProxyTestCase, connectProxyHandler}
}

type httpConnHandleTestCase struct {
	*tools.CommonHTTPProxyTestCase
	connectProxyHandler *ConnectHandler
}

func newConnHandleHTTPTestCase(ctx context.Context, endpoint string) httpConnHandleTestCase {
	commonProxyTestCase := tools.NewCommonHTTPProxyTestCase(ctx)

	connectProxy, err := NewHTTPConnectProxy(getTestHttpProxy(commonProxyTestCase, endpoint))
	if err != nil {
		log.Fatalln("could not create http connect proxy: ", err)
	}

	ruleContainer, err := rule.NewContainer(rule.DefaultConfig)
	if err != nil {
		panic(err)
	}

	connectProxyHandler := NewConnectHandler(ConnectHandlerConfig{
		Proxy: connectProxy,
	}, ruleContainer)

	return httpConnHandleTestCase{commonProxyTestCase, connectProxyHandler}
}

type connHandleTestCase struct {
	connectProxyHandler *ConnectHandler
	protocol            string
	node                *centrifuge.Node
}

func (c connHandleTestCase) invokeHandle(ctx context.Context) (reply centrifuge.ConnectReply, err error) {
	connectEvent := centrifuge.ConnectEvent{
		Transport: tools.NewTestTransport(),
	}

	connHandler := c.connectProxyHandler.Handle(c.node)
	reply, _, err = connHandler(ctx, connectEvent)

	return reply, err
}

func newConnHandleTestCases(httpTestCase httpConnHandleTestCase, grpcTestCase grpcConnHandleTestCase) []connHandleTestCase {
	return []connHandleTestCase{
		{
			connectProxyHandler: grpcTestCase.connectProxyHandler,
			node:                grpcTestCase.Node,
			protocol:            "grpc",
		},
		{
			connectProxyHandler: httpTestCase.connectProxyHandler,
			node:                httpTestCase.Node,
			protocol:            "http",
		},
	}
}

func TestHandleConnectWithEmptyReply(t *testing.T) {
	grpcTestCase := newConnHandleGRPCTestCase(context.Background(), newProxyGRPCTestServer("", proxyGRPCTestServerOptions{}))
	defer grpcTestCase.Teardown()

	httpTestCase := newConnHandleHTTPTestCase(context.Background(), "/proxy")
	httpTestCase.Mux.HandleFunc("/proxy", func(w http.ResponseWriter, req *http.Request) {
		require.Equal(t, "test", req.Header.Get("X-Test"))
		_, _ = w.Write([]byte(`{}`))
	})
	defer httpTestCase.Teardown()

	cases := newConnHandleTestCases(httpTestCase, grpcTestCase)
	for _, c := range cases {
		reply, err := c.invokeHandle(context.Background())
		require.NoError(t, err, c.protocol)
		require.Equal(t, centrifuge.ConnectReply{}, reply, c.protocol)
	}
}

func TestHandleConnectWithResult(t *testing.T) {
	customData := "test"
	customDataB64 := base64.StdEncoding.EncodeToString([]byte(customData))

	opts := proxyGRPCTestServerOptions{
		User:     "56",
		ExpireAt: 1565436268,
		B64Data:  customDataB64,
	}
	grpcTestCase := newConnHandleGRPCTestCase(context.Background(), newProxyGRPCTestServer("result", opts))
	defer grpcTestCase.Teardown()

	httpTestCase := newConnHandleHTTPTestCase(context.Background(), "/proxy")
	httpTestCase.Mux.HandleFunc("/proxy", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(fmt.Sprintf(`{"result": {"user": "56", "expire_at": 1565436268, "b64data": "%s"}}`, customDataB64)))
	})
	defer httpTestCase.Teardown()

	cases := newConnHandleTestCases(httpTestCase, grpcTestCase)
	for _, c := range cases {
		expectedReply := centrifuge.ConnectReply{
			Credentials: &centrifuge.Credentials{
				UserID:   "56",
				ExpireAt: 1565436268,
			},
			Data: []byte(customData),
		}

		reply, err := c.invokeHandle(context.Background())
		require.NoError(t, err, c.protocol)
		require.Equal(t, expectedReply, reply, c.protocol)
	}
}

func TestHandleConnectWithInvalidCustomData(t *testing.T) {
	opts := proxyGRPCTestServerOptions{
		User:     "56",
		ExpireAt: 1565436268,
		B64Data:  "invalid data",
	}
	grpcTestCase := newConnHandleGRPCTestCase(context.Background(), newProxyGRPCTestServer("result", opts))
	defer grpcTestCase.Teardown()

	httpTestCase := newConnHandleHTTPTestCase(context.Background(), "/proxy")
	httpTestCase.Mux.HandleFunc("/proxy", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{"result": {"user": "56", "expire_at": 1565436268, "b64data": "invalid data"}}`))
	})
	defer httpTestCase.Teardown()

	cases := newConnHandleTestCases(httpTestCase, grpcTestCase)
	for _, c := range cases {
		reply, err := c.invokeHandle(context.Background())
		require.ErrorIs(t, centrifuge.ErrorInternal, err, c.protocol)
		require.Equal(t, centrifuge.ConnectReply{}, reply, c.protocol)
	}
}

func TestHandleConnectWithContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	grpcTestCase := newConnHandleGRPCTestCase(ctx, proxyGRPCTestServer{})
	defer grpcTestCase.Teardown()

	httpTestCase := newConnHandleHTTPTestCase(ctx, "/proxy")
	httpTestCase.Mux.HandleFunc("/proxy", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	})

	cases := newConnHandleTestCases(httpTestCase, grpcTestCase)
	for _, c := range cases {
		reply, err := c.invokeHandle(ctx)
		require.ErrorIs(t, centrifuge.DisconnectConnectionClosed, err, c.protocol)
		require.Equal(t, centrifuge.ConnectReply{}, reply, c.protocol)
	}
}

func TestHandleConnectWithoutProxyServerStart(t *testing.T) {
	grpcTestCase := newConnHandleGRPCTestCase(context.Background(), proxyGRPCTestServer{})
	grpcTestCase.Teardown()

	httpTestCase := newConnHandleHTTPTestCase(context.Background(), "/proxy")
	httpTestCase.Teardown()

	cases := newConnHandleTestCases(httpTestCase, grpcTestCase)
	for _, c := range cases {
		reply, err := c.invokeHandle(context.Background())
		require.ErrorIs(t, centrifuge.ErrorInternal, err, c.protocol)
		require.Equal(t, centrifuge.ConnectReply{}, reply, c.protocol)
	}
}

func TestHandleConnectWithProxyServerCustomDisconnect(t *testing.T) {
	grpcTestCase := newConnHandleGRPCTestCase(context.Background(), newProxyGRPCTestServer("custom disconnect", proxyGRPCTestServerOptions{}))
	defer grpcTestCase.Teardown()

	httpTestCase := newConnHandleHTTPTestCase(context.Background(), "/proxy")
	httpTestCase.Mux.HandleFunc("/proxy", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{"disconnect": {"code": 4000, "reconnect": false, "reason": "custom disconnect"}}`))
	})
	defer httpTestCase.Teardown()

	cases := newConnHandleTestCases(httpTestCase, grpcTestCase)
	for _, c := range cases {
		expectedErr := centrifuge.Disconnect{
			Code:   4000,
			Reason: "custom disconnect",
		}

		reply, err := c.invokeHandle(context.Background())
		require.NotNil(t, err, c.protocol)
		require.Equal(t, expectedErr.Error(), err.Error(), c.protocol)
		require.Equal(t, centrifuge.ConnectReply{}, reply, c.protocol)
	}
}

func TestHandleConnectWithProxyServerCustomError(t *testing.T) {
	grpcTestCase := newConnHandleGRPCTestCase(context.Background(), newProxyGRPCTestServer("custom error", proxyGRPCTestServerOptions{}))
	defer grpcTestCase.Teardown()

	httpTestCase := newConnHandleHTTPTestCase(context.Background(), "/proxy")
	httpTestCase.Mux.HandleFunc("/proxy", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{"error": {"code": 1000, "message": "custom error"}}`))
	})
	defer httpTestCase.Teardown()

	cases := newConnHandleTestCases(httpTestCase, grpcTestCase)
	for _, c := range cases {
		expectedErr := centrifuge.Error{
			Code:    1000,
			Message: "custom error",
		}

		reply, err := c.invokeHandle(context.Background())
		require.NotNil(t, err, c.protocol)
		require.Equal(t, expectedErr.Error(), err.Error(), c.protocol)
		require.Equal(t, centrifuge.ConnectReply{}, reply, c.protocol)
	}
}

func TestHandleConnectWithSubscription(t *testing.T) {
	opts := proxyGRPCTestServerOptions{
		User:     "56",
		Channels: []string{"test_ch"},
	}
	grpcTestCase := newConnHandleGRPCTestCase(context.Background(), newProxyGRPCTestServer("subscription", opts))
	defer grpcTestCase.Teardown()

	httpTestCase := newConnHandleHTTPTestCase(context.Background(), "/proxy")
	httpTestCase.Mux.HandleFunc("/proxy", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{"result": {"user": "56", "channels": ["test_ch"]}}`))
	})
	defer httpTestCase.Teardown()

	cases := newConnHandleTestCases(httpTestCase, grpcTestCase)
	for _, c := range cases {
		reply, err := c.invokeHandle(context.Background())
		require.NoError(t, err, c.protocol)
		require.NotNil(t, reply.Subscriptions, c.protocol)
		require.Len(t, reply.Subscriptions, 1, c.protocol)
		require.Contains(t, reply.Subscriptions, "test_ch", c.protocol)
	}

}

func TestHandleConnectWithSubscriptionError(t *testing.T) {
	opts := proxyGRPCTestServerOptions{
		User:     "56",
		Channels: []string{"test_ch:test"},
	}
	grpcTestCase := newConnHandleGRPCTestCase(context.Background(), newProxyGRPCTestServer("subscription error", opts))
	defer grpcTestCase.Teardown()

	httpTestCase := newConnHandleHTTPTestCase(context.Background(), "/proxy")
	httpTestCase.Mux.HandleFunc("/proxy", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{"result": {"user": "56", "channels": ["test_ch:test"]}}`))
	})
	defer httpTestCase.Teardown()

	cases := newConnHandleTestCases(httpTestCase, grpcTestCase)
	for _, c := range cases {
		reply, err := c.invokeHandle(context.Background())
		require.ErrorIs(t, centrifuge.ErrorUnknownChannel, err, c.protocol)
		require.Equal(t, centrifuge.ConnectReply{}, reply, c.protocol)
	}
}
