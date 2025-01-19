package proxy

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/tools"

	"github.com/centrifugal/centrifuge"
	"github.com/stretchr/testify/require"
)

type grpcRefreshHandleTestCase struct {
	*tools.CommonGRPCProxyTestCase
	refreshProxyHandler *RefreshHandler
}

func newRefreshHandlerGRPCTestCase(ctx context.Context, proxyGRPCServer proxyGRPCTestServer) grpcRefreshHandleTestCase {
	commonProxyTestCase := tools.NewCommonGRPCProxyTestCase(ctx, proxyGRPCServer)

	refreshProxy, err := NewGRPCRefreshProxy("default", getTestGrpcProxy(commonProxyTestCase))
	if err != nil {
		log.Fatalln("could not create grpc refresh proxy: ", err)
	}

	refreshProxyHandler := NewRefreshHandler(RefreshHandlerConfig{
		Proxy: refreshProxy,
	})

	return grpcRefreshHandleTestCase{commonProxyTestCase, refreshProxyHandler}
}

type httpRefreshHandleTestCase struct {
	*tools.CommonHTTPProxyTestCase
	refreshProxyHandler *RefreshHandler
}

func newRefreshHandlerHTTPTestCase(ctx context.Context, endpoint string) httpRefreshHandleTestCase {
	commonProxyTestCase := tools.NewCommonHTTPProxyTestCase(ctx)

	refreshProxy, err := NewHTTPRefreshProxy(getTestHttpProxy(commonProxyTestCase, endpoint))
	if err != nil {
		log.Fatalln("could not create http refresh proxy: ", err)
	}

	refreshProxyHandler := NewRefreshHandler(RefreshHandlerConfig{
		Proxy: refreshProxy,
	})

	return httpRefreshHandleTestCase{commonProxyTestCase, refreshProxyHandler}
}

type refreshHandleTestCase struct {
	refreshProxyHandler *RefreshHandler
	protocol            string
	node                *centrifuge.Node
	client              *centrifuge.Client
}

func (c refreshHandleTestCase) invokeHandle() (reply centrifuge.RefreshReply, err error) {
	refreshHandler := c.refreshProxyHandler.Handle()
	reply, _, err = refreshHandler(c.client, centrifuge.RefreshEvent{}, PerCallData{})

	return reply, err
}

func newRefreshHandlerTestCases(httpTestCase httpRefreshHandleTestCase, grpcTestCase grpcRefreshHandleTestCase) []refreshHandleTestCase {
	return []refreshHandleTestCase{
		{
			refreshProxyHandler: grpcTestCase.refreshProxyHandler,
			node:                grpcTestCase.Node,
			client:              grpcTestCase.Client,
			protocol:            "grpc",
		},
		{
			refreshProxyHandler: httpTestCase.refreshProxyHandler,
			node:                httpTestCase.Node,
			client:              httpTestCase.Client,
			protocol:            "http",
		},
	}
}

func TestHandleRefreshWithCredentials(t *testing.T) {
	customData := "test"
	customDataB64 := base64.StdEncoding.EncodeToString([]byte(customData))
	expireAt := 1565436268
	opts := proxyGRPCTestServerOptions{
		B64Data:  customDataB64,
		ExpireAt: int64(expireAt),
	}

	grpcTestCase := newRefreshHandlerGRPCTestCase(context.Background(), newProxyGRPCTestServer("with credentials", opts))
	defer grpcTestCase.Teardown()

	httpTestCase := newRefreshHandlerHTTPTestCase(context.Background(), "/refresh")
	httpTestCase.Mux.HandleFunc("/refresh", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(fmt.Sprintf(`{"result": {"expire_at": %d, "b64info": "%s"}}`,
			expireAt,
			customDataB64,
		)))
	})
	defer httpTestCase.Teardown()

	cases := newRefreshHandlerTestCases(httpTestCase, grpcTestCase)
	for _, c := range cases {
		reply, err := c.invokeHandle()
		require.NoError(t, err, c.protocol)
		require.Equal(t, int64(expireAt), reply.ExpireAt, c.protocol)
		require.False(t, reply.Expired, c.protocol)
		require.Equal(t, customData, string(reply.Info), c.protocol)
	}
}

func TestHandleRefreshWithEmptyCredentials(t *testing.T) {
	opts := proxyGRPCTestServerOptions{}
	grpcTestCase := newRefreshHandlerGRPCTestCase(context.Background(), newProxyGRPCTestServer("", opts))
	defer grpcTestCase.Teardown()

	httpTestCase := newRefreshHandlerHTTPTestCase(context.Background(), "/refresh")
	httpTestCase.Mux.HandleFunc("/refresh", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	})
	defer httpTestCase.Teardown()

	cases := newRefreshHandlerTestCases(httpTestCase, grpcTestCase)
	for _, c := range cases {
		reply, err := c.invokeHandle()
		require.NoError(t, err, c.protocol)
		require.Equal(t, int64(0), reply.ExpireAt, c.protocol)
		require.True(t, reply.Expired, c.protocol)
		require.Equal(t, "", string(reply.Info), c.protocol)
	}
}

func TestHandleRefreshWithExpired(t *testing.T) {
	opts := proxyGRPCTestServerOptions{}
	grpcTestCase := newRefreshHandlerGRPCTestCase(context.Background(), newProxyGRPCTestServer("expired", opts))
	defer grpcTestCase.Teardown()

	httpTestCase := newRefreshHandlerHTTPTestCase(context.Background(), "/refresh")
	httpTestCase.Mux.HandleFunc("/refresh", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{"result": {"expired": true}}`))
	})
	defer httpTestCase.Teardown()

	cases := newRefreshHandlerTestCases(httpTestCase, grpcTestCase)
	for _, c := range cases {
		reply, err := c.invokeHandle()
		require.NoError(t, err, c.protocol)
		require.Equal(t, int64(0), reply.ExpireAt, c.protocol)
		require.True(t, reply.Expired, c.protocol)
		require.Equal(t, "", string(reply.Info), c.protocol)
	}
}

func TestHandleRefreshWithContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	grpcTestCase := newRefreshHandlerGRPCTestCase(ctx, proxyGRPCTestServer{})
	defer grpcTestCase.Teardown()

	httpTestCase := newRefreshHandlerHTTPTestCase(ctx, "/refresh")
	httpTestCase.Mux.HandleFunc("/refresh", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	})
	defer httpTestCase.Teardown()

	cases := newRefreshHandlerTestCases(httpTestCase, grpcTestCase)
	for _, c := range cases {
		reply, err := c.invokeHandle()
		require.ErrorIs(t, centrifuge.DisconnectConnectionClosed, err, c.protocol)
		require.Equal(t, centrifuge.RefreshReply{}, reply, c.protocol)
	}
}

func TestHandleRefreshWithoutProxyServerStart(t *testing.T) {
	grpcTestCase := newRefreshHandlerGRPCTestCase(context.Background(), proxyGRPCTestServer{})
	grpcTestCase.Teardown()

	httpTestCase := newRefreshHandlerHTTPTestCase(context.Background(), "/refresh")
	httpTestCase.Teardown()

	cases := newRefreshHandlerTestCases(httpTestCase, grpcTestCase)
	for _, c := range cases {
		reply, err := c.invokeHandle()
		require.NoError(t, err, c.protocol)

		expectedReply := centrifuge.RefreshReply{
			ExpireAt: time.Now().Unix() + 60,
		}
		require.Equal(t, expectedReply, reply, c.protocol)
	}
}

func TestHandleRefreshWithInvalidCustomData(t *testing.T) {
	opts := proxyGRPCTestServerOptions{
		B64Data: "invalid data",
	}
	grpcTestCase := newRefreshHandlerGRPCTestCase(context.Background(), newProxyGRPCTestServer("with credentials", opts))
	defer grpcTestCase.Teardown()

	httpTestCase := newRefreshHandlerHTTPTestCase(context.Background(), "/refresh")
	httpTestCase.Mux.HandleFunc("/refresh", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{"result": {"b64info": "invalid data"}}`))
	})
	defer httpTestCase.Teardown()

	cases := newRefreshHandlerTestCases(httpTestCase, grpcTestCase)
	for _, c := range cases {
		reply, err := c.invokeHandle()
		require.ErrorIs(t, centrifuge.ErrorInternal, err, c.protocol)
		require.Equal(t, centrifuge.RefreshReply{}, reply, c.protocol)
	}
}
