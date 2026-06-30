package proxy

import (
	"encoding/json"
	"strings"

	"github.com/centrifugal/centrifugo/v6/internal/configtypes"

	"github.com/rs/zerolog/log"
)

type Config = configtypes.Proxy

// prepareProxyConfig normalizes a proxy config's header allow lists and emits
// deprecation warnings. It must be called for every proxy before use.
//
// Note: a header name may legitimately appear in both http_headers and
// client_emulated_headers - e.g. an Authorization token a browser sends via
// headers emulation and a native client sends as a real transport header. The
// transport value is preferred, the emulated value is a fallback, and the
// backend validates it either way - so this is not flagged.
func prepareProxyConfig(name string, p *Config) {
	normalizeProxyHeaderNames(p)
	if p.HttpHeadersIncludeClientEmulated {
		// Deprecated pre-v6.9.0 behavior: http_headers also sources client-emulated
		// (forgeable) headers. Warn so its use is visible at startup.
		log.Warn().Str("proxy", name).Msg("http_headers_include_client_emulated is deprecated and will be removed in Centrifugo v7, list client-supplied headers in client_emulated_headers instead")
	}
}

// normalizeProxyHeaderNames lowercases configured header/metadata name allow
// lists so matching against incoming header names is case-insensitive.
func normalizeProxyHeaderNames(p *Config) {
	for i, header := range p.HttpHeaders {
		p.HttpHeaders[i] = strings.ToLower(header)
	}
	for i, header := range p.ClientEmulatedHeaders {
		p.ClientEmulatedHeaders[i] = strings.ToLower(header)
	}
}

// emulatedHeaderAllowList returns the set of header names that may be sourced
// from the client headers emulation map (the client connect frame). By default
// only client_emulated_headers are eligible; the http_headers allow list is
// reserved for transport-level headers (set on the connection request, not the
// client emulation map). The deprecated
// http_headers_include_client_emulated option restores the behavior before
// v6.9.0 of also sourcing http_headers names from emulation (removed in v7).
func emulatedHeaderAllowList(p Config) []string {
	if p.HttpHeadersIncludeClientEmulated {
		return append(append([]string{}, p.ClientEmulatedHeaders...), p.HttpHeaders...)
	}
	return p.ClientEmulatedHeaders
}

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
	prepareProxyConfig(name, &p)
	if isHttpEndpoint(p.Endpoint) {
		return NewHTTPConnectProxy(p)
	}
	return NewGRPCConnectProxy(name, p)
}

func GetRefreshProxy(name string, p Config) (RefreshProxy, error) {
	prepareProxyConfig(name, &p)
	if isHttpEndpoint(p.Endpoint) {
		return NewHTTPRefreshProxy(p)
	}
	return NewGRPCRefreshProxy(name, p)
}

func GetRpcProxy(name string, p Config) (RPCProxy, error) {
	prepareProxyConfig(name, &p)
	if isHttpEndpoint(p.Endpoint) {
		return NewHTTPRPCProxy(p)
	}
	return NewGRPCRPCProxy(name, p)
}

func GetSubRefreshProxy(name string, p Config) (SubRefreshProxy, error) {
	prepareProxyConfig(name, &p)
	if isHttpEndpoint(p.Endpoint) {
		return NewHTTPSubRefreshProxy(p)
	}
	return NewGRPCSubRefreshProxy(name, p)
}

func GetPublishProxy(name string, p Config) (PublishProxy, error) {
	prepareProxyConfig(name, &p)
	if isHttpEndpoint(p.Endpoint) {
		return NewHTTPPublishProxy(p)
	}
	return NewGRPCPublishProxy(name, p)
}

func GetSubscribeProxy(name string, p Config) (SubscribeProxy, error) {
	prepareProxyConfig(name, &p)
	if isHttpEndpoint(p.Endpoint) {
		return NewHTTPSubscribeProxy(p)
	}
	return NewGRPCSubscribeProxy(name, p)
}

func GetMapPublishProxy(name string, p Config) (MapPublishProxy, error) {
	prepareProxyConfig(name, &p)
	if isHttpEndpoint(p.Endpoint) {
		return NewHTTPMapPublishProxy(p)
	}
	return NewGRPCMapPublishProxy(name, p)
}

func GetMapRemoveProxy(name string, p Config) (MapRemoveProxy, error) {
	prepareProxyConfig(name, &p)
	if isHttpEndpoint(p.Endpoint) {
		return NewHTTPMapRemoveProxy(p)
	}
	return NewGRPCMapRemoveProxy(name, p)
}

func GetSharedPollRefreshProxy(name string, p Config) (SharedPollRefreshProxy, error) {
	prepareProxyConfig(name, &p)
	if isHttpEndpoint(p.Endpoint) {
		return NewHTTPSharedPollRefreshProxy(p)
	}
	return NewGRPCSharedPollRefreshProxy(name, p)
}

type PerCallData struct {
	Meta json.RawMessage
}
