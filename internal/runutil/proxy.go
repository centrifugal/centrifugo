package runutil

import (
	"strings"

	"github.com/centrifugal/centrifugo/v5/internal/client"
	"github.com/centrifugal/centrifugo/v5/internal/config"
	"github.com/centrifugal/centrifugo/v5/internal/proxy"

	"github.com/rs/zerolog/log"
)

func buildProxyMap(cfg config.Config) (*client.ProxyMap, bool) {
	proxyMap := &client.ProxyMap{
		RpcProxies:             map[string]proxy.RPCProxy{},
		PublishProxies:         map[string]proxy.PublishProxy{},
		SubscribeProxies:       map[string]proxy.SubscribeProxy{},
		SubRefreshProxies:      map[string]proxy.SubRefreshProxy{},
		SubscribeStreamProxies: map[string]*proxy.SubscribeStreamProxy{},
		CacheEmptyProxies:      map[string]proxy.CacheEmptyProxy{},
	}
	proxyList := cfg.Proxies
	proxies := make(map[string]proxy.Config)
	for _, p := range proxyList {
		for i, header := range p.HttpHeaders {
			p.HttpHeaders[i] = strings.ToLower(header)
		}
		proxies[p.Name] = p
	}

	globalProxyConfig := proxy.Config{
		Name:        config.UnifiedProxyName,
		ProxyCommon: cfg.UnifiedProxy.ProxyCommon,
	}

	var keepHeadersInContext bool

	connectProxyName := cfg.Client.ConnectProxyName
	if connectProxyName != "" {
		var p proxy.Config
		if connectProxyName == config.UnifiedProxyName {
			globalProxyConfig.Endpoint = cfg.UnifiedProxy.ConnectEndpoint
			globalProxyConfig.Timeout = cfg.UnifiedProxy.ConnectTimeout
			p = globalProxyConfig
		} else {
			var ok bool
			p, ok = proxies[connectProxyName]
			if !ok {
				log.Fatal().Msgf("connect proxy with name %s not found", connectProxyName)
			}
		}
		var err error
		proxyMap.ConnectProxy, err = proxy.GetConnectProxy(p)
		if err != nil {
			log.Fatal().Msgf("error creating connect proxy: %v", err)
		}
		log.Info().Str("proxy_name", connectProxyName).Str("endpoint", p.Endpoint).Msg("connect proxy enabled")
		keepHeadersInContext = true
	}

	refreshProxyName := cfg.Client.RefreshProxyName
	if refreshProxyName != "" {
		var p proxy.Config
		if refreshProxyName == config.UnifiedProxyName {
			globalProxyConfig.Endpoint = cfg.UnifiedProxy.RefreshEndpoint
			globalProxyConfig.Timeout = cfg.UnifiedProxy.RefreshTimeout
			p = globalProxyConfig
		} else {
			var ok bool
			p, ok = proxies[refreshProxyName]
			if !ok {
				log.Fatal().Msgf("refresh proxy with name %s not found", refreshProxyName)
			}
		}
		var err error
		proxyMap.RefreshProxy, err = proxy.GetRefreshProxy(p)
		if err != nil {
			log.Fatal().Msgf("error creating refresh proxy: %v", err)
		}
		log.Info().Str("proxy_name", refreshProxyName).Str("endpoint", p.Endpoint).Msg("refresh proxy enabled")
		keepHeadersInContext = true
	}

	subscribeProxyName := cfg.Channel.WithoutNamespace.SubscribeProxyName
	if subscribeProxyName != "" {
		var p proxy.Config
		if subscribeProxyName == config.UnifiedProxyName {
			globalProxyConfig.Endpoint = cfg.UnifiedProxy.SubscribeEndpoint
			globalProxyConfig.Timeout = cfg.UnifiedProxy.SubscribeTimeout
			p = globalProxyConfig
		} else {
			var ok bool
			p, ok = proxies[subscribeProxyName]
			if !ok {
				log.Fatal().Msgf("subscribe proxy with name %s not found", subscribeProxyName)
			}
		}
		sp, err := proxy.GetSubscribeProxy(p)
		if err != nil {
			log.Fatal().Msgf("error creating subscribe proxy: %v", err)
		}
		proxyMap.SubscribeProxies[subscribeProxyName] = sp
		log.Info().Str("proxy_name", subscribeProxyName).Str("endpoint", p.Endpoint).Msg("subscribe proxy enabled for channels without namespace")
		keepHeadersInContext = true
	}

	publishProxyName := cfg.Channel.WithoutNamespace.PublishProxyName
	if publishProxyName != "" {
		var p proxy.Config
		if publishProxyName == config.UnifiedProxyName {
			globalProxyConfig.Endpoint = cfg.UnifiedProxy.PublishEndpoint
			globalProxyConfig.Timeout = cfg.UnifiedProxy.PublishTimeout
			p = globalProxyConfig
		} else {
			var ok bool
			p, ok = proxies[publishProxyName]
			if !ok {
				log.Fatal().Msgf("publish proxy with name %s not found", publishProxyName)
			}
		}
		pp, err := proxy.GetPublishProxy(p)
		if err != nil {
			log.Fatal().Msgf("error creating publish proxy: %v", err)
		}
		proxyMap.PublishProxies[publishProxyName] = pp
		log.Info().Str("proxy_name", publishProxyName).Str("endpoint", p.Endpoint).Msg("publish proxy enabled for channels without namespace")
		keepHeadersInContext = true
	}

	subRefreshProxyName := cfg.Channel.WithoutNamespace.SubRefreshProxyName
	if subRefreshProxyName != "" {
		var p proxy.Config
		if subRefreshProxyName == config.UnifiedProxyName {
			globalProxyConfig.Endpoint = cfg.UnifiedProxy.SubRefreshEndpoint
			globalProxyConfig.Timeout = cfg.UnifiedProxy.SubRefreshTimeout
			p = globalProxyConfig
		} else {
			var ok bool
			p, ok = proxies[subRefreshProxyName]
			if !ok {
				log.Fatal().Msgf("sub refresh proxy not found: %s", subRefreshProxyName)
			}
		}
		srp, err := proxy.GetSubRefreshProxy(p)
		if err != nil {
			log.Fatal().Msgf("error creating publish proxy: %v", err)
		}
		proxyMap.SubRefreshProxies[subRefreshProxyName] = srp
		log.Info().Str("proxy_name", subRefreshProxyName).Str("endpoint", p.Endpoint).Msg("sub refresh proxy enabled for channels without namespace")
		keepHeadersInContext = true
	}

	subscribeStreamProxyName := cfg.Channel.WithoutNamespace.SubscribeStreamProxyName
	if subscribeStreamProxyName != "" {
		var p proxy.Config
		if subscribeStreamProxyName == config.UnifiedProxyName {
			globalProxyConfig.Endpoint = cfg.UnifiedProxy.SubscribeStreamEndpoint
			globalProxyConfig.Timeout = cfg.UnifiedProxy.SubscribeStreamTimeout
			p = globalProxyConfig
		} else {
			var ok bool
			p, ok = proxies[subscribeStreamProxyName]
			if !ok {
				log.Fatal().Msgf("subscribe stream proxy not found: %s", subscribeStreamProxyName)
			}
		}
		if strings.HasPrefix(p.Endpoint, "http") {
			log.Fatal().Msgf("error creating subscribe stream proxy %s only GRPC endpoints supported", subscribeStreamProxyName)
		}
		sp, err := proxy.NewSubscribeStreamProxy(p)
		if err != nil {
			log.Fatal().Msgf("error creating subscribe proxy: %v", err)
		}
		proxyMap.SubscribeStreamProxies[subscribeProxyName] = sp
		log.Info().Str("proxy_name", subscribeStreamProxyName).Str("endpoint", p.Endpoint).Msg("subscribe stream proxy enabled for channels without namespace")
		keepHeadersInContext = true
	}

	for _, ns := range cfg.Channel.Namespaces {
		subscribeProxyName := ns.SubscribeProxyName
		publishProxyName := ns.PublishProxyName
		subRefreshProxyName := ns.SubRefreshProxyName
		subscribeStreamProxyName := ns.SubscribeStreamProxyName

		if subscribeProxyName != "" {
			var p proxy.Config
			if subscribeProxyName == config.UnifiedProxyName {
				globalProxyConfig.Endpoint = cfg.UnifiedProxy.SubscribeEndpoint
				globalProxyConfig.Timeout = cfg.UnifiedProxy.SubscribeTimeout
				p = globalProxyConfig
			} else {
				var ok bool
				p, ok = proxies[subscribeProxyName]
				if !ok {
					log.Fatal().Msgf("subscribe proxy with name %s not found", subscribeProxyName)
				}
			}
			if _, ok := proxyMap.SubscribeProxies[subscribeProxyName]; !ok {
				sp, err := proxy.GetSubscribeProxy(p)
				if err != nil {
					log.Fatal().Msgf("error creating subscribe proxy: %v", err)
				}
				proxyMap.SubscribeProxies[subscribeProxyName] = sp
			}
			log.Info().Str("proxy_name", subscribeProxyName).Str("endpoint", p.Endpoint).Str("namespace", ns.Name).Msg("subscribe proxy enabled for channels in namespace")
		}

		if publishProxyName != "" {
			var p proxy.Config
			if publishProxyName == config.UnifiedProxyName {
				globalProxyConfig.Endpoint = cfg.UnifiedProxy.PublishEndpoint
				globalProxyConfig.Timeout = cfg.UnifiedProxy.PublishTimeout
				p = globalProxyConfig
			} else {
				var ok bool
				p, ok = proxies[publishProxyName]
				if !ok {
					log.Fatal().Msgf("publish proxy with name %s not found", publishProxyName)
				}
			}
			if _, ok := proxyMap.PublishProxies[publishProxyName]; !ok {
				pp, err := proxy.GetPublishProxy(p)
				if err != nil {
					log.Fatal().Msgf("error creating publish proxy: %v", err)
				}
				proxyMap.PublishProxies[publishProxyName] = pp
			}
			log.Info().Str("proxy_name", publishProxyName).Str("endpoint", p.Endpoint).Str("namespace", ns.Name).Msg("publish proxy enabled for channels in namespace")
			keepHeadersInContext = true
		}

		if subRefreshProxyName != "" {
			var p proxy.Config
			if subRefreshProxyName == config.UnifiedProxyName {
				globalProxyConfig.Endpoint = cfg.UnifiedProxy.SubRefreshEndpoint
				globalProxyConfig.Timeout = cfg.UnifiedProxy.SubRefreshTimeout
				p = globalProxyConfig
			} else {
				var ok bool
				p, ok = proxies[subRefreshProxyName]
				if !ok {
					log.Fatal().Msgf("sub refresh proxy not found: %s", subRefreshProxyName)
				}
			}
			if _, ok := proxyMap.SubRefreshProxies[subRefreshProxyName]; !ok {
				srp, err := proxy.GetSubRefreshProxy(p)
				if err != nil {
					log.Fatal().Msgf("error creating publish proxy: %v", err)
				}
				proxyMap.SubRefreshProxies[subRefreshProxyName] = srp
			}
			log.Info().Str("proxy_name", subRefreshProxyName).Str("endpoint", p.Endpoint).Str("namespace", ns.Name).Msg("sub refresh proxy enabled for channels in namespace")
			keepHeadersInContext = true
		}

		if subscribeStreamProxyName != "" {
			var p proxy.Config
			if subscribeStreamProxyName == config.UnifiedProxyName {
				globalProxyConfig.Endpoint = cfg.UnifiedProxy.SubscribeStreamEndpoint
				globalProxyConfig.Timeout = cfg.UnifiedProxy.SubscribeStreamTimeout
				p = globalProxyConfig
			} else {
				var ok bool
				p, ok = proxies[subscribeStreamProxyName]
				if !ok {
					log.Fatal().Msgf("subscribe stream proxy not found: %s", subscribeStreamProxyName)
				}
			}
			if strings.HasPrefix(p.Endpoint, "http") {
				log.Fatal().Msgf("error creating subscribe stream proxy %s only GRPC endpoints supported", subscribeStreamProxyName)
			}
			if _, ok := proxyMap.SubscribeStreamProxies[subscribeStreamProxyName]; !ok {
				sp, err := proxy.NewSubscribeStreamProxy(p)
				if err != nil {
					log.Fatal().Msgf("error creating subscribe proxy: %v", err)
				}
				proxyMap.SubscribeStreamProxies[subscribeStreamProxyName] = sp
			}
			log.Info().Str("proxy_name", subscribeStreamProxyName).Str("endpoint", p.Endpoint).Str("namespace", ns.Name).Msg("subscribe stream proxy enabled for channels in namespace")
			keepHeadersInContext = true
		}
	}

	rpcProxyName := cfg.RPC.WithoutNamespace.RpcProxyName
	if rpcProxyName != "" {
		var p proxy.Config
		if rpcProxyName == config.UnifiedProxyName {
			globalProxyConfig.Endpoint = cfg.UnifiedProxy.RPCEndpoint
			globalProxyConfig.Timeout = cfg.UnifiedProxy.RPCTimeout
			p = globalProxyConfig
		} else {
			var ok bool
			p, ok = proxies[rpcProxyName]
			if !ok {
				log.Fatal().Msgf("rpc proxy not found: %s", rpcProxyName)
			}
		}
		rp, err := proxy.GetRpcProxy(p)
		if err != nil {
			log.Fatal().Msgf("error creating rpc proxy: %v", err)
		}
		proxyMap.RpcProxies[rpcProxyName] = rp
		log.Info().Str("proxy_name", rpcProxyName).Str("endpoint", p.Endpoint).Msg("RPC proxy enabled for RPC calls without namespace")
		keepHeadersInContext = true
	}

	for _, ns := range cfg.RPC.Namespaces {
		rpcProxyName := ns.RpcProxyName
		if rpcProxyName != "" {
			var p proxy.Config
			if rpcProxyName == config.UnifiedProxyName {
				globalProxyConfig.Endpoint = cfg.UnifiedProxy.RPCEndpoint
				globalProxyConfig.Timeout = cfg.UnifiedProxy.RPCTimeout
				p = globalProxyConfig
			} else {
				var ok bool
				p, ok = proxies[rpcProxyName]
				if !ok {
					log.Fatal().Msgf("rpc proxy not found: %s", rpcProxyName)
				}
			}
			if _, ok := proxyMap.RpcProxies[rpcProxyName]; !ok {
				rp, err := proxy.GetRpcProxy(p)
				if err != nil {
					log.Fatal().Msgf("error creating rpc proxy: %v", err)
				}
				proxyMap.RpcProxies[rpcProxyName] = rp
			}
			log.Info().Str("proxy_name", rpcProxyName).Str("endpoint", p.Endpoint).Str("namespace", ns.Name).Msg("RPC proxy enabled for RPC calls in namespace")
			keepHeadersInContext = true
		}
	}

	return proxyMap, keepHeadersInContext
}
