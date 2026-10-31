package app

import (
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/config"
	"github.com/centrifugal/centrifugo/v6/internal/confighelpers"
	"github.com/centrifugal/centrifugo/v6/internal/configtypes"
	"github.com/centrifugal/centrifugo/v6/internal/jwtverify"
	"github.com/centrifugal/centrifugo/v6/internal/metrics"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

const testUnbuiltProxy = "unbuilt"

func initTestMetrics(t *testing.T) {
	t.Helper()
	require.NoError(t, metrics.Init(metrics.Config{Registerer: prometheus.NewRegistry()}))
}

// configWithAllProxies enables every channel and RPC proxy kind which requires
// a name, for channels and RPC without namespace and within a namespace.
func configWithAllProxies(t *testing.T) config.Config {
	t.Helper()
	initTestMetrics(t)
	cfg := config.DefaultConfig()
	httpProxy := func() configtypes.Proxy {
		p := configtypes.Proxy{Endpoint: "http://localhost:1"}
		p.Timeout = configtypes.Duration(time.Second)
		return p
	}
	cfg.Channel.Proxy.Subscribe = httpProxy()
	cfg.Channel.Proxy.Publish = httpProxy()
	cfg.Channel.Proxy.SubRefresh = httpProxy()
	cfg.Channel.Proxy.MapPublish = httpProxy()
	cfg.Channel.Proxy.MapRemove = httpProxy()
	cfg.Channel.Proxy.SharedPollRefresh = httpProxy()
	cfg.RPC.Proxy = httpProxy()
	streamProxy := httpProxy()
	streamProxy.Endpoint = "grpc://localhost:1"
	cfg.Channel.Proxy.SubscribeStream = streamProxy

	opts := configtypes.ChannelOptions{
		SubscribeProxyEnabled:  true,
		SubscribeProxyName:     config.DefaultProxyName,
		PublishProxyEnabled:    true,
		PublishProxyName:       config.DefaultProxyName,
		SubRefreshProxyEnabled: true,
		SubRefreshProxyName:    config.DefaultProxyName,
	}
	opts.Map.PublishProxyEnabled = true
	opts.Map.PublishProxyName = config.DefaultProxyName
	opts.Map.RemoveProxyEnabled = true
	opts.Map.RemoveProxyName = config.DefaultProxyName
	cfg.Channel.WithoutNamespace = opts

	cfg.Channel.Namespaces = []configtypes.ChannelNamespace{
		{Name: "all", ChannelOptions: opts},
		{Name: "stream", ChannelOptions: configtypes.ChannelOptions{
			SubscribeStreamProxyEnabled: true,
			SubscribeStreamProxyName:    config.DefaultProxyName,
		}},
		{Name: "poll", ChannelOptions: configtypes.ChannelOptions{SubscriptionType: "shared_poll"}},
	}
	cfg.RPC.WithoutNamespace = configtypes.RpcOptions{ProxyEnabled: true, ProxyName: config.DefaultProxyName}
	cfg.RPC.Namespaces = []configtypes.RpcNamespace{
		{Name: "rpc", RpcOptions: configtypes.RpcOptions{ProxyEnabled: true, ProxyName: config.DefaultProxyName}},
	}
	return cfg
}

// Everything built on start must pass the check, otherwise reloading an unchanged
// config would be rejected.
func TestCheckProxiesBuiltAcceptsStartupConfig(t *testing.T) {
	cfg := configWithAllProxies(t)
	proxyMap, _, err := buildProxyMap(cfg)
	require.NoError(t, err)
	require.NoError(t, checkProxiesBuilt(cfg, proxyMap))
}

func TestCheckProxiesBuiltRejectsUnbuiltName(t *testing.T) {
	testCases := []struct {
		name    string
		modify  func(cfg *config.Config)
		wantErr string
	}{
		{"subscribe without namespace", func(cfg *config.Config) {
			cfg.Channel.WithoutNamespace.SubscribeProxyName = testUnbuiltProxy
		}, "subscribe proxy"},
		{"publish without namespace", func(cfg *config.Config) {
			cfg.Channel.WithoutNamespace.PublishProxyName = testUnbuiltProxy
		}, "publish proxy"},
		{"subscribe in namespace", func(cfg *config.Config) {
			cfg.Channel.Namespaces[0].SubscribeProxyName = testUnbuiltProxy
		}, "subscribe proxy"},
		{"publish in namespace", func(cfg *config.Config) {
			cfg.Channel.Namespaces[0].PublishProxyName = testUnbuiltProxy
		}, "publish proxy"},
		{"sub refresh in namespace", func(cfg *config.Config) {
			cfg.Channel.Namespaces[0].SubRefreshProxyName = testUnbuiltProxy
		}, "sub refresh proxy"},
		{"map publish in namespace", func(cfg *config.Config) {
			cfg.Channel.Namespaces[0].Map.PublishProxyName = testUnbuiltProxy
		}, "map publish proxy"},
		{"map remove in namespace", func(cfg *config.Config) {
			cfg.Channel.Namespaces[0].Map.RemoveProxyName = testUnbuiltProxy
		}, "map remove proxy"},
		{"subscribe stream in namespace", func(cfg *config.Config) {
			cfg.Channel.Namespaces[1].SubscribeStreamProxyName = testUnbuiltProxy
		}, "subscribe stream proxy"},
		{"shared poll in namespace", func(cfg *config.Config) {
			cfg.Channel.Namespaces[2].SharedPoll.ProxyName = testUnbuiltProxy
		}, "shared poll refresh proxy"},
		{"rpc without namespace", func(cfg *config.Config) {
			cfg.RPC.WithoutNamespace.ProxyName = testUnbuiltProxy
		}, "rpc proxy"},
		{"rpc in namespace", func(cfg *config.Config) {
			cfg.RPC.Namespaces[0].ProxyName = testUnbuiltProxy
		}, "rpc proxy"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := configWithAllProxies(t)
			proxyMap, _, err := buildProxyMap(cfg)
			require.NoError(t, err)
			tc.modify(&cfg)
			err = checkProxiesBuilt(cfg, proxyMap)
			require.ErrorContains(t, err, tc.wantErr)
			require.ErrorContains(t, err, testUnbuiltProxy)
		})
	}
}

// A kind with no proxies built on start has no handler at all, so enabling it on
// reload could never work.
func TestCheckProxiesBuiltRejectsKindNotBuilt(t *testing.T) {
	initTestMetrics(t)
	cfg := config.DefaultConfig()
	cfg.Channel.WithoutNamespace.SubscribeProxyEnabled = true
	cfg.Channel.WithoutNamespace.SubscribeProxyName = config.DefaultProxyName
	proxyMap, _, err := buildProxyMap(config.DefaultConfig())
	require.NoError(t, err)
	require.ErrorContains(t, checkProxiesBuilt(cfg, proxyMap), "subscribe proxy")
}

func TestCheckProxiesBuiltAllowsDroppingProxy(t *testing.T) {
	cfg := configWithAllProxies(t)
	proxyMap, _, err := buildProxyMap(cfg)
	require.NoError(t, err)
	require.NoError(t, checkProxiesBuilt(config.DefaultConfig(), proxyMap))
}

func TestReloadConfigRejectsUnbuiltProxy(t *testing.T) {
	initTestMetrics(t)
	httpProxy := configtypes.Proxy{Endpoint: "http://localhost:1", Timeout: configtypes.Duration(time.Second)}
	cfg := config.DefaultConfig()
	cfg.Client.Token.HMACSecretKey = "old"
	cfg.Channel.Proxy.Subscribe = httpProxy
	cfg.Channel.Namespaces = []configtypes.ChannelNamespace{
		{Name: "built", ChannelOptions: configtypes.ChannelOptions{
			SubscribeProxyEnabled: true,
			SubscribeProxyName:    config.DefaultProxyName,
		}},
	}
	proxyMap, _, err := buildProxyMap(cfg)
	require.NoError(t, err)
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)
	verifierConfig, err := confighelpers.MakeVerifierConfig(cfg.Client.Token)
	require.NoError(t, err)
	tokenVerifier, err := jwtverify.NewTokenVerifierJWT(verifierConfig, cfgContainer)
	require.NoError(t, err)

	newCfg := cfg
	newCfg.Client.Token.HMACSecretKey = "new"
	newCfg.Client.AllowAnonymousConnectWithoutToken = true
	newCfg.Proxies = configtypes.NamedProxies{{Name: testUnbuiltProxy, Proxy: httpProxy}}
	newCfg.Channel.Namespaces = []configtypes.ChannelNamespace{
		cfg.Channel.Namespaces[0],
		{Name: "other", ChannelOptions: configtypes.ChannelOptions{
			SubscribeProxyEnabled: true,
			SubscribeProxyName:    testUnbuiltProxy,
		}},
	}
	// Config itself is valid, it just can't be applied without a restart.
	require.NoError(t, newCfg.Validate())

	err = reloadConfig(newCfg, cfgContainer, proxyMap, tokenVerifier, nil)
	require.ErrorContains(t, err, testUnbuiltProxy)

	// The whole reload is rejected: neither config nor verifier changed.
	require.False(t, cfgContainer.Config().Client.AllowAnonymousConnectWithoutToken)
	_, err = tokenVerifier.VerifyConnectToken(hmacToken(t, "old", ""), false)
	require.NoError(t, err)

	// Using proxies which are already built keeps working.
	newCfg.Channel.Namespaces[1].SubscribeProxyName = config.DefaultProxyName
	require.NoError(t, newCfg.Validate())
	require.NoError(t, reloadConfig(newCfg, cfgContainer, proxyMap, tokenVerifier, nil))
	require.True(t, cfgContainer.Config().Client.AllowAnonymousConnectWithoutToken)
}
