package proxy

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"testing"

	"github.com/centrifugal/centrifugo/v3/internal/rule"
	"github.com/centrifugal/centrifugo/v3/internal/tools"

	"github.com/centrifugal/centrifuge"
	"github.com/stretchr/testify/require"
)

type grpcSubscribeHandleTestCase struct {
	*tools.CommonGRPCProxyTestCase
	subscribeProxyHandler *SubscribeHandler
	channelOpts           rule.ChannelOptions
}

func newSubscribeHandleGRPCTestCase(ctx context.Context, proxyGRPCServer proxyGRPCTestServer, opts rule.ChannelOptions) grpcSubscribeHandleTestCase {
	commonProxyTestCase := tools.NewCommonGRPCProxyTestCase(ctx, proxyGRPCServer)

	subscribeProxy, err := NewGRPCSubscribeProxy(getTestGrpcProxy(commonProxyTestCase))
	if err != nil {
		log.Fatalln("could not create grpc subscribe proxy: ", err)
	}

	subscribeProxyHandler := NewSubscribeHandler(SubscribeHandlerConfig{
		Proxies: map[string]SubscribeProxy{
			"": subscribeProxy,
		},
	})

	return grpcSubscribeHandleTestCase{commonProxyTestCase, subscribeProxyHandler, opts}
}

type httpSubscribeHandleTestCase struct {
	*tools.CommonHTTPProxyTestCase
	subscribeProxyHandler *SubscribeHandler
	channelOpts           rule.ChannelOptions
}

func newSubscribeHandleHTTPTestCase(ctx context.Context, endpoint string, opts rule.ChannelOptions) httpSubscribeHandleTestCase {
	commonProxyTestCase := tools.NewCommonHTTPProxyTestCase(ctx)

	subscribeProxy, err := NewHTTPSubscribeProxy(getTestHttpProxy(commonProxyTestCase, endpoint))
	if err != nil {
		log.Fatalln("could not create http subscribe proxy: ", err)
	}

	subscribeProxyHandler := NewSubscribeHandler(SubscribeHandlerConfig{
		Proxies: map[string]SubscribeProxy{
			"": subscribeProxy,
		},
	})

	return httpSubscribeHandleTestCase{commonProxyTestCase, subscribeProxyHandler, opts}
}

type subscribeHandleTestCase struct {
	subscribeProxyHandler *SubscribeHandler
	protocol              string
	node                  *centrifuge.Node
	client                *centrifuge.Client
	channelOpts           rule.ChannelOptions
}

func (c subscribeHandleTestCase) invokeHandle() (reply centrifuge.SubscribeReply, err error) {
	subscribeHandler := c.subscribeProxyHandler.Handle(c.node)
	reply, err = subscribeHandler(c.client, centrifuge.SubscribeEvent{}, c.channelOpts)

	return reply, err
}

func newSubscribeHandleTestCases(httpTestCase httpSubscribeHandleTestCase, grpcTestCase grpcSubscribeHandleTestCase) []subscribeHandleTestCase {
	return []subscribeHandleTestCase{
		{
			subscribeProxyHandler: grpcTestCase.subscribeProxyHandler,
			node:                  grpcTestCase.Node,
			client:                grpcTestCase.Client,
			channelOpts:           grpcTestCase.channelOpts,
			protocol:              "grpc",
		},
		{
			subscribeProxyHandler: httpTestCase.subscribeProxyHandler,
			node:                  httpTestCase.Node,
			client:                httpTestCase.Client,
			channelOpts:           httpTestCase.channelOpts,
			protocol:              "http",
		},
	}
}

func TestHandleSubscribeWithResult(t *testing.T) {
	customData := "test"
	customDataB64 := base64.StdEncoding.EncodeToString([]byte(customData))
	chOpts := rule.ChannelOptions{
		Presence:  true,
		JoinLeave: true,
		Recover:   true,
		Position:  true,
	}
	opts := proxyGRPCTestServerOptions{
		B64Data: customDataB64,
	}

	grpcTestCase := newSubscribeHandleGRPCTestCase(context.Background(), newProxyGRPCTestServer("result", opts), chOpts)
	defer grpcTestCase.Teardown()

	httpTestCase := newSubscribeHandleHTTPTestCase(context.Background(), "/subscribe", chOpts)
	httpTestCase.Mux.HandleFunc("/subscribe", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(fmt.Sprintf(`{"result": {"b64info": "%s", "b64data": "%s"}}`, customDataB64, customDataB64)))
	})
	defer httpTestCase.Teardown()

	expectedSubscribeOpts := centrifuge.SubscribeOptions{
		ChannelInfo: []byte(customData),
		Data:        []byte(customData),
		Presence:    true,
		JoinLeave:   true,
		Recover:     true,
		Position:    true,
	}

	cases := newSubscribeHandleTestCases(httpTestCase, grpcTestCase)
	for _, c := range cases {
		reply, err := c.invokeHandle()
		require.NoError(t, err, c.protocol)
		require.Equal(t, expectedSubscribeOpts, reply.Options, c.protocol)
		require.True(t, reply.ClientSideRefresh, c.protocol)
	}
}

func TestHandleSubscribeWithOverride(t *testing.T) {
	customData := "test"
	customDataB64 := base64.StdEncoding.EncodeToString([]byte(customData))
	chOpts := rule.ChannelOptions{
		Presence:  false,
		JoinLeave: true,
		Recover:   false,
		Position:  false,
	}
	opts := proxyGRPCTestServerOptions{
		B64Data: customDataB64,
	}

	grpcTestCase := newSubscribeHandleGRPCTestCase(context.Background(), newProxyGRPCTestServer("override", opts), chOpts)
	defer grpcTestCase.Teardown()

	httpTestCase := newSubscribeHandleHTTPTestCase(context.Background(), "/subscribe", chOpts)
	httpTestCase.Mux.HandleFunc("/subscribe", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(fmt.Sprintf(`{"result": {"b64info": "%s", "override": {"join_leave": {"value": false}, "presence": {"value": true}, "position": {"value": true}, "recover": {"value": true}}}}`, customDataB64)))
	})
	defer httpTestCase.Teardown()

	expectedSubscribeOpts := centrifuge.SubscribeOptions{
		ChannelInfo: []byte(customData),
		Presence:    true,
		JoinLeave:   false,
		Position:    true,
		Recover:     true,
	}

	cases := newSubscribeHandleTestCases(httpTestCase, grpcTestCase)
	for _, c := range cases {
		reply, err := c.invokeHandle()
		require.NoError(t, err, c.protocol)
		require.Equal(t, expectedSubscribeOpts, reply.Options, c.protocol)
		require.True(t, reply.ClientSideRefresh, c.protocol)
	}
}

func TestHandleSubscribeWithContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	grpcTestCase := newSubscribeHandleGRPCTestCase(ctx, proxyGRPCTestServer{}, rule.ChannelOptions{})
	defer grpcTestCase.Teardown()

	httpTestCase := newSubscribeHandleHTTPTestCase(ctx, "/subscribe", rule.ChannelOptions{})
	httpTestCase.Mux.HandleFunc("/subscribe", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	})
	defer httpTestCase.Teardown()

	cases := newSubscribeHandleTestCases(httpTestCase, grpcTestCase)
	for _, c := range cases {
		reply, err := c.invokeHandle()
		require.ErrorIs(t, centrifuge.DisconnectNormal, err, c.protocol)
		require.Equal(t, centrifuge.SubscribeReply{}, reply, c.protocol)
	}
}

func TestHandleSubscribeWithoutProxyServerStart(t *testing.T) {
	grpcTestCase := newSubscribeHandleGRPCTestCase(context.Background(), proxyGRPCTestServer{}, rule.ChannelOptions{})
	grpcTestCase.Teardown()

	httpTestCase := newSubscribeHandleHTTPTestCase(context.Background(), "/subscribe", rule.ChannelOptions{})
	httpTestCase.Teardown()

	cases := newSubscribeHandleTestCases(httpTestCase, grpcTestCase)
	for _, c := range cases {
		reply, err := c.invokeHandle()
		require.ErrorIs(t, centrifuge.ErrorInternal, err, c.protocol)
		require.Equal(t, centrifuge.SubscribeReply{}, reply, c.protocol)
	}
}

func TestHandleSubscribeWithProxyServerCustomDisconnect(t *testing.T) {
	chOpts := rule.ChannelOptions{}
	grpcTestCase := newSubscribeHandleGRPCTestCase(context.Background(), newProxyGRPCTestServer("custom disconnect", proxyGRPCTestServerOptions{}), chOpts)
	defer grpcTestCase.Teardown()

	httpTestCase := newSubscribeHandleHTTPTestCase(context.Background(), "/subscribe", chOpts)
	httpTestCase.Mux.HandleFunc("/subscribe", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{"disconnect": {"code": 4000, "reconnect": false, "reason": "custom disconnect"}}`))
	})
	defer httpTestCase.Teardown()

	expectedErr := centrifuge.Disconnect{
		Code:      4000,
		Reason:    "custom disconnect",
		Reconnect: false,
	}

	cases := newSubscribeHandleTestCases(httpTestCase, grpcTestCase)
	for _, c := range cases {
		reply, err := c.invokeHandle()
		require.NotNil(t, err, c.protocol)
		require.Equal(t, expectedErr.Error(), err.Error(), c.protocol)
		require.Equal(t, centrifuge.SubscribeReply{}, reply, c.protocol)
	}
}

func TestHandleSubscribeWithProxyServerCustomError(t *testing.T) {
	chOpts := rule.ChannelOptions{}
	grpcTestCase := newSubscribeHandleGRPCTestCase(context.Background(), newProxyGRPCTestServer("custom error", proxyGRPCTestServerOptions{}), chOpts)
	defer grpcTestCase.Teardown()

	httpTestCase := newSubscribeHandleHTTPTestCase(context.Background(), "/subscribe", chOpts)
	httpTestCase.Mux.HandleFunc("/subscribe", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{"error": {"code": 1000, "message": "custom error"}}`))
	})
	defer httpTestCase.Teardown()

	expectedErr := centrifuge.Error{
		Code:    1000,
		Message: "custom error",
	}

	cases := newSubscribeHandleTestCases(httpTestCase, grpcTestCase)
	for _, c := range cases {
		reply, err := c.invokeHandle()
		require.NotNil(t, err, c.protocol)
		require.Equal(t, expectedErr.Error(), err.Error(), c.protocol)
		require.Equal(t, centrifuge.SubscribeReply{}, reply, c.protocol)
	}
}

func TestHandleSubscribeWithInvalidCustomData(t *testing.T) {
	chOpts := rule.ChannelOptions{}
	opts := proxyGRPCTestServerOptions{
		B64Data: "invalid data",
	}
	grpcTestCase := newSubscribeHandleGRPCTestCase(context.Background(), newProxyGRPCTestServer("result", opts), chOpts)
	defer grpcTestCase.Teardown()

	httpTestCase := newSubscribeHandleHTTPTestCase(context.Background(), "/subscribe", chOpts)
	httpTestCase.Mux.HandleFunc("/subscribe", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{"result": {"b64info": "invalid data"}}`))
	})
	defer httpTestCase.Teardown()

	cases := newSubscribeHandleTestCases(httpTestCase, grpcTestCase)
	for _, c := range cases {
		reply, err := c.invokeHandle()
		require.ErrorIs(t, centrifuge.ErrorInternal, err, c.protocol)
		require.Equal(t, centrifuge.SubscribeReply{}, reply, c.protocol)
	}
}
