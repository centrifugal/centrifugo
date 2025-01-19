package proxy

import (
	"encoding/json"
	"strings"

	"github.com/centrifugal/centrifugo/v6/internal/configtypes"
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

func GetConnectProxy(name string, p Config) (ConnectProxy, error) {
	for i, header := range p.HttpHeaders {
		p.HttpHeaders[i] = strings.ToLower(header)
	}
	if isHttpEndpoint(p.Endpoint) {
		return NewHTTPConnectProxy(p)
	}
	return NewGRPCConnectProxy(name, p)
}

func GetRefreshProxy(name string, p Config) (RefreshProxy, error) {
	for i, header := range p.HttpHeaders {
		p.HttpHeaders[i] = strings.ToLower(header)
	}
	if isHttpEndpoint(p.Endpoint) {
		return NewHTTPRefreshProxy(p)
	}
	return NewGRPCRefreshProxy(name, p)
}

func GetRpcProxy(name string, p Config) (RPCProxy, error) {
	for i, header := range p.HttpHeaders {
		p.HttpHeaders[i] = strings.ToLower(header)
	}
	if isHttpEndpoint(p.Endpoint) {
		return NewHTTPRPCProxy(p)
	}
	return NewGRPCRPCProxy(name, p)
}

func GetSubRefreshProxy(name string, p Config) (SubRefreshProxy, error) {
	for i, header := range p.HttpHeaders {
		p.HttpHeaders[i] = strings.ToLower(header)
	}
	if isHttpEndpoint(p.Endpoint) {
		return NewHTTPSubRefreshProxy(p)
	}
	return NewGRPCSubRefreshProxy(name, p)
}

func GetPublishProxy(name string, p Config) (PublishProxy, error) {
	for i, header := range p.HttpHeaders {
		p.HttpHeaders[i] = strings.ToLower(header)
	}
	if isHttpEndpoint(p.Endpoint) {
		return NewHTTPPublishProxy(p)
	}
	return NewGRPCPublishProxy(name, p)
}

func GetSubscribeProxy(name string, p Config) (SubscribeProxy, error) {
	for i, header := range p.HttpHeaders {
		p.HttpHeaders[i] = strings.ToLower(header)
	}
	if isHttpEndpoint(p.Endpoint) {
		return NewHTTPSubscribeProxy(p)
	}
	return NewGRPCSubscribeProxy(name, p)
}

type PerCallData struct {
	Meta json.RawMessage
}
