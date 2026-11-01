package proxy

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/config"
	"github.com/centrifugal/centrifugo/v6/internal/configtypes"
	"github.com/centrifugal/centrifugo/v6/internal/tools"

	"github.com/centrifugal/centrifuge"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingProxyServer records the body and content type of every request it
// gets, by path, and answers each one with an error.
type recordingProxyServer struct {
	*httptest.Server
	mu       sync.Mutex
	requests map[string]string
}

func newRecordingProxyServer(t *testing.T) *recordingProxyServer {
	s := &recordingProxyServer{requests: map[string]string{}}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		s.requests[r.URL.Path] = r.Header.Get("Content-Type") + " " + string(body)
		s.mu.Unlock()
		_, _ = w.Write([]byte(`{"error":{"code":1000,"message":"test"}}`))
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *recordingProxyServer) request(path string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.requests[path]
}

// TestHTTPProxyRequestBodies pins the requests every proxy handler sends to an
// HTTP proxy.
func TestHTTPProxyRequestBodies(t *testing.T) {
	srv := newRecordingProxyServer(t)
	proxyConfig := func(path string) Config {
		return Config{
			Endpoint: srv.URL + path,
			Timeout:  configtypes.Duration(5 * time.Second),
			ProxyCommon: configtypes.ProxyCommon{
				IncludeConnectionMeta: true,
			},
		}
	}

	node := tools.NodeWithMemoryEngine()
	t.Cleanup(func() { _ = node.Shutdown(context.Background()) })

	client := &tools.TestClientMock{
		IDFunc:        func() string { return "client-id" },
		UserIDFunc:    func() string { return "user-id" },
		ContextFunc:   context.Background,
		TransportFunc: func() centrifuge.TransportInfo { return tools.NewTestTransport() },
	}
	pcd := PerCallData{Meta: json.RawMessage(`{"m":1}`)}
	chOpts := configtypes.ChannelOptions{
		SubscribeProxyEnabled:  true,
		SubscribeProxyName:     "pp",
		PublishProxyEnabled:    true,
		PublishProxyName:       "pp",
		SubRefreshProxyEnabled: true,
		SubRefreshProxyName:    "pp",
	}
	chOpts.Map.PublishProxyEnabled = true
	chOpts.Map.PublishProxyName = "pp"
	chOpts.Map.RemoveProxyEnabled = true
	chOpts.Map.RemoveProxyName = "pp"

	connectProxy, err := GetConnectProxy("default", proxyConfig("/connect"))
	require.NoError(t, err)
	_, _, err = NewConnectHandler(ConnectHandlerConfig{Proxy: connectProxy}, nil).Handle()(context.Background(), centrifuge.ConnectEvent{
		ClientID:  "client-id",
		Transport: tools.NewTestTransport(),
		Data:      []byte(`{"d":1}`),
		Name:      "name",
		Version:   "version",
		Channels:  []string{"a", "b"},
	})
	require.Error(t, err)

	refreshProxy, err := GetRefreshProxy("default", proxyConfig("/refresh"))
	require.NoError(t, err)
	_, _, err = NewRefreshHandler(RefreshHandlerConfig{Proxy: refreshProxy}).Handle()(client, centrifuge.RefreshEvent{}, pcd)
	require.NoError(t, err)

	subscribeProxy, err := GetSubscribeProxy("pp", proxyConfig("/subscribe"))
	require.NoError(t, err)
	_, _, err = NewSubscribeHandler(SubscribeHandlerConfig{
		Proxies: map[string]SubscribeProxy{"pp": subscribeProxy},
	}).Handle()(client, centrifuge.SubscribeEvent{Channel: "ch", Token: "token", Data: []byte(`{"d":1}`)}, chOpts, pcd)
	require.Error(t, err)

	subRefreshProxy, err := GetSubRefreshProxy("pp", proxyConfig("/sub_refresh"))
	require.NoError(t, err)
	_, _, err = NewSubRefreshHandler(SubRefreshHandlerConfig{
		Proxies: map[string]SubRefreshProxy{"pp": subRefreshProxy},
	}).Handle()(client, centrifuge.SubRefreshEvent{Channel: "ch"}, chOpts, pcd)
	require.NoError(t, err)

	publishProxy, err := GetPublishProxy("pp", proxyConfig("/publish"))
	require.NoError(t, err)
	_, err = NewPublishHandler(PublishHandlerConfig{
		Proxies: map[string]PublishProxy{"pp": publishProxy},
	}).Handle(node)(client, centrifuge.PublishEvent{Channel: "ch", Data: []byte(`{"d":1}`)}, chOpts, pcd)
	require.Error(t, err)

	rpcProxy, err := GetRpcProxy("pp", proxyConfig("/rpc"))
	require.NoError(t, err)
	cfg := config.DefaultConfig()
	cfg.RPC.WithoutNamespace = configtypes.RpcOptions{ProxyEnabled: true, ProxyName: "pp"}
	cfg.Proxies = configtypes.NamedProxies{{Name: "pp", Proxy: proxyConfig("/rpc")}}
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)
	_, err = NewRPCHandler(RPCHandlerConfig{
		Proxies: map[string]RPCProxy{"pp": rpcProxy},
	}).Handle()(client, centrifuge.RPCEvent{Method: "method", Data: []byte(`{"d":1}`)}, cfgContainer, pcd)
	require.Error(t, err)

	mapPublishProxy, err := GetMapPublishProxy("pp", proxyConfig("/map_publish"))
	require.NoError(t, err)
	_, err = NewMapPublishHandler(MapPublishHandlerConfig{
		Proxies: map[string]MapPublishProxy{"pp": mapPublishProxy},
	}).Handle(node)(client, centrifuge.MapPublishEvent{Channel: "ch", Key: "key", Data: []byte(`{"d":1}`)}, chOpts, pcd)
	require.Error(t, err)

	mapRemoveProxy, err := GetMapRemoveProxy("pp", proxyConfig("/map_remove"))
	require.NoError(t, err)
	_, err = NewMapRemoveHandler(MapRemoveHandlerConfig{
		Proxies: map[string]MapRemoveProxy{"pp": mapRemoveProxy},
	}).Handle(node)(client, centrifuge.MapRemoveEvent{Channel: "ch", Key: "key"}, chOpts, pcd)
	require.Error(t, err)

	sharedPollProxy, err := GetSharedPollRefreshProxy("pp", proxyConfig("/shared_poll_refresh"))
	require.NoError(t, err)
	_, err = NewSharedPollRefreshHandler(SharedPollRefreshHandlerConfig{
		Proxy: sharedPollProxy,
		Name:  "pp",
	}).Handle(node)(context.Background(), centrifuge.SharedPollEvent{
		Channel: "ch",
		Items:   []centrifuge.SharedPollItem{{Key: "k", Version: 2}},
	})
	require.Error(t, err)

	for path, expected := range map[string]string{
		"/connect":             `application/json {"client":"client-id","transport":"test_transport","protocol":"json","encoding":"json","data":{"d":1},"name":"name","version":"version","channels":["a","b"]}`,
		"/refresh":             `application/json {"client":"client-id","transport":"test_transport","protocol":"json","encoding":"json","user":"user-id","meta":{"m":1}}`,
		"/subscribe":           `application/json {"client":"client-id","transport":"test_transport","protocol":"json","encoding":"json","user":"user-id","channel":"ch","token":"token","meta":{"m":1},"data":{"d":1}}`,
		"/sub_refresh":         `application/json {"client":"client-id","transport":"test_transport","protocol":"json","encoding":"json","user":"user-id","channel":"ch","meta":{"m":1}}`,
		"/publish":             `application/json {"client":"client-id","transport":"test_transport","protocol":"json","encoding":"json","user":"user-id","channel":"ch","data":{"d":1},"meta":{"m":1}}`,
		"/rpc":                 `application/json {"client":"client-id","transport":"test_transport","protocol":"json","encoding":"json","user":"user-id","method":"method","data":{"d":1},"meta":{"m":1}}`,
		"/map_publish":         `application/json {"client":"client-id","transport":"test_transport","protocol":"json","encoding":"json","user":"user-id","channel":"ch","key":"key","data":{"d":1},"meta":{"m":1}}`,
		"/map_remove":          `application/json {"client":"client-id","transport":"test_transport","protocol":"json","encoding":"json","user":"user-id","channel":"ch","key":"key","meta":{"m":1}}`,
		"/shared_poll_refresh": `application/json {"channel":"ch","items":[{"key":"k","version":2}]}`,
	} {
		assert.Equal(t, expected, srv.request(path), path)
	}
}
