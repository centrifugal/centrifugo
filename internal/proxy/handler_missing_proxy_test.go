package proxy

import (
	"context"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/config"
	"github.com/centrifugal/centrifugo/v6/internal/configtypes"
	"github.com/centrifugal/centrifugo/v6/internal/tools"

	"github.com/centrifugal/centrifuge"
	"github.com/stretchr/testify/require"
)

const testMissingProxy = "missing"

func TestHandlersReturnNotAvailableForMissingProxy(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tc := tools.NewCommonHTTPProxyTestCase(ctx)
	defer tc.Teardown()

	cfg := config.DefaultConfig()
	// Declared in config, so it passes validation, but not built into any handler.
	cfg.Proxies = configtypes.NamedProxies{{
		Name:  testMissingProxy,
		Proxy: configtypes.Proxy{Endpoint: "http://localhost:1", Timeout: configtypes.Duration(time.Second)},
	}}
	cfg.RPC.WithoutNamespace = configtypes.RpcOptions{ProxyEnabled: true, ProxyName: testMissingProxy}
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)

	opts := configtypes.ChannelOptions{
		SubscribeProxyEnabled:       true,
		SubscribeProxyName:          testMissingProxy,
		PublishProxyEnabled:         true,
		PublishProxyName:            testMissingProxy,
		SubscribeStreamProxyEnabled: true,
		SubscribeStreamProxyName:    testMissingProxy,
	}
	opts.Map.PublishProxyEnabled = true
	opts.Map.PublishProxyName = testMissingProxy
	opts.Map.RemoveProxyEnabled = true
	opts.Map.RemoveProxyName = testMissingProxy

	testCases := []struct {
		name string
		call func() error
	}{
		{"subscribe", func() error {
			_, _, err := NewSubscribeHandler(SubscribeHandlerConfig{}).Handle()(tc.Client, centrifuge.SubscribeEvent{}, opts, PerCallData{})
			return err
		}},
		{"publish", func() error {
			_, err := NewPublishHandler(PublishHandlerConfig{}).Handle(tc.Node)(tc.Client, centrifuge.PublishEvent{}, opts, PerCallData{})
			return err
		}},
		{"rpc", func() error {
			_, err := NewRPCHandler(RPCHandlerConfig{}).Handle()(tc.Client, centrifuge.RPCEvent{Method: "test"}, cfgContainer, PerCallData{})
			return err
		}},
		{"subscribe stream", func() error {
			_, _, _, err := NewSubscribeStreamHandler(SubscribeStreamHandlerConfig{}).Handle()(tc.Client, false, centrifuge.SubscribeEvent{}, opts, PerCallData{})
			return err
		}},
		{"map publish", func() error {
			_, err := NewMapPublishHandler(MapPublishHandlerConfig{}).Handle(tc.Node)(tc.Client, centrifuge.MapPublishEvent{}, opts, PerCallData{})
			return err
		}},
		{"map remove", func() error {
			_, err := NewMapRemoveHandler(MapRemoveHandlerConfig{}).Handle(tc.Node)(tc.Client, centrifuge.MapRemoveEvent{}, opts, PerCallData{})
			return err
		}},
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			require.ErrorIs(t, c.call(), centrifuge.ErrorNotAvailable)
		})
	}
}

// A sub refresh can run from the subscription expiration timer, where an error
// disconnects the client. So the subscription gets prolonged instead, as for a
// failed proxy call.
func TestSubRefreshHandlerProlongsForMissingProxy(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tc := tools.NewCommonHTTPProxyTestCase(ctx)
	defer tc.Teardown()

	opts := configtypes.ChannelOptions{SubRefreshProxyEnabled: true, SubRefreshProxyName: testMissingProxy}
	now := time.Now().Unix()
	reply, _, err := NewSubRefreshHandler(SubRefreshHandlerConfig{}).Handle()(tc.Client, centrifuge.SubRefreshEvent{}, opts, PerCallData{})
	require.NoError(t, err)
	require.Greater(t, reply.ExpireAt, now)
	require.False(t, reply.Expired)
}
