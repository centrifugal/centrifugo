package proxy

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/configtypes"
	"github.com/centrifugal/centrifugo/v6/internal/tools"

	"github.com/centrifugal/centrifuge"
	"github.com/stretchr/testify/require"
)

type grpcPublishHandleTestCase struct {
	*tools.CommonGRPCProxyTestCase
	publishProxyHandler *PublishHandler
	channelOpts         configtypes.ChannelOptions
}

func newPublishHandleGRPCTestCase(ctx context.Context, proxyGRPCServer proxyGRPCTestServer, opts configtypes.ChannelOptions) grpcPublishHandleTestCase {
	commonProxyTestCase := tools.NewCommonGRPCProxyTestCase(ctx, proxyGRPCServer)

	publishProxy, err := NewGRPCPublishProxy("default", getTestGrpcProxy(commonProxyTestCase))
	if err != nil {
		log.Fatalln("could not create grpc publish proxy: ", err)
	}

	publishProxyHandler := NewPublishHandler(PublishHandlerConfig{
		Proxies: map[string]PublishProxy{
			"test": publishProxy,
		},
	})

	return grpcPublishHandleTestCase{commonProxyTestCase, publishProxyHandler, opts}
}

type httpPublishHandleTestCase struct {
	*tools.CommonHTTPProxyTestCase
	publishProxyHandler *PublishHandler
	channelOpts         configtypes.ChannelOptions
}

func newPublishHandleHTTPTestCase(ctx context.Context, endpoint string, opts configtypes.ChannelOptions) httpPublishHandleTestCase {
	commonProxyTestCase := tools.NewCommonHTTPProxyTestCase(ctx)

	publishProxy, err := NewHTTPPublishProxy(getTestHttpProxy(commonProxyTestCase, endpoint))
	if err != nil {
		log.Fatalln("could not create http publish proxy: ", err)
	}

	publishProxyHandler := NewPublishHandler(PublishHandlerConfig{
		Proxies: map[string]PublishProxy{
			"test": publishProxy,
		},
	})

	return httpPublishHandleTestCase{commonProxyTestCase, publishProxyHandler, opts}
}

type publishHandleTestCase struct {
	publishProxyHandler *PublishHandler
	protocol            string
	node                *centrifuge.Node
	client              *centrifuge.Client
	channelOpts         configtypes.ChannelOptions
}

func (c publishHandleTestCase) invokeHandle() (reply centrifuge.PublishReply, err error) {
	publishHandler := c.publishProxyHandler.Handle(c.node)
	reply, err = publishHandler(c.client, centrifuge.PublishEvent{}, c.channelOpts, PerCallData{})

	return reply, err
}

func newPublishHandleTestCases(httpTestCase httpPublishHandleTestCase, grpcTestCase grpcPublishHandleTestCase) []publishHandleTestCase {
	return []publishHandleTestCase{
		{
			publishProxyHandler: grpcTestCase.publishProxyHandler,
			node:                grpcTestCase.Node,
			client:              grpcTestCase.Client,
			channelOpts:         grpcTestCase.channelOpts,
			protocol:            "grpc",
		},
		{
			publishProxyHandler: httpTestCase.publishProxyHandler,
			node:                httpTestCase.Node,
			client:              httpTestCase.Client,
			channelOpts:         httpTestCase.channelOpts,
			protocol:            "http",
		},
	}
}

func TestHandlePublishWithResult(t *testing.T) {
	customData := "test"
	customDataB64 := base64.StdEncoding.EncodeToString([]byte(customData))
	chOpts := configtypes.ChannelOptions{
		HistoryTTL:          configtypes.Duration(1 * time.Second),
		HistorySize:         1,
		PublishProxyEnabled: true,
		PublishProxyName:    "test",
	}

	opts := proxyGRPCTestServerOptions{
		B64Data: customDataB64,
	}
	grpcTestCase := newPublishHandleGRPCTestCase(context.Background(), newProxyGRPCTestServer("result", opts), chOpts)
	defer grpcTestCase.Teardown()

	httpTestCase := newPublishHandleHTTPTestCase(context.Background(), "/publish", chOpts)
	httpTestCase.Mux.HandleFunc("/publish", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(fmt.Sprintf(`{"result": {"b64data": "%s"}}`, customDataB64)))
	})
	defer httpTestCase.Teardown()

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
	chOpts := configtypes.ChannelOptions{
		HistoryTTL:          configtypes.Duration(1 * time.Second),
		HistorySize:         1,
		PublishProxyEnabled: true,
		PublishProxyName:    "test",
	}

	opts := proxyGRPCTestServerOptions{
		B64Data: customDataB64,
	}
	grpcTestCase := newPublishHandleGRPCTestCase(context.Background(), newProxyGRPCTestServer("skip history", opts), chOpts)
	defer grpcTestCase.Teardown()

	httpTestCase := newPublishHandleHTTPTestCase(context.Background(), "/publish", chOpts)
	httpTestCase.Mux.HandleFunc("/publish", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(fmt.Sprintf(`{"result": {"b64data": "%s", "skip_history": true}}`, customDataB64)))
	})
	defer httpTestCase.Teardown()

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

	grpcTestCase := newPublishHandleGRPCTestCase(ctx, proxyGRPCTestServer{}, configtypes.ChannelOptions{
		PublishProxyEnabled: true,
		PublishProxyName:    "test",
	})
	defer grpcTestCase.Teardown()

	httpTestCase := newPublishHandleHTTPTestCase(ctx, "/publish", configtypes.ChannelOptions{
		PublishProxyEnabled: true,
		PublishProxyName:    "test",
	})
	httpTestCase.Mux.HandleFunc("/publish", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	})
	defer httpTestCase.Teardown()

	cases := newPublishHandleTestCases(httpTestCase, grpcTestCase)
	for _, c := range cases {
		reply, err := c.invokeHandle()
		require.ErrorIs(t, err, centrifuge.DisconnectConnectionClosed, c.protocol)
		require.Equal(t, centrifuge.PublishReply{}, reply, c.protocol)
	}
}

func TestHandlePublishWithoutProxyServerStart(t *testing.T) {
	grpcTestCase := newPublishHandleGRPCTestCase(context.Background(), proxyGRPCTestServer{}, configtypes.ChannelOptions{
		PublishProxyEnabled: true,
		PublishProxyName:    "test",
	})
	grpcTestCase.Teardown()

	httpTestCase := newPublishHandleHTTPTestCase(context.Background(), "/publish", configtypes.ChannelOptions{
		PublishProxyEnabled: true,
		PublishProxyName:    "test",
	})
	httpTestCase.Teardown()

	cases := newPublishHandleTestCases(httpTestCase, grpcTestCase)
	for _, c := range cases {
		reply, err := c.invokeHandle()
		require.Error(t, err, c.protocol)
		require.Equal(t, centrifuge.PublishReply{}, reply, c.protocol)
	}
}

func TestHandlePublishWithProxyServerCustomDisconnect(t *testing.T) {
	chOpts := configtypes.ChannelOptions{
		PublishProxyEnabled: true,
		PublishProxyName:    "test",
	}
	grpcTestCase := newPublishHandleGRPCTestCase(context.Background(), newProxyGRPCTestServer("custom disconnect", proxyGRPCTestServerOptions{}), chOpts)
	defer grpcTestCase.Teardown()

	httpTestCase := newPublishHandleHTTPTestCase(context.Background(), "/publish", chOpts)
	httpTestCase.Mux.HandleFunc("/publish", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{"disconnect": {"code": 4000, "reconnect": false, "reason": "custom disconnect"}}`))
	})
	defer httpTestCase.Teardown()

	expectedErr := centrifuge.Disconnect{
		Code:   4000,
		Reason: "custom disconnect",
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
	chOpts := configtypes.ChannelOptions{
		PublishProxyEnabled: true,
		PublishProxyName:    "test",
	}
	grpcTestCase := newPublishHandleGRPCTestCase(context.Background(), newProxyGRPCTestServer("custom error", proxyGRPCTestServerOptions{}), chOpts)
	defer grpcTestCase.Teardown()

	httpTestCase := newPublishHandleHTTPTestCase(context.Background(), "/publish", chOpts)
	httpTestCase.Mux.HandleFunc("/publish", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{"error": {"code": 1000, "message": "custom error"}}`))
	})
	defer httpTestCase.Teardown()

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
	chOpts := configtypes.ChannelOptions{
		PublishProxyEnabled: true,
		PublishProxyName:    "test",
	}
	opts := proxyGRPCTestServerOptions{
		B64Data: "invalid data",
	}
	grpcTestCase := newPublishHandleGRPCTestCase(context.Background(), newProxyGRPCTestServer("result", opts), chOpts)
	defer grpcTestCase.Teardown()

	httpTestCase := newPublishHandleHTTPTestCase(context.Background(), "/publish", chOpts)
	httpTestCase.Mux.HandleFunc("/publish", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{"result": {"b64data": "invalid data"}}`))
	})
	defer httpTestCase.Teardown()

	cases := newPublishHandleTestCases(httpTestCase, grpcTestCase)
	for _, c := range cases {
		reply, err := c.invokeHandle()
		require.ErrorIs(t, centrifuge.ErrorInternal, err, c.protocol)
		require.Equal(t, centrifuge.PublishReply{}, reply, c.protocol)
	}
}
