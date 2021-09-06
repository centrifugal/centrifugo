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

	"github.com/centrifugal/centrifuge"
	"github.com/stretchr/testify/require"
)

type publishHandlerHTTPTestDepsConfig struct {
	publishProxyHandler *PublishHandler
	transport           *tools.TestTransport
}

func newPublishHandlerHTTPTestDepsConfig(proxyEndpoint string) publishHandlerHTTPTestDepsConfig {
	proxyCfg := Config{
		HTTPConfig: HTTPConfig{
			Encoder: &proxyproto.JSONEncoder{},
			Decoder: &proxyproto.JSONDecoder{},
		},
		PublishEndpoint: proxyEndpoint,
	}

	publishProxy, err := NewHTTPPublishProxy(
		proxyEndpoint,
		proxyCfg,
	)
	if err != nil {
		log.Fatalln("could not create http publish proxy: ", err)
	}

	publishProxyHandler := NewPublishHandler(PublishHandlerConfig{
		Proxy: publishProxy,
	})

	return publishHandlerHTTPTestDepsConfig{
		publishProxyHandler: publishProxyHandler,
		transport:           tools.NewTestTransport(),
	}
}

type publishHandlerGRPCTestDepsConfig struct {
	publishProxyHandler *PublishHandler
	transport           *tools.TestTransport
}

func newPublishHandlerGRPCTestDepsConfig(listener *bufconn.Listener) publishHandlerGRPCTestDepsConfig {
	proxyCfg := Config{
		PublishTimeout: 5 * time.Second,
		GRPCConfig: GRPCConfig{
			testDialer: func(ctx context.Context, s string) (net.Conn, error) {
				return listener.Dial()
			},
		},
	}

	publishProxy, err := NewGRPCPublishProxy(
		listener.Addr().String(),
		proxyCfg,
	)
	if err != nil {
		log.Fatalln("could not create grpc publish proxy: ", err)
	}

	publishProxyHandler := NewPublishHandler(PublishHandlerConfig{
		Proxy: publishProxy,
	})

	return publishHandlerGRPCTestDepsConfig{
		publishProxyHandler: publishProxyHandler,
		transport:           tools.NewTestTransport(),
	}
}

type grpcPublishHandleTestCase struct {
	cfg             publishHandlerGRPCTestDepsConfig
	node            *centrifuge.Node
	server          *grpc.Server
	ctx             context.Context
	client          *centrifuge.Client
	clientCloseFunc centrifuge.ClientCloseFunc
	channelOpts     rule.ChannelOptions
}

func newPublishHandleGRPCTestCase(ctx context.Context, proxyGRPCServer proxyGRPCTestServer, opts rule.ChannelOptions) grpcPublishHandleTestCase {
	node := tools.NodeWithMemoryEngineNoHandlers()

	listener := bufconn.Listen(1024)
	server := grpc.NewServer()
	proxyproto.RegisterCentrifugoProxyServer(server, proxyGRPCServer)

	go func() {
		if err := server.Serve(listener); err != nil {
			log.Fatalf("GRPC server exited with error: %v", err)
		}
	}()

	grpcTestDepsCfg := newPublishHandlerGRPCTestDepsConfig(listener)

	client, closeFn, err := centrifuge.NewClient(ctx, node, grpcTestDepsCfg.transport)
	if err != nil {
		log.Fatalf("could not create centrifuge client: %v", err)
	}

	return grpcPublishHandleTestCase{
		cfg:             grpcTestDepsCfg,
		node:            node,
		server:          server,
		client:          client,
		clientCloseFunc: closeFn,
		channelOpts:     opts,
	}
}

func teardownPublishHandleGRPCTestCase(c grpcPublishHandleTestCase) {
	defer func() { _ = c.node.Shutdown(context.Background()) }()
	defer func() { _ = c.clientCloseFunc() }()
	c.server.Stop()
}

type httpPublishHandleTestCase struct {
	cfg             publishHandlerHTTPTestDepsConfig
	node            *centrifuge.Node
	server          *httptest.Server
	mux             *http.ServeMux
	ctx             context.Context
	client          *centrifuge.Client
	clientCloseFunc centrifuge.ClientCloseFunc
	channelOpts     rule.ChannelOptions
}

func newPublishHandleHTTPTestCase(ctx context.Context, endpoint string, opts rule.ChannelOptions) httpPublishHandleTestCase {
	node := tools.NodeWithMemoryEngineNoHandlers()

	mux := http.NewServeMux()
	server := httptest.NewServer(mux)

	httpTestDepsCfg := newPublishHandlerHTTPTestDepsConfig(server.URL + endpoint)

	client, closeFn, err := centrifuge.NewClient(ctx, node, httpTestDepsCfg.transport)
	if err != nil {
		log.Fatalf("could not create centrifuge client: %v", err)
	}

	return httpPublishHandleTestCase{
		cfg:             httpTestDepsCfg,
		node:            node,
		server:          server,
		mux:             mux,
		client:          client,
		clientCloseFunc: closeFn,
		channelOpts:     opts,
	}
}

func teardownPublishHandleHTTPTestCase(c httpPublishHandleTestCase) {
	defer func() { _ = c.node.Shutdown(context.Background()) }()
	defer func() { _ = c.clientCloseFunc() }()
	c.server.Close()
}

type publishHandleTestCase struct {
	publishProxyHandler *PublishHandler
	protocol            string
	node                *centrifuge.Node
	channelOpts         rule.ChannelOptions
	client              *centrifuge.Client
}

func (c publishHandleTestCase) invokeHandle() (reply centrifuge.PublishReply, err error) {
	publishHandler := c.publishProxyHandler.Handle(c.node)
	reply, err = publishHandler(c.client, centrifuge.PublishEvent{}, c.channelOpts)

	return reply, err
}

func newPublishHandleTestCases(httpTestCase httpPublishHandleTestCase, grpcTestCase grpcPublishHandleTestCase) []publishHandleTestCase {
	return []publishHandleTestCase{
		{
			publishProxyHandler: grpcTestCase.cfg.publishProxyHandler,
			node:                grpcTestCase.node,
			client:              grpcTestCase.client,
			channelOpts:         grpcTestCase.channelOpts,
			protocol:            "grpc",
		},
		{
			publishProxyHandler: httpTestCase.cfg.publishProxyHandler,
			node:                httpTestCase.node,
			client:              httpTestCase.client,
			channelOpts:         httpTestCase.channelOpts,
			protocol:            "http",
		},
	}
}

func TestHandlePublishWithResult(t *testing.T) {
	customData := "test"
	customDataB64 := base64.StdEncoding.EncodeToString([]byte(customData))
	chOpts := rule.ChannelOptions{
		HistoryTTL:  tools.Duration(1 * time.Second),
		HistorySize: 1,
	}

	opts := proxyGRPCTestServerOptions{
		B64Data: customDataB64,
	}
	grpcTestCase := newPublishHandleGRPCTestCase(context.Background(), newProxyGRPCTestServer("result", opts), chOpts)
	defer teardownPublishHandleGRPCTestCase(grpcTestCase)

	httpTestCase := newPublishHandleHTTPTestCase(context.Background(), "/publish", chOpts)
	httpTestCase.mux.HandleFunc("/publish", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(fmt.Sprintf(`{"result": {"b64data": "%s"}}`, customDataB64)))
	})
	defer teardownPublishHandleHTTPTestCase(httpTestCase)

	cases := newPublishHandleTestCases(httpTestCase, grpcTestCase)
	for _, c := range cases {
		reply, err := c.invokeHandle()
		require.NoError(t, err, c.protocol)
		require.Equal(t, uint64(1), reply.Result.Offset, c.protocol)
		require.NotEmpty(t, reply.Result.Epoch, c.protocol)
	}
}

func TestHandlePublishWithSkipHistory(t *testing.T) {
	customData := "test"
	customDataB64 := base64.StdEncoding.EncodeToString([]byte(customData))
	chOpts := rule.ChannelOptions{
		HistoryTTL:  tools.Duration(1 * time.Second),
		HistorySize: 1,
	}

	opts := proxyGRPCTestServerOptions{
		B64Data: customDataB64,
	}
	grpcTestCase := newPublishHandleGRPCTestCase(context.Background(), newProxyGRPCTestServer("skip history", opts), chOpts)
	defer teardownPublishHandleGRPCTestCase(grpcTestCase)

	httpTestCase := newPublishHandleHTTPTestCase(context.Background(), "/publish", chOpts)
	httpTestCase.mux.HandleFunc("/publish", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(fmt.Sprintf(`{"result": {"b64data": "%s", "skip_history": true}}`, customDataB64)))
	})
	defer teardownPublishHandleHTTPTestCase(httpTestCase)

	cases := newPublishHandleTestCases(httpTestCase, grpcTestCase)
	for _, c := range cases {
		reply, err := c.invokeHandle()
		require.NoError(t, err, c.protocol)
		require.Equal(t, uint64(0), reply.Result.Offset, c.protocol)
		require.Empty(t, reply.Result.Epoch, c.protocol)
	}
}

func TestHandlePublishWithContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	grpcTestCase := newPublishHandleGRPCTestCase(ctx, proxyGRPCTestServer{}, rule.ChannelOptions{})
	defer teardownPublishHandleGRPCTestCase(grpcTestCase)

	httpTestCase := newPublishHandleHTTPTestCase(ctx, "/publish", rule.ChannelOptions{})
	httpTestCase.mux.HandleFunc("/publish", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	})
	defer teardownPublishHandleHTTPTestCase(httpTestCase)

	cases := newPublishHandleTestCases(httpTestCase, grpcTestCase)
	for _, c := range cases {
		reply, err := c.invokeHandle()
		require.NoError(t, err, c.protocol)
		require.Equal(t, centrifuge.PublishReply{}, reply, c.protocol)
	}
}

func TestHandlePublishWithoutProxyServerStart(t *testing.T) {
	grpcTestCase := newPublishHandleGRPCTestCase(context.Background(), proxyGRPCTestServer{}, rule.ChannelOptions{})
	teardownPublishHandleGRPCTestCase(grpcTestCase)

	httpTestCase := newPublishHandleHTTPTestCase(context.Background(), "/publish", rule.ChannelOptions{})
	teardownPublishHandleHTTPTestCase(httpTestCase)

	cases := newPublishHandleTestCases(httpTestCase, grpcTestCase)
	for _, c := range cases {
		reply, err := c.invokeHandle()
		require.ErrorIs(t, centrifuge.ErrorInternal, err, c.protocol)
		require.Equal(t, centrifuge.PublishReply{}, reply, c.protocol)
	}
}

func TestHandlePublishWithProxyServerCustomDisconnect(t *testing.T) {
	chOpts := rule.ChannelOptions{}
	grpcTestCase := newPublishHandleGRPCTestCase(context.Background(), newProxyGRPCTestServer("custom disconnect", proxyGRPCTestServerOptions{}), chOpts)
	defer teardownPublishHandleGRPCTestCase(grpcTestCase)

	httpTestCase := newPublishHandleHTTPTestCase(context.Background(), "/publish", chOpts)
	httpTestCase.mux.HandleFunc("/publish", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{"disconnect": {"code": 4000, "reconnect": false, "reason": "custom disconnect"}}`))
	})
	defer teardownPublishHandleHTTPTestCase(httpTestCase)

	expectedErr := centrifuge.Disconnect{
		Code:      4000,
		Reason:    "custom disconnect",
		Reconnect: false,
	}

	cases := newPublishHandleTestCases(httpTestCase, grpcTestCase)
	for _, c := range cases {
		reply, err := c.invokeHandle()
		require.NotNil(t, err, c.protocol)
		require.Equal(t, expectedErr.Error(), err.Error(), c.protocol)
		require.Equal(t, centrifuge.PublishReply{}, reply, c.protocol)
	}
}

func TestHandlePublishWithProxyServerCustomError(t *testing.T) {
	chOpts := rule.ChannelOptions{}
	grpcTestCase := newPublishHandleGRPCTestCase(context.Background(), newProxyGRPCTestServer("custom error", proxyGRPCTestServerOptions{}), chOpts)
	defer teardownPublishHandleGRPCTestCase(grpcTestCase)

	httpTestCase := newPublishHandleHTTPTestCase(context.Background(), "/publish", chOpts)
	httpTestCase.mux.HandleFunc("/publish", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{"error": {"code": 1000, "message": "custom error"}}`))
	})
	defer teardownPublishHandleHTTPTestCase(httpTestCase)

	expectedErr := centrifuge.Error{
		Code:    1000,
		Message: "custom error",
	}

	cases := newPublishHandleTestCases(httpTestCase, grpcTestCase)
	for _, c := range cases {
		reply, err := c.invokeHandle()
		require.NotNil(t, err, c.protocol)
		require.Equal(t, expectedErr.Error(), err.Error(), c.protocol)
		require.Equal(t, centrifuge.PublishReply{}, reply, c.protocol)
	}
}

func TestHandlePublishWithInvalidCustomData(t *testing.T) {
	chOpts := rule.ChannelOptions{}
	opts := proxyGRPCTestServerOptions{
		B64Data: "invalid data",
	}
	grpcTestCase := newPublishHandleGRPCTestCase(context.Background(), newProxyGRPCTestServer("result", opts), chOpts)
	defer teardownPublishHandleGRPCTestCase(grpcTestCase)

	httpTestCase := newPublishHandleHTTPTestCase(context.Background(), "/publish", chOpts)
	httpTestCase.mux.HandleFunc("/publish", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{"result": {"b64data": "invalid data"}}`))
	})
	defer teardownPublishHandleHTTPTestCase(httpTestCase)

	cases := newPublishHandleTestCases(httpTestCase, grpcTestCase)
	for _, c := range cases {
		reply, err := c.invokeHandle()
		require.ErrorIs(t, centrifuge.ErrorInternal, err, c.protocol)
		require.Equal(t, centrifuge.PublishReply{}, reply, c.protocol)
	}
}
