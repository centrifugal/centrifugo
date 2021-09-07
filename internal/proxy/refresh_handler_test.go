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
	"github.com/centrifugal/centrifugo/v3/internal/tools"

	"github.com/centrifugal/centrifuge"
	"github.com/stretchr/testify/require"
)

type refreshHandlerHTTPTestDepsConfig struct {
	refreshProxyHandler *RefreshHandler
	transport           *tools.TestTransport
}

func newRefreshHandlerHTTPTestDepsConfig(proxyEndpoint string) refreshHandlerHTTPTestDepsConfig {
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

	return refreshHandlerHTTPTestDepsConfig{
		refreshProxyHandler: refreshProxyHandler,
		transport:           tools.NewTestTransport(),
	}
}

type refreshHandlerGRPCTestDepsConfig struct {
	refreshProxyHandler *RefreshHandler
	transport           *tools.TestTransport
}

func newRefreshHandlerGRPCTestDepsConfig(listener *bufconn.Listener) refreshHandlerGRPCTestDepsConfig {
	proxyCfg := Config{
		RefreshTimeout: 5 * time.Second,
		GRPCConfig: GRPCConfig{
			testDialer: func(ctx context.Context, s string) (net.Conn, error) {
				return listener.Dial()
			},
		},
	}

	refreshProxy, err := NewGRPCRefreshProxy(
		listener.Addr().String(),
		proxyCfg,
	)
	if err != nil {
		log.Fatalln("could not create grpc refresh proxy: ", err)
	}

	refreshProxyHandler := NewRefreshHandler(RefreshHandlerConfig{
		Proxy: refreshProxy,
	})

	return refreshHandlerGRPCTestDepsConfig{
		refreshProxyHandler: refreshProxyHandler,
		transport:           tools.NewTestTransport(),
	}
}

type grpcRefreshHandleTestCase struct {
	cfg             refreshHandlerGRPCTestDepsConfig
	node            *centrifuge.Node
	server          *grpc.Server
	client          *centrifuge.Client
	clientCloseFunc centrifuge.ClientCloseFunc
}

func newRefreshHandleGRPCTestCase(ctx context.Context, proxyGRPCServer proxyGRPCTestServer) grpcRefreshHandleTestCase {
	node := tools.NodeWithMemoryEngineNoHandlers()

	listener := bufconn.Listen(1024)
	server := grpc.NewServer()
	proxyproto.RegisterCentrifugoProxyServer(server, proxyGRPCServer)

	go func() {
		if err := server.Serve(listener); err != nil {
			log.Fatalf("GRPC server exited with error: %v", err)
		}
	}()

	grpcTestDepsCfg := newRefreshHandlerGRPCTestDepsConfig(listener)

	client, closeFn, err := centrifuge.NewClient(ctx, node, grpcTestDepsCfg.transport)
	if err != nil {
		log.Fatalf("could not create centrifuge client: %v", err)
	}

	return grpcRefreshHandleTestCase{
		cfg:             grpcTestDepsCfg,
		node:            node,
		server:          server,
		client:          client,
		clientCloseFunc: closeFn,
	}
}

func teardownRefreshHandleGRPCTestCase(c grpcRefreshHandleTestCase) {
	defer func() { _ = c.node.Shutdown(context.Background()) }()
	defer func() { _ = c.clientCloseFunc() }()
	c.server.Stop()
}

type httpRefreshHandleTestCase struct {
	cfg             refreshHandlerHTTPTestDepsConfig
	node            *centrifuge.Node
	server          *httptest.Server
	mux             *http.ServeMux
	client          *centrifuge.Client
	clientCloseFunc centrifuge.ClientCloseFunc
}

func newRefreshHandleHTTPTestCase(ctx context.Context, endpoint string) httpRefreshHandleTestCase {
	node := tools.NodeWithMemoryEngineNoHandlers()

	mux := http.NewServeMux()
	server := httptest.NewServer(mux)

	httpTestDepsCfg := newRefreshHandlerHTTPTestDepsConfig(server.URL + endpoint)

	client, closeFn, err := centrifuge.NewClient(ctx, node, httpTestDepsCfg.transport)
	if err != nil {
		log.Fatalf("could not create centrifuge client: %v", err)
	}

	return httpRefreshHandleTestCase{
		cfg:             httpTestDepsCfg,
		node:            node,
		server:          server,
		mux:             mux,
		client:          client,
		clientCloseFunc: closeFn,
	}
}

func teardownRefreshHandleHTTPTestCase(c httpRefreshHandleTestCase) {
	defer func() { _ = c.node.Shutdown(context.Background()) }()
	defer func() { _ = c.clientCloseFunc() }()
	c.server.Close()
}

type refreshHandleTestCase struct {
	refreshProxyHandler *RefreshHandler
	protocol            string
	node                *centrifuge.Node
	client              *centrifuge.Client
}

func (c refreshHandleTestCase) invokeHandle() (reply centrifuge.RefreshReply, err error) {
	refreshHandler := c.refreshProxyHandler.Handle(c.node)
	reply, err = refreshHandler(c.client, centrifuge.RefreshEvent{})

	return reply, err
}

func newRefreshHandleTestCases(httpTestCase httpRefreshHandleTestCase, grpcTestCase grpcRefreshHandleTestCase) []refreshHandleTestCase {
	return []refreshHandleTestCase{
		{
			refreshProxyHandler: grpcTestCase.cfg.refreshProxyHandler,
			node:                grpcTestCase.node,
			client:              grpcTestCase.client,
			protocol:            "grpc",
		},
		{
			refreshProxyHandler: httpTestCase.cfg.refreshProxyHandler,
			node:                httpTestCase.node,
			client:              httpTestCase.client,
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

	grpcTestCase := newRefreshHandleGRPCTestCase(context.Background(), newProxyGRPCTestServer("with credentials", opts))
	defer teardownRefreshHandleGRPCTestCase(grpcTestCase)

	httpTestCase := newRefreshHandleHTTPTestCase(context.Background(), "/refresh")
	httpTestCase.mux.HandleFunc("/refresh", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(fmt.Sprintf(`{"result": {"expire_at": %d, "b64info": "%s"}}`,
			expireAt,
			customDataB64,
		)))
	})
	defer teardownRefreshHandleHTTPTestCase(httpTestCase)

	cases := newRefreshHandleTestCases(httpTestCase, grpcTestCase)
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
	grpcTestCase := newRefreshHandleGRPCTestCase(context.Background(), newProxyGRPCTestServer("", opts))
	defer teardownRefreshHandleGRPCTestCase(grpcTestCase)

	httpTestCase := newRefreshHandleHTTPTestCase(context.Background(), "/refresh")
	httpTestCase.mux.HandleFunc("/refresh", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	})
	defer teardownRefreshHandleHTTPTestCase(httpTestCase)

	cases := newRefreshHandleTestCases(httpTestCase, grpcTestCase)
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
	grpcTestCase := newRefreshHandleGRPCTestCase(context.Background(), newProxyGRPCTestServer("expired", opts))
	defer teardownRefreshHandleGRPCTestCase(grpcTestCase)

	httpTestCase := newRefreshHandleHTTPTestCase(context.Background(), "/refresh")
	httpTestCase.mux.HandleFunc("/refresh", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{"result": {"expired": true}}`))
	})
	defer teardownRefreshHandleHTTPTestCase(httpTestCase)

	cases := newRefreshHandleTestCases(httpTestCase, grpcTestCase)
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

	grpcTestCase := newRefreshHandleGRPCTestCase(ctx, proxyGRPCTestServer{})
	defer teardownRefreshHandleGRPCTestCase(grpcTestCase)

	httpTestCase := newRefreshHandleHTTPTestCase(ctx, "/refresh")
	httpTestCase.mux.HandleFunc("/refresh", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	})
	defer teardownRefreshHandleHTTPTestCase(httpTestCase)

	cases := newRefreshHandleTestCases(httpTestCase, grpcTestCase)
	for _, c := range cases {
		reply, err := c.invokeHandle()
		require.ErrorIs(t, centrifuge.DisconnectNormal, err, c.protocol)
		require.Equal(t, centrifuge.RefreshReply{}, reply, c.protocol)
	}
}

func TestHandleRefreshWithoutProxyServerStart(t *testing.T) {
	grpcTestCase := newRefreshHandleGRPCTestCase(context.Background(), proxyGRPCTestServer{})
	teardownRefreshHandleGRPCTestCase(grpcTestCase)

	httpTestCase := newRefreshHandleHTTPTestCase(context.Background(), "/refresh")
	teardownRefreshHandleHTTPTestCase(httpTestCase)

	expectedReply := centrifuge.RefreshReply{
		ExpireAt: time.Now().Unix() + 60,
	}

	cases := newRefreshHandleTestCases(httpTestCase, grpcTestCase)
	for _, c := range cases {
		reply, err := c.invokeHandle()
		require.NoError(t, err, c.protocol)
		require.Equal(t, expectedReply, reply, c.protocol)
	}
}

func TestHandleRefreshWithInvalidCustomData(t *testing.T) {
	opts := proxyGRPCTestServerOptions{
		B64Data: "invalid data",
	}
	grpcTestCase := newRefreshHandleGRPCTestCase(context.Background(), newProxyGRPCTestServer("with credentials", opts))
	defer teardownRefreshHandleGRPCTestCase(grpcTestCase)

	httpTestCase := newRefreshHandleHTTPTestCase(context.Background(), "/refresh")
	httpTestCase.mux.HandleFunc("/refresh", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{"result": {"b64info": "invalid data"}}`))
	})
	defer teardownRefreshHandleHTTPTestCase(httpTestCase)

	cases := newRefreshHandleTestCases(httpTestCase, grpcTestCase)
	for _, c := range cases {
		reply, err := c.invokeHandle()
		require.ErrorIs(t, centrifuge.ErrorInternal, err, c.protocol)
		require.Equal(t, centrifuge.RefreshReply{}, reply, c.protocol)
	}
}
