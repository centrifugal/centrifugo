package proxy

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/clientcontext"
	"github.com/centrifugal/centrifugo/v6/internal/configtypes"
	"github.com/centrifugal/centrifugo/v6/internal/middleware"
	"github.com/centrifugal/centrifugo/v6/internal/subsource"
	"github.com/centrifugal/centrifugo/v6/internal/tools"

	"github.com/centrifugal/centrifuge"
	"github.com/stretchr/testify/require"
)

type grpcSubscribeHandleTestCase struct {
	*tools.CommonGRPCProxyTestCase
	subscribeProxyHandler *SubscribeHandler
	channelOpts           configtypes.ChannelOptions
}

func newSubscribeHandleGRPCTestCase(ctx context.Context, proxyGRPCServer proxyGRPCTestServer, opts configtypes.ChannelOptions) grpcSubscribeHandleTestCase {
	commonProxyTestCase := tools.NewCommonGRPCProxyTestCase(ctx, proxyGRPCServer)

	subscribeProxy, err := NewGRPCSubscribeProxy("default", getTestGrpcProxy(commonProxyTestCase))
	if err != nil {
		log.Fatalln("could not create grpc subscribe proxy: ", err)
	}

	subscribeProxyHandler := NewSubscribeHandler(SubscribeHandlerConfig{
		Proxies: map[string]SubscribeProxy{
			"test": subscribeProxy,
		},
	})

	return grpcSubscribeHandleTestCase{commonProxyTestCase, subscribeProxyHandler, opts}
}

type httpSubscribeHandleTestCase struct {
	*tools.CommonHTTPProxyTestCase
	subscribeProxyHandler *SubscribeHandler
	channelOpts           configtypes.ChannelOptions
}

func newSubscribeHandleHTTPTestCase(ctx context.Context, endpoint string, opts configtypes.ChannelOptions) httpSubscribeHandleTestCase {
	commonProxyTestCase := tools.NewCommonHTTPProxyTestCase(ctx)

	subscribeProxy, err := NewHTTPSubscribeProxy(getTestHttpProxy(commonProxyTestCase, endpoint))
	if err != nil {
		log.Fatalln("could not create http subscribe proxy: ", err)
	}

	subscribeProxyHandler := NewSubscribeHandler(SubscribeHandlerConfig{
		Proxies: map[string]SubscribeProxy{
			"test": subscribeProxy,
		},
	})

	return httpSubscribeHandleTestCase{commonProxyTestCase, subscribeProxyHandler, opts}
}

type subscribeHandleTestCase struct {
	subscribeProxyHandler *SubscribeHandler
	protocol              string
	node                  *centrifuge.Node
	client                *centrifuge.Client
	channelOpts           configtypes.ChannelOptions
}

func (c subscribeHandleTestCase) invokeHandle() (reply centrifuge.SubscribeReply, err error) {
	subscribeHandler := c.subscribeProxyHandler.Handle()
	reply, _, err = subscribeHandler(c.client, centrifuge.SubscribeEvent{}, c.channelOpts, PerCallData{})

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

// TestHandleSubscribeForwardsHeaders is an end-to-end test that the split header
// model works for a channel (subscribe) proxy, not just connect: the emulated and
// real transport headers captured at connect (here placed in the client context,
// exactly as OnClientConnecting sets them) are forwarded to the subscribe backend
// filtered by the subscribe proxy's own http_headers / emulated_headers lists.
func TestHandleSubscribeForwardsHeaders(t *testing.T) {
	// Client context as it would look after connect: a real transport header, plus
	// an emulated map that includes a legit token and a forged value for a name that
	// is only in http_headers (must not win via emulation).
	ctx := middleware.SetHeadersToContext(context.Background(), http.Header{"X-Real-Ip": []string{"1.2.3.4"}})
	ctx = clientcontext.SetEmulatedHeadersToContext(ctx, map[string]string{
		"x-app-token": "from-client",
		"x-real-ip":   "forged-by-client",
	})

	commonProxyTestCase := tools.NewCommonHTTPProxyTestCase(ctx)
	defer commonProxyTestCase.Teardown()

	var received http.Header
	commonProxyTestCase.Mux.HandleFunc("/subscribe", func(w http.ResponseWriter, req *http.Request) {
		received = req.Header.Clone()
		_, _ = w.Write([]byte(`{"result": {}}`))
	})

	proxyCfg := configtypes.Proxy{
		Endpoint: commonProxyTestCase.Server.URL + "/subscribe",
		Timeout:  configtypes.Duration(5 * time.Second),
		ProxyCommon: configtypes.ProxyCommon{
			HttpHeaders:     []string{"X-Real-Ip"},   // transport-only
			EmulatedHeaders: []string{"X-App-Token"}, // client-supplied
		},
	}
	subscribeProxy, err := GetSubscribeProxy("test", proxyCfg)
	require.NoError(t, err)

	handler := NewSubscribeHandler(SubscribeHandlerConfig{
		Proxies: map[string]SubscribeProxy{"test": subscribeProxy},
	}).Handle()

	chOpts := configtypes.ChannelOptions{
		SubscribeProxyEnabled: true,
		SubscribeProxyName:    "test",
	}
	_, _, err = handler(commonProxyTestCase.Client, centrifuge.SubscribeEvent{}, chOpts, PerCallData{})
	require.NoError(t, err)

	require.NotNil(t, received, "subscribe backend should have been called")
	require.Equal(t, "from-client", received.Get("X-App-Token"), "emulated header must be forwarded via the subscribe proxy's emulated_headers")
	require.Equal(t, "1.2.3.4", received.Get("X-Real-Ip"), "real transport value must win; forged emulated value must NOT be forwarded via http_headers")
}

func TestHandleSubscribeWithResult(t *testing.T) {
	customData := "test"
	customDataB64 := base64.StdEncoding.EncodeToString([]byte(customData))
	chOpts := configtypes.ChannelOptions{
		Presence:              true,
		JoinLeave:             true,
		ForcePushJoinLeave:    true,
		ForceRecovery:         true,
		ForcePositioning:      true,
		SubscribeProxyEnabled: true,
		SubscribeProxyName:    "test",
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
		ChannelInfo:       []byte(customData),
		Data:              []byte(customData),
		EmitPresence:      true,
		EmitJoinLeave:     true,
		PushJoinLeave:     true,
		EnableRecovery:    true,
		EnablePositioning: true,
		Source:            subsource.SubscribeProxy,
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
	chOpts := configtypes.ChannelOptions{
		Presence:              false,
		JoinLeave:             true,
		ForceRecovery:         false,
		ForcePositioning:      false,
		SubscribeProxyEnabled: true,
		SubscribeProxyName:    "test",
	}
	opts := proxyGRPCTestServerOptions{
		B64Data: customDataB64,
	}

	grpcTestCase := newSubscribeHandleGRPCTestCase(context.Background(), newProxyGRPCTestServer("override", opts), chOpts)
	defer grpcTestCase.Teardown()

	httpTestCase := newSubscribeHandleHTTPTestCase(context.Background(), "/subscribe", chOpts)
	httpTestCase.Mux.HandleFunc("/subscribe", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(fmt.Sprintf(`{"result": {"b64info": "%s", "override": {"join_leave": {"value": false}, "presence": {"value": true}, "force_positioning": {"value": true}, "force_recovery": {"value": true}}}}`, customDataB64)))
	})
	defer httpTestCase.Teardown()

	expectedSubscribeOpts := centrifuge.SubscribeOptions{
		ChannelInfo:       []byte(customData),
		EmitPresence:      true,
		EmitJoinLeave:     false,
		PushJoinLeave:     false,
		EnablePositioning: true,
		EnableRecovery:    true,
		Source:            subsource.SubscribeProxy,
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

	grpcTestCase := newSubscribeHandleGRPCTestCase(ctx, proxyGRPCTestServer{}, configtypes.ChannelOptions{
		SubscribeProxyEnabled: true,
		SubscribeProxyName:    "test",
	})
	defer grpcTestCase.Teardown()

	httpTestCase := newSubscribeHandleHTTPTestCase(ctx, "/subscribe", configtypes.ChannelOptions{
		SubscribeProxyEnabled: true,
		SubscribeProxyName:    "test",
	})
	httpTestCase.Mux.HandleFunc("/subscribe", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	})
	defer httpTestCase.Teardown()

	cases := newSubscribeHandleTestCases(httpTestCase, grpcTestCase)
	for _, c := range cases {
		reply, err := c.invokeHandle()
		require.ErrorIs(t, centrifuge.DisconnectConnectionClosed, err, c.protocol)
		require.Equal(t, centrifuge.SubscribeReply{}, reply, c.protocol)
	}
}

func TestHandleSubscribeWithoutProxyServerStart(t *testing.T) {
	grpcTestCase := newSubscribeHandleGRPCTestCase(context.Background(), proxyGRPCTestServer{}, configtypes.ChannelOptions{
		SubscribeProxyEnabled: true,
		SubscribeProxyName:    "test",
	})
	grpcTestCase.Teardown()

	httpTestCase := newSubscribeHandleHTTPTestCase(context.Background(), "/subscribe", configtypes.ChannelOptions{
		SubscribeProxyEnabled: true,
		SubscribeProxyName:    "test",
	})
	httpTestCase.Teardown()

	cases := newSubscribeHandleTestCases(httpTestCase, grpcTestCase)
	for _, c := range cases {
		reply, err := c.invokeHandle()
		require.Error(t, err, c.protocol)
		require.Equal(t, centrifuge.SubscribeReply{}, reply, c.protocol)
	}
}

func TestHandleSubscribeWithProxyServerCustomDisconnect(t *testing.T) {
	chOpts := configtypes.ChannelOptions{
		SubscribeProxyEnabled: true,
		SubscribeProxyName:    "test",
	}
	grpcTestCase := newSubscribeHandleGRPCTestCase(context.Background(), newProxyGRPCTestServer("custom disconnect", proxyGRPCTestServerOptions{}), chOpts)
	defer grpcTestCase.Teardown()

	httpTestCase := newSubscribeHandleHTTPTestCase(context.Background(), "/subscribe", chOpts)
	httpTestCase.Mux.HandleFunc("/subscribe", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{"disconnect": {"code": 4000, "reconnect": false, "reason": "custom disconnect"}}`))
	})
	defer httpTestCase.Teardown()

	expectedErr := centrifuge.Disconnect{
		Code:   4000,
		Reason: "custom disconnect",
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
	chOpts := configtypes.ChannelOptions{
		SubscribeProxyEnabled: true,
		SubscribeProxyName:    "test",
	}
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
	chOpts := configtypes.ChannelOptions{
		SubscribeProxyEnabled: true,
		SubscribeProxyName:    "test",
	}
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
		require.ErrorIs(t, err, centrifuge.ErrorInternal, c.protocol)
		require.Equal(t, centrifuge.SubscribeReply{}, reply, c.protocol)
	}
}
