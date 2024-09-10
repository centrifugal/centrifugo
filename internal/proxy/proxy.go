package proxy

import (
	"encoding/json"
	"strings"

	"github.com/centrifugal/centrifugo/v5/internal/configtypes"
)

type Config = configtypes.Proxy

func getEncoding(useBase64 bool) string {
	if useBase64 {
		return "binary"
	}
	return "json"
}

func isHttpEndpoint(endpoint string) bool {
	return strings.HasPrefix(endpoint, "http://") || strings.HasPrefix(endpoint, "https://")
}

func GetConnectProxy(p Config) (ConnectProxy, error) {
	if isHttpEndpoint(p.Endpoint) {
		return NewHTTPConnectProxy(p)
	}
	return NewGRPCConnectProxy(p)
}

func GetRefreshProxy(p Config) (RefreshProxy, error) {
	if isHttpEndpoint(p.Endpoint) {
		return NewHTTPRefreshProxy(p)
	}
	return NewGRPCRefreshProxy(p)
}

func GetRpcProxy(p Config) (RPCProxy, error) {
	if isHttpEndpoint(p.Endpoint) {
		return NewHTTPRPCProxy(p)
	}
	return NewGRPCRPCProxy(p)
}

func GetSubRefreshProxy(p Config) (SubRefreshProxy, error) {
	if isHttpEndpoint(p.Endpoint) {
		return NewHTTPSubRefreshProxy(p)
	}
	return NewGRPCSubRefreshProxy(p)
}

func GetPublishProxy(p Config) (PublishProxy, error) {
	if isHttpEndpoint(p.Endpoint) {
		return NewHTTPPublishProxy(p)
	}
	return NewGRPCPublishProxy(p)
}

func GetSubscribeProxy(p Config) (SubscribeProxy, error) {
	if isHttpEndpoint(p.Endpoint) {
		return NewHTTPSubscribeProxy(p)
	}
	return NewGRPCSubscribeProxy(p)
}

func GetCacheEmptyProxy(p Config) (CacheEmptyProxy, error) {
	if isHttpEndpoint(p.Endpoint) {
		return NewHTTPCacheEmptyProxy(p)
	}
	return NewGRPCCacheEmptyProxy(p)
}

type PerCallData struct {
	Meta json.RawMessage
}
