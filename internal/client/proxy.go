package client

import (
	"strings"

	"github.com/centrifugal/centrifugo/v3/internal/proxy"
)

func isGRPC(endpoint string) bool {
	return strings.HasPrefix(endpoint, "grpc://")
}

func (h *Handler) getConnectProxy() (proxy.ConnectProxy, error) {
	endpoint := h.proxyConfig.ConnectEndpoint
	if isGRPC(endpoint) {
		return proxy.NewGRPCConnectProxy(
			endpoint,
			h.proxyConfig,
		)
	}
	return proxy.NewHTTPConnectProxy(
		endpoint,
		h.proxyConfig,
	)
}

func (h *Handler) getRefreshProxy() (proxy.RefreshProxy, error) {
	endpoint := h.proxyConfig.RefreshEndpoint
	if isGRPC(endpoint) {
		return proxy.NewGRPCRefreshProxy(
			endpoint,
			h.proxyConfig,
		)
	}
	return proxy.NewHTTPRefreshProxy(
		endpoint,
		h.proxyConfig,
	)
}

func (h *Handler) getRPCProxy() (proxy.RPCProxy, error) {
	endpoint := h.proxyConfig.RPCEndpoint
	if isGRPC(endpoint) {
		return proxy.NewGRPCRPCProxy(
			endpoint,
			h.proxyConfig,
		)
	}
	return proxy.NewHTTPRPCProxy(
		endpoint,
		h.proxyConfig,
	)
}

func (h *Handler) getPublishProxy() (proxy.PublishProxy, error) {
	endpoint := h.proxyConfig.PublishEndpoint
	if isGRPC(endpoint) {
		return proxy.NewGRPCPublishProxy(
			endpoint,
			h.proxyConfig,
		)
	}
	return proxy.NewHTTPPublishProxy(
		endpoint,
		h.proxyConfig,
	)
}

func (h *Handler) getSubscribeProxy() (proxy.SubscribeProxy, error) {
	endpoint := h.proxyConfig.SubscribeEndpoint
	if isGRPC(endpoint) {
		return proxy.NewGRPCSubscribeProxy(
			endpoint,
			h.proxyConfig,
		)
	}
	return proxy.NewHTTPSubscribeProxy(
		endpoint,
		h.proxyConfig,
	)
}
