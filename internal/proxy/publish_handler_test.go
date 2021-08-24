package proxy

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/centrifugal/centrifugo/v3/internal/rule"

	"github.com/centrifugal/centrifuge"
	"github.com/centrifugal/centrifugo/v3/internal/proxyproto"
	"github.com/centrifugal/centrifugo/v3/internal/tools"
)

type publishHandlerTestDepsConfig struct {
	publishProxyHandler *PublishHandler
	transport           *tools.TestTransport
}

func newPublishHandlerTestDepsConfig(proxyEndpoint string) publishHandlerTestDepsConfig {
	proxyCfg := Config{
		HTTPConfig: HTTPConfig{
			Encoder: &proxyproto.JSONEncoder{},
			Decoder: &proxyproto.JSONDecoder{},
		},
		PublishEndpoint: proxyEndpoint,
	}

	connectProxy, err := NewHTTPPublishProxy(
		proxyEndpoint,
		proxyCfg,
	)
	if err != nil {
		log.Fatalln("could not create http publish proxy: ", err)
	}

	publishProxyHandler := NewPublishHandler(PublishHandlerConfig{
		Proxy: connectProxy,
	})

	return publishHandlerTestDepsConfig{
		publishProxyHandler: publishProxyHandler,
		transport:           tools.NewTestTransport(),
	}
}

func TestHandlePublishWithResult(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	custData := "test"
	custDataB64 := base64.StdEncoding.EncodeToString([]byte(custData))

	mux := http.NewServeMux()
	mux.HandleFunc("/publish", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(fmt.Sprintf(`{"result": {"b64data": "%s"}}`, custDataB64)))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	testDepsCfg := newPublishHandlerTestDepsConfig(server.URL + "/publish")
	publishHandler := testDepsCfg.publishProxyHandler.Handle(node)

	client, closeFn, err := centrifuge.NewClient(context.Background(), node, testDepsCfg.transport)
	require.NoError(t, err)
	defer func() { _ = closeFn() }()

	publishEvent := centrifuge.PublishEvent{}
	chOpts := rule.ChannelOptions{
		HistoryTTL:  tools.Duration(1 * time.Nanosecond),
		HistorySize: 1,
	}

	publishReply, err := publishHandler(client, publishEvent, chOpts)
	require.NoError(t, err)
	require.Equal(t, uint64(1), publishReply.Result.Offset)
	require.NotEmpty(t, publishReply.Result.Epoch)
}

func TestHandlePublishWithContextCancel(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	mux := http.NewServeMux()
	mux.HandleFunc("/publish", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	testDepsCfg := newPublishHandlerTestDepsConfig(server.URL + "/publish")
	publishHandler := testDepsCfg.publishProxyHandler.Handle(node)

	ctx, cancel := context.WithCancel(context.Background())
	client, closeFn, err := centrifuge.NewClient(ctx, node, testDepsCfg.transport)
	require.NoError(t, err)
	defer func() { _ = closeFn() }()

	cancel()
	publishReply, err := publishHandler(client, centrifuge.PublishEvent{}, rule.ChannelOptions{})
	require.NoError(t, err)
	require.Equal(t, centrifuge.PublishReply{}, publishReply)
}

func TestHandlePublishWithoutProxyServerStart(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	testDepsCfg := newPublishHandlerTestDepsConfig("/publish")
	publishHandler := testDepsCfg.publishProxyHandler.Handle(node)

	client, closeFn, err := centrifuge.NewClient(context.Background(), node, testDepsCfg.transport)
	require.NoError(t, err)
	defer func() { _ = closeFn() }()

	publishReply, err := publishHandler(client, centrifuge.PublishEvent{}, rule.ChannelOptions{})

	require.ErrorIs(t, centrifuge.ErrorInternal, err)
	require.Equal(t, centrifuge.PublishReply{}, publishReply)
}

func TestHandlePublishWithProxyServerCustomDisconnect(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	mux := http.NewServeMux()
	mux.HandleFunc("/publish", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{"disconnect": {"code": 4000, "reconnect": false, "reason": "custom disconnect"}}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	testDepsCfg := newPublishHandlerTestDepsConfig(server.URL + "/publish")
	publishHandler := testDepsCfg.publishProxyHandler.Handle(node)

	client, closeFn, err := centrifuge.NewClient(context.Background(), node, testDepsCfg.transport)
	require.NoError(t, err)
	defer func() { _ = closeFn() }()

	expectedErr := centrifuge.Disconnect{
		Code:      4000,
		Reason:    "custom disconnect",
		Reconnect: false,
	}
	publishReply, err := publishHandler(client, centrifuge.PublishEvent{}, rule.ChannelOptions{})

	require.Equal(t, expectedErr.Error(), err.Error())
	require.Equal(t, centrifuge.PublishReply{}, publishReply)
}

func TestHandlePublishWithProxyServerCustomError(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	mux := http.NewServeMux()
	mux.HandleFunc("/publish", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{"error": {"code": 1000, "message": "custom error"}}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	testDepsCfg := newPublishHandlerTestDepsConfig(server.URL + "/publish")
	publishHandler := testDepsCfg.publishProxyHandler.Handle(node)

	client, closeFn, err := centrifuge.NewClient(context.Background(), node, testDepsCfg.transport)
	require.NoError(t, err)
	defer func() { _ = closeFn() }()

	publishReply, err := publishHandler(client, centrifuge.PublishEvent{}, rule.ChannelOptions{})
	expectedErr := centrifuge.Error{
		Code:    1000,
		Message: "custom error",
	}

	require.Equal(t, expectedErr.Error(), err.Error())
	require.Equal(t, centrifuge.PublishReply{}, publishReply)
}

func TestHandlePublishWithInvalidCustomData(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	mux := http.NewServeMux()
	mux.HandleFunc("/publish", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{"result": {"b64data": "invalid data"}}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	testDepsCfg := newPublishHandlerTestDepsConfig(server.URL + "/publish")
	publishHandler := testDepsCfg.publishProxyHandler.Handle(node)

	client, closeFn, err := centrifuge.NewClient(context.Background(), node, testDepsCfg.transport)
	require.NoError(t, err)
	defer func() { _ = closeFn() }()

	publishReply, err := publishHandler(client, centrifuge.PublishEvent{}, rule.ChannelOptions{})
	require.ErrorIs(t, centrifuge.ErrorInternal, err)
	require.Equal(t, centrifuge.PublishReply{}, publishReply)

}
