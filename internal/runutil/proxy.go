package runutil

import (
	"strings"

	"github.com/centrifugal/centrifugo/v5/internal/client"
	"github.com/centrifugal/centrifugo/v5/internal/config"
	"github.com/centrifugal/centrifugo/v5/internal/proxy"

	"github.com/rs/zerolog/log"
)

func proxyMapConfig(cfg config.Config) (*client.ProxyMap, bool) {
	proxyMap := &client.ProxyMap{
		SubscribeProxies:       map[string]proxy.SubscribeProxy{},
		PublishProxies:         map[string]proxy.PublishProxy{},
		RpcProxies:             map[string]proxy.RPCProxy{},
		SubRefreshProxies:      map[string]proxy.SubRefreshProxy{},
		SubscribeStreamProxies: map[string]*proxy.SubscribeStreamProxy{},
	}
	proxyConfig := proxy.Config{
		ProxyCommon: cfg.Proxy.ProxyCommon,
	}

	connectEndpoint := cfg.Proxy.ConnectEndpoint
	connectTimeout := cfg.Proxy.ConnectTimeout
	refreshEndpoint := cfg.Proxy.RefreshEndpoint
	refreshTimeout := cfg.Proxy.RefreshTimeout
	rpcEndpoint := cfg.Proxy.RPCEndpoint
	rpcTimeout := cfg.Proxy.RPCTimeout
	subscribeEndpoint := cfg.Proxy.SubscribeEndpoint
	subscribeTimeout := cfg.Proxy.SubscribeTimeout
	publishEndpoint := cfg.Proxy.PublishEndpoint
	publishTimeout := cfg.Proxy.PublishTimeout
	subRefreshEndpoint := cfg.Proxy.SubRefreshEndpoint
	subRefreshTimeout := cfg.Proxy.SubRefreshTimeout
	proxyStreamSubscribeEndpoint := cfg.Proxy.StreamSubscribeEndpoint
	if strings.HasPrefix(proxyStreamSubscribeEndpoint, "http") {
		log.Fatal().Msg("error creating subscribe stream proxy: only GRPC endpoints supported")
	}
	proxyStreamSubscribeTimeout := cfg.Proxy.StreamSubscribeTimeout

	if connectEndpoint != "" {
		proxyConfig.Endpoint = connectEndpoint
		proxyConfig.Timeout = connectTimeout
		var err error
		proxyMap.ConnectProxy, err = proxy.GetConnectProxy(proxyConfig)
		if err != nil {
			log.Fatal().Msgf("error creating connect proxy: %v", err)
		}
		log.Info().Str("endpoint", connectEndpoint).Msg("connect proxy enabled")
	}

	if refreshEndpoint != "" {
		proxyConfig.Endpoint = refreshEndpoint
		proxyConfig.Timeout = refreshTimeout
		var err error
		proxyMap.RefreshProxy, err = proxy.GetRefreshProxy(proxyConfig)
		if err != nil {
			log.Fatal().Msgf("error creating refresh proxy: %v", err)
		}
		log.Info().Str("endpoint", refreshEndpoint).Msg("refresh proxy enabled")
	}

	if subscribeEndpoint != "" {
		proxyConfig.Endpoint = subscribeEndpoint
		proxyConfig.Timeout = subscribeTimeout
		sp, err := proxy.GetSubscribeProxy(proxyConfig)
		if err != nil {
			log.Fatal().Msgf("error creating subscribe proxy: %v", err)
		}
		proxyMap.SubscribeProxies[""] = sp
		log.Info().Str("endpoint", subscribeEndpoint).Msg("subscribe proxy enabled")
	}

	if publishEndpoint != "" {
		proxyConfig.Endpoint = publishEndpoint
		proxyConfig.Timeout = publishTimeout
		pp, err := proxy.GetPublishProxy(proxyConfig)
		if err != nil {
			log.Fatal().Msgf("error creating publish proxy: %v", err)
		}
		proxyMap.PublishProxies[""] = pp
		log.Info().Str("endpoint", publishEndpoint).Msg("publish proxy enabled")
	}

	if rpcEndpoint != "" {
		proxyConfig.Endpoint = rpcEndpoint
		proxyConfig.Timeout = rpcTimeout
		rp, err := proxy.GetRpcProxy(proxyConfig)
		if err != nil {
			log.Fatal().Msgf("error creating rpc proxy: %v", err)
		}
		proxyMap.RpcProxies[""] = rp
		log.Info().Str("endpoint", rpcEndpoint).Msg("RPC proxy enabled")
	}

	if subRefreshEndpoint != "" {
		proxyConfig.Endpoint = subRefreshEndpoint
		proxyConfig.Timeout = subRefreshTimeout
		srp, err := proxy.GetSubRefreshProxy(proxyConfig)
		if err != nil {
			log.Fatal().Msgf("error creating sub refresh proxy: %v", err)
		}
		proxyMap.SubRefreshProxies[""] = srp
		log.Info().Str("endpoint", subRefreshEndpoint).Msg("sub refresh proxy enabled")
	}

	if proxyStreamSubscribeEndpoint != "" {
		proxyConfig.Endpoint = proxyStreamSubscribeEndpoint
		proxyConfig.Timeout = proxyStreamSubscribeTimeout
		streamProxy, err := proxy.NewSubscribeStreamProxy(proxyConfig)
		if err != nil {
			log.Fatal().Msgf("error creating subscribe stream proxy: %v", err)
		}
		proxyMap.SubscribeStreamProxies[""] = streamProxy
		log.Info().Str("endpoint", proxyStreamSubscribeEndpoint).Msg("subscribe stream proxy enabled")
	}

	keepHeadersInContext := connectEndpoint != "" || refreshEndpoint != "" ||
		rpcEndpoint != "" || subscribeEndpoint != "" || publishEndpoint != "" ||
		subRefreshEndpoint != "" || proxyStreamSubscribeEndpoint != ""

	return proxyMap, keepHeadersInContext
}

func granularProxyMapConfig(cfg config.Config) (*client.ProxyMap, bool) {
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

	var keepHeadersInContext bool

	connectProxyName := cfg.ConnectProxyName
	if connectProxyName != "" {
		p, ok := proxies[connectProxyName]
		if !ok {
			log.Fatal().Msgf("connect proxy not found: %s", connectProxyName)
		}
		var err error
		proxyMap.ConnectProxy, err = proxy.GetConnectProxy(p)
		if err != nil {
			log.Fatal().Msgf("error creating connect proxy: %v", err)
		}
		keepHeadersInContext = true
	}
	refreshProxyName := cfg.RefreshProxyName
	if refreshProxyName != "" {
		p, ok := proxies[refreshProxyName]
		if !ok {
			log.Fatal().Msgf("refresh proxy not found: %s", refreshProxyName)
		}
		var err error
		proxyMap.RefreshProxy, err = proxy.GetRefreshProxy(p)
		if err != nil {
			log.Fatal().Msgf("error creating refresh proxy: %v", err)
		}
		keepHeadersInContext = true
	}
	subscribeProxyName := cfg.Channel.SubscribeProxyName
	if subscribeProxyName != "" {
		p, ok := proxies[subscribeProxyName]
		if !ok {
			log.Fatal().Msgf("subscribe proxy not found: %s", subscribeProxyName)
		}
		sp, err := proxy.GetSubscribeProxy(p)
		if err != nil {
			log.Fatal().Msgf("error creating subscribe proxy: %v", err)
		}
		proxyMap.SubscribeProxies[subscribeProxyName] = sp
		keepHeadersInContext = true
	}

	publishProxyName := cfg.Channel.PublishProxyName
	if publishProxyName != "" {
		p, ok := proxies[publishProxyName]
		if !ok {
			log.Fatal().Msgf("publish proxy not found: %s", publishProxyName)
		}
		pp, err := proxy.GetPublishProxy(p)
		if err != nil {
			log.Fatal().Msgf("error creating publish proxy: %v", err)
		}
		proxyMap.PublishProxies[publishProxyName] = pp
		keepHeadersInContext = true
	}

	subRefreshProxyName := cfg.Channel.SubRefreshProxyName
	if subRefreshProxyName != "" {
		p, ok := proxies[subRefreshProxyName]
		if !ok {
			log.Fatal().Msgf("sub refresh proxy not found: %s", subRefreshProxyName)
		}
		srp, err := proxy.GetSubRefreshProxy(p)
		if err != nil {
			log.Fatal().Msgf("error creating publish proxy: %v", err)
		}
		proxyMap.SubRefreshProxies[subRefreshProxyName] = srp
		keepHeadersInContext = true
	}

	subscribeStreamProxyName := cfg.Channel.SubscribeStreamProxyName
	if subscribeStreamProxyName != "" {
		p, ok := proxies[subscribeStreamProxyName]
		if !ok {
			log.Fatal().Msgf("subscribe stream proxy not found: %s", subscribeStreamProxyName)
		}
		if strings.HasPrefix(p.Endpoint, "http") {
			log.Fatal().Msgf("error creating subscribe stream proxy %s only GRPC endpoints supported", subscribeStreamProxyName)
		}
		sp, err := proxy.NewSubscribeStreamProxy(p)
		if err != nil {
			log.Fatal().Msgf("error creating subscribe proxy: %v", err)
		}
		proxyMap.SubscribeStreamProxies[subscribeProxyName] = sp
		keepHeadersInContext = true
	}

	for _, ns := range cfg.Channel.Namespaces {
		subscribeProxyName := ns.SubscribeProxyName
		publishProxyName := ns.PublishProxyName
		subRefreshProxyName := ns.SubRefreshProxyName
		subscribeStreamProxyName := ns.SubscribeStreamProxyName

		if subscribeProxyName != "" {
			p, ok := proxies[subscribeProxyName]
			if !ok {
				log.Fatal().Msgf("subscribe proxy not found: %s", subscribeProxyName)
			}
			sp, err := proxy.GetSubscribeProxy(p)
			if err != nil {
				log.Fatal().Msgf("error creating subscribe proxy: %v", err)
			}
			proxyMap.SubscribeProxies[subscribeProxyName] = sp
			keepHeadersInContext = true
		}

		if publishProxyName != "" {
			p, ok := proxies[publishProxyName]
			if !ok {
				log.Fatal().Msgf("publish proxy not found: %s", publishProxyName)
			}
			pp, err := proxy.GetPublishProxy(p)
			if err != nil {
				log.Fatal().Msgf("error creating publish proxy: %v", err)
			}
			proxyMap.PublishProxies[publishProxyName] = pp
			keepHeadersInContext = true
		}

		if subRefreshProxyName != "" {
			p, ok := proxies[subRefreshProxyName]
			if !ok {
				log.Fatal().Msgf("sub refresh proxy not found: %s", subRefreshProxyName)
			}
			srp, err := proxy.GetSubRefreshProxy(p)
			if err != nil {
				log.Fatal().Msgf("error creating sub refresh proxy: %v", err)
			}
			proxyMap.SubRefreshProxies[subRefreshProxyName] = srp
			keepHeadersInContext = true
		}

		if subscribeStreamProxyName != "" {
			p, ok := proxies[subscribeStreamProxyName]
			if !ok {
				log.Fatal().Msgf("subscribe stream proxy not found: %s", subscribeStreamProxyName)
			}
			if strings.HasPrefix(p.Endpoint, "http") {
				log.Fatal().Msgf("error creating subscribe stream proxy %s only GRPC endpoints supported", subscribeStreamProxyName)
			}
			ssp, err := proxy.NewSubscribeStreamProxy(p)
			if err != nil {
				log.Fatal().Msgf("error creating subscribe stream proxy: %v", err)
			}
			proxyMap.SubscribeStreamProxies[subscribeStreamProxyName] = ssp
			keepHeadersInContext = true
		}
	}

	rpcProxyName := cfg.RPC.RpcProxyName
	if rpcProxyName != "" {
		p, ok := proxies[rpcProxyName]
		if !ok {
			log.Fatal().Msgf("rpc proxy not found: %s", rpcProxyName)
		}
		rp, err := proxy.GetRpcProxy(p)
		if err != nil {
			log.Fatal().Msgf("error creating rpc proxy: %v", err)
		}
		proxyMap.RpcProxies[rpcProxyName] = rp
		keepHeadersInContext = true
	}

	for _, ns := range cfg.RPC.Namespaces {
		rpcProxyName := ns.RpcProxyName
		if rpcProxyName != "" {
			p, ok := proxies[rpcProxyName]
			if !ok {
				log.Fatal().Msgf("rpc proxy not found: %s", rpcProxyName)
			}
			rp, err := proxy.GetRpcProxy(p)
			if err != nil {
				log.Fatal().Msgf("error creating rpc proxy: %v", err)
			}
			proxyMap.RpcProxies[rpcProxyName] = rp
			keepHeadersInContext = true
		}
	}

	return proxyMap, keepHeadersInContext
}
