package client

import (
	"github.com/centrifugal/centrifugo/v3/internal/proxy"
)

// ProxyMap is a structure which contains all configured and already initialized
// proxies which can be used from inside client event handlers.
type ProxyMap struct {
	ConnectProxy     proxy.ConnectProxy
	RefreshProxy     proxy.RefreshProxy
	RpcProxies       map[string]proxy.RPCProxy
	PublishProxies   map[string]proxy.PublishProxy
	SubscribeProxies map[string]proxy.SubscribeProxy
}

func GetConnectProxy(p proxy.Proxy) (proxy.ConnectProxy, error) {
	if p.Type == "grpc" {
		return proxy.NewGRPCConnectProxy(p)
	}
	return proxy.NewHTTPConnectProxy(p)
}

func GetRefreshProxy(p proxy.Proxy) (proxy.RefreshProxy, error) {
	if p.Type == "grpc" {
		return proxy.NewGRPCRefreshProxy(p)
	}
	return proxy.NewHTTPRefreshProxy(p)
}

func GetRpcProxy(p proxy.Proxy) (proxy.RPCProxy, error) {
	if p.Type == "grpc" {
		return proxy.NewGRPCRPCProxy(p)
	}
	return proxy.NewHTTPRPCProxy(p)
}

func GetPublishProxy(p proxy.Proxy) (proxy.PublishProxy, error) {
	if p.Type == "grpc" {
		return proxy.NewGRPCPublishProxy(p)
	}
	return proxy.NewHTTPPublishProxy(p)
}

func GetSubscribeProxy(p proxy.Proxy) (proxy.SubscribeProxy, error) {
	if p.Type == "grpc" {
		return proxy.NewGRPCSubscribeProxy(p)
	}
	return proxy.NewHTTPSubscribeProxy(p)
}
