package app

import (
	"fmt"
	"strings"

	"github.com/centrifugal/centrifugo/v6/internal/client"
	"github.com/centrifugal/centrifugo/v6/internal/config"
	"github.com/centrifugal/centrifugo/v6/internal/configtypes"
	"github.com/centrifugal/centrifugo/v6/internal/proxy"
	"github.com/centrifugal/centrifugo/v6/internal/tools"

	"github.com/rs/zerolog/log"
)

func buildProxyMap(cfg config.Config) (*client.ProxyMap, bool, error) {
	proxyMap := &client.ProxyMap{
		ConnectProxy:           nil,
		RefreshProxy:           nil,
		RpcProxies:             map[string]proxy.RPCProxy{},
		PublishProxies:         map[string]proxy.PublishProxy{},
		SubscribeProxies:       map[string]proxy.SubscribeProxy{},
		SubRefreshProxies:      map[string]proxy.SubRefreshProxy{},
		SubscribeStreamProxies: map[string]*proxy.SubscribeStreamProxy{},
	}

	var keepHeadersInContext bool

	var err error
	var proxyFound bool

	if cfg.Client.Proxy.Connect.Enabled {
		p := cfg.Client.Proxy.Connect
		proxyMap.ConnectProxy, err = proxy.GetConnectProxy("connect", cfg.Client.Proxy.Connect.Proxy)
		if err != nil {
			return nil, false, fmt.Errorf("error creating connect proxy: %w", err)
		}
		log.Info().Str("endpoint", tools.RedactedLogURLs(p.Endpoint)[0]).Msg("connect proxy enabled")
		if len(p.HttpHeaders) > 0 {
			keepHeadersInContext = true
		}
	}

	if cfg.Client.Proxy.Refresh.Enabled {
		p := cfg.Client.Proxy.Refresh
		proxyMap.RefreshProxy, err = proxy.GetRefreshProxy("refresh", cfg.Client.Proxy.Refresh.Proxy)
		if err != nil {
			return nil, false, fmt.Errorf("error creating refresh proxy: %w", err)
		}
		log.Info().Str("endpoint", tools.RedactedLogURLs(p.Endpoint)[0]).Msg("refresh proxy enabled")
		if len(p.HttpHeaders) > 0 {
			keepHeadersInContext = true
		}
	}

	namedProxies := make(map[string]configtypes.Proxy)
	for _, p := range cfg.Proxies {
		namedProxies[p.Name] = p.Proxy
	}

	subscribeProxyEnabled := cfg.Channel.WithoutNamespace.SubscribeProxyEnabled
	subscribeProxyName := cfg.Channel.WithoutNamespace.SubscribeProxyName
	if subscribeProxyEnabled {
		var p proxy.Config
		if subscribeProxyName == config.DefaultProxyName {
			p = cfg.Channel.Proxy.Subscribe
		} else {
			p, proxyFound = namedProxies[subscribeProxyName]
			if !proxyFound {
				return nil, false, fmt.Errorf("subscribe proxy not found: %s", subscribeProxyName)
			}
		}
		if _, ok := proxyMap.SubscribeProxies[subscribeProxyName]; !ok {
			sp, err := proxy.GetSubscribeProxy(subscribeProxyName, p)
			if err != nil {
				return nil, false, fmt.Errorf("error creating subscribe proxy %s: %w", subscribeProxyName, err)
			}
			proxyMap.SubscribeProxies[subscribeProxyName] = sp
		}
		log.Info().Str("proxy_name", subscribeProxyName).Str("endpoint", tools.RedactedLogURLs(p.Endpoint)[0]).Msg("subscribe proxy enabled for channels without namespace")
		if len(p.HttpHeaders) > 0 {
			keepHeadersInContext = true
		}
	}

	publishProxyEnabled := cfg.Channel.WithoutNamespace.PublishProxyEnabled
	publishProxyName := cfg.Channel.WithoutNamespace.PublishProxyName
	if publishProxyEnabled {
		var p proxy.Config
		if publishProxyName == config.DefaultProxyName {
			p = cfg.Channel.Proxy.Publish
		} else {
			p, proxyFound = namedProxies[publishProxyName]
			if !proxyFound {
				return nil, false, fmt.Errorf("publish proxy not found: %s", publishProxyName)
			}
		}
		if _, ok := proxyMap.PublishProxies[publishProxyName]; !ok {
			pp, err := proxy.GetPublishProxy(publishProxyName, p)
			if err != nil {
				return nil, false, fmt.Errorf("error creating publish proxy %s: %w", publishProxyName, err)
			}
			proxyMap.PublishProxies[publishProxyName] = pp
		}
		log.Info().Str("proxy_name", publishProxyName).Str("endpoint", tools.RedactedLogURLs(p.Endpoint)[0]).Msg("publish proxy enabled for channels without namespace")
		if len(p.HttpHeaders) > 0 {
			keepHeadersInContext = true
		}
	}

	subRefreshProxyEnabled := cfg.Channel.WithoutNamespace.SubRefreshProxyEnabled
	subRefreshProxyName := cfg.Channel.WithoutNamespace.SubRefreshProxyName
	if subRefreshProxyEnabled {
		var p proxy.Config
		if subRefreshProxyName == config.DefaultProxyName {
			p = cfg.Channel.Proxy.SubRefresh
		} else {
			p, proxyFound = namedProxies[subRefreshProxyName]
			if !proxyFound {
				return nil, false, fmt.Errorf("sub refresh proxy not found: %s", subRefreshProxyName)
			}
		}
		if _, ok := proxyMap.SubRefreshProxies[subRefreshProxyName]; !ok {
			srp, err := proxy.GetSubRefreshProxy(subRefreshProxyName, p)
			if err != nil {
				return nil, false, fmt.Errorf("error creating sub refresh proxy %s: %w", subRefreshProxyName, err)
			}
			proxyMap.SubRefreshProxies[subRefreshProxyName] = srp
		}
		log.Info().Str("proxy_name", subRefreshProxyName).Str("endpoint", tools.RedactedLogURLs(p.Endpoint)[0]).Msg("sub refresh proxy enabled for channels without namespace")
		if len(p.HttpHeaders) > 0 {
			keepHeadersInContext = true
		}
	}

	subscribeStreamProxyEnabled := cfg.Channel.WithoutNamespace.SubscribeStreamProxyEnabled
	subscribeStreamProxyName := cfg.Channel.WithoutNamespace.SubscribeStreamProxyName
	if subscribeStreamProxyEnabled {
		var p proxy.Config
		if subscribeStreamProxyName == config.DefaultProxyName {
			p = cfg.Channel.Proxy.SubscribeStream
		} else {
			p, proxyFound = namedProxies[subscribeStreamProxyName]
			if !proxyFound {
				return nil, false, fmt.Errorf("subscribe stream proxy not found: %s", subscribeStreamProxyName)
			}
		}
		if strings.HasPrefix(p.Endpoint, "http") {
			log.Fatal().Str("name", subscribeStreamProxyName).Msg("error creating subscribe stream proxy – only GRPC endpoints supported")
		}
		if _, ok := proxyMap.SubscribeStreamProxies[subscribeStreamProxyName]; !ok {
			sp, err := proxy.NewSubscribeStreamProxy(subscribeStreamProxyName, p)
			if err != nil {
				return nil, false, fmt.Errorf("error creating subscribe stream proxy %s: %w", subscribeStreamProxyName, err)
			}
			proxyMap.SubscribeStreamProxies[subscribeStreamProxyName] = sp
		}
		log.Info().Str("proxy_name", subscribeStreamProxyName).Str("endpoint", tools.RedactedLogURLs(p.Endpoint)[0]).Msg("subscribe stream proxy enabled for channels without namespace")
		if len(p.HttpHeaders) > 0 {
			keepHeadersInContext = true
		}
	}

	for _, ns := range cfg.Channel.Namespaces {
		subscribeProxyEnabled := ns.SubscribeProxyEnabled
		subscribeProxyName := ns.SubscribeProxyName
		if subscribeProxyEnabled {
			var p proxy.Config
			if subscribeProxyName == config.DefaultProxyName {
				p = cfg.Channel.Proxy.Subscribe
			} else {
				p, proxyFound = namedProxies[subscribeProxyName]
				if !proxyFound {
					return nil, false, fmt.Errorf("subscribe proxy not found: %s", subscribeProxyName)
				}
			}
			if _, ok := proxyMap.SubscribeProxies[subscribeProxyName]; !ok {
				sp, err := proxy.GetSubscribeProxy(subscribeProxyName, p)
				if err != nil {
					return nil, false, fmt.Errorf("error creating subscribe proxy %s: %w", subscribeProxyName, err)
				}
				proxyMap.SubscribeProxies[subscribeProxyName] = sp
			}
			log.Info().Str("proxy_name", subscribeProxyName).Str("endpoint", tools.RedactedLogURLs(p.Endpoint)[0]).Str("namespace", ns.Name).Msg("subscribe proxy enabled for channels in namespace")
			if len(p.HttpHeaders) > 0 {
				keepHeadersInContext = true
			}
		}

		publishProxyEnabled := ns.PublishProxyEnabled
		publishProxyName := ns.PublishProxyName
		if publishProxyEnabled {
			var p proxy.Config
			if publishProxyName == config.DefaultProxyName {
				p = cfg.Channel.Proxy.Publish
			} else {
				p, proxyFound = namedProxies[publishProxyName]
				if !proxyFound {
					return nil, false, fmt.Errorf("publish proxy not found: %s", publishProxyName)
				}
			}
			if _, ok := proxyMap.PublishProxies[publishProxyName]; !ok {
				pp, err := proxy.GetPublishProxy(publishProxyName, p)
				if err != nil {
					return nil, false, fmt.Errorf("error creating publish proxy %s: %w", publishProxyName, err)
				}
				proxyMap.PublishProxies[publishProxyName] = pp
			}
			log.Info().Str("proxy_name", publishProxyName).Str("endpoint", tools.RedactedLogURLs(p.Endpoint)[0]).Str("namespace", ns.Name).Msg("publish proxy enabled for channels in namespace")
			if len(p.HttpHeaders) > 0 {
				keepHeadersInContext = true
			}
		}

		subRefreshProxyEnabled := ns.SubRefreshProxyEnabled
		subRefreshProxyName := ns.SubRefreshProxyName
		if subRefreshProxyEnabled {
			var p proxy.Config
			if subRefreshProxyName == config.DefaultProxyName {
				p = cfg.Channel.Proxy.SubRefresh
			} else {
				p, proxyFound = namedProxies[subRefreshProxyName]
				if !proxyFound {
					return nil, false, fmt.Errorf("sub refresh proxy not found: %s", subRefreshProxyName)
				}
			}
			if _, ok := proxyMap.SubRefreshProxies[subRefreshProxyName]; !ok {
				srp, err := proxy.GetSubRefreshProxy(subRefreshProxyName, p)
				if err != nil {
					return nil, false, fmt.Errorf("error creating sub refresh proxy %s: %w", subRefreshProxyName, err)
				}
				proxyMap.SubRefreshProxies[subRefreshProxyName] = srp
			}
			log.Info().Str("proxy_name", subRefreshProxyName).Str("endpoint", tools.RedactedLogURLs(p.Endpoint)[0]).Str("namespace", ns.Name).Msg("sub refresh proxy enabled for channels in namespace")
			if len(p.HttpHeaders) > 0 {
				keepHeadersInContext = true
			}
		}

		subscribeStreamProxyEnabled := ns.SubscribeStreamProxyEnabled
		subscribeStreamProxyName := ns.SubscribeStreamProxyName
		if subscribeStreamProxyEnabled {
			var p proxy.Config
			if subscribeStreamProxyName == config.DefaultProxyName {
				p = cfg.Channel.Proxy.SubscribeStream
			} else {
				p, proxyFound = namedProxies[subscribeStreamProxyName]
				if !proxyFound {
					return nil, false, fmt.Errorf("subscribe stream proxy not found: %s", subscribeStreamProxyName)
				}
			}
			if strings.HasPrefix(p.Endpoint, "http") {
				return nil, false, fmt.Errorf("error creating subscribe stream proxy %s only GRPC endpoints supported", subscribeStreamProxyName)
			}
			if _, ok := proxyMap.SubscribeStreamProxies[subscribeStreamProxyName]; !ok {
				sp, err := proxy.NewSubscribeStreamProxy(subscribeStreamProxyName, p)
				if err != nil {
					return nil, false, fmt.Errorf("error creating subscribe stream proxy %s: %w", subscribeStreamProxyName, err)
				}
				proxyMap.SubscribeStreamProxies[subscribeStreamProxyName] = sp
			}
			log.Info().Str("proxy_name", subscribeStreamProxyName).Str("endpoint", tools.RedactedLogURLs(p.Endpoint)[0]).Str("namespace", ns.Name).Msg("subscribe stream proxy enabled for channels in namespace")
			if len(p.HttpHeaders) > 0 {
				keepHeadersInContext = true
			}
		}
	}

	rpcProxyEnabled := cfg.RPC.WithoutNamespace.ProxyEnabled
	rpcProxyName := cfg.RPC.WithoutNamespace.ProxyName
	if rpcProxyEnabled {
		var p proxy.Config
		if rpcProxyName == config.DefaultProxyName {
			p = cfg.RPC.Proxy
		} else {
			p, proxyFound = namedProxies[rpcProxyName]
			if !proxyFound {
				return nil, false, fmt.Errorf("rpc proxy not found: %s", rpcProxyName)
			}
		}
		if _, ok := proxyMap.RpcProxies[rpcProxyName]; !ok {
			rp, err := proxy.GetRpcProxy(rpcProxyName, p)
			if err != nil {
				return nil, false, fmt.Errorf("error creating rpc proxy %s: %w", rpcProxyName, err)
			}
			proxyMap.RpcProxies[rpcProxyName] = rp
		}
		log.Info().Str("proxy_name", rpcProxyName).Str("endpoint", tools.RedactedLogURLs(p.Endpoint)[0]).Msg("RPC proxy enabled for methods without namespace")
		if len(p.HttpHeaders) > 0 {
			keepHeadersInContext = true
		}
	}

	for _, ns := range cfg.RPC.Namespaces {
		rpcProxyEnabled := ns.ProxyEnabled
		rpcProxyName := ns.ProxyName
		if rpcProxyEnabled {
			var p proxy.Config
			if rpcProxyName == config.DefaultProxyName {
				p = cfg.RPC.Proxy
			} else {
				p, proxyFound = namedProxies[rpcProxyName]
				if !proxyFound {
					return nil, false, fmt.Errorf("rpc proxy not found: %s", rpcProxyName)
				}
			}
			if _, ok := proxyMap.RpcProxies[rpcProxyName]; !ok {
				rp, err := proxy.GetRpcProxy(rpcProxyName, p)
				if err != nil {
					return nil, false, fmt.Errorf("error creating rpc proxy %s: %w", rpcProxyName, err)
				}
				proxyMap.RpcProxies[rpcProxyName] = rp
			}
			log.Info().Str("proxy_name", rpcProxyName).Str("endpoint", tools.RedactedLogURLs(p.Endpoint)[0]).Str("namespace", ns.Name).Msg("RPC proxy enabled for namespace")
			if len(p.HttpHeaders) > 0 {
				keepHeadersInContext = true
			}
		}
	}

	return proxyMap, keepHeadersInContext, nil
}
