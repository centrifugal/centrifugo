package proxy

import (
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/configtypes"
	"github.com/centrifugal/centrifugo/v6/internal/tools"

	"github.com/rs/zerolog/log"
)

type Config = configtypes.Proxy

// droppedEmulatedHeaderLogLimiter throttles the (rare) migration hint emitted
// when a client sends an emulated header that would have been forwarded before
// v6.9.0 (listed in http_headers) but is now dropped because it is not in
// client_emulated_headers. Single global limiter - at most one line per interval
// across all proxies.
var droppedEmulatedHeaderLogLimiter = tools.NewIntervalLimiter(5 * time.Minute)

// logDroppedEmulatedHeadersHint emits a one-off INFO hint listing every emulated
// header dropped for this request. It is only called when at least one header was
// dropped, and the body runs at most once per limiter interval, so it is
// effectively free on the hot path. allowedHeaders/allowedEmulatedHeaders are the
// lowercased http_headers / client_emulated_headers allow lists.
func logDroppedEmulatedHeadersHint(emulatedHeaders map[string]string, allowedHeaders, allowedEmulatedHeaders []string) {
	if !droppedEmulatedHeaderLogLimiter.Allow() {
		return
	}
	dropped := droppedEmulatedHeaderNames(emulatedHeaders, allowedHeaders, allowedEmulatedHeaders)
	if len(dropped) == 0 {
		return
	}
	// Informational only - deliberately no call to action. These names came from
	// the client, so this may be a missed migration OR an unexpected/malicious
	// client probing http_headers names; the operator must decide. Never imply the
	// names should be allow-listed: their values are client-controlled.
	log.Info().Strs("headers", dropped).Msg("client sent emulation headers matching http_headers but not client_emulated_headers - not forwarded to proxy (since v6.9.0); values are client-controlled, see headers emulation docs")
}

// droppedEmulatedHeaderNames returns the lowercased names of client-supplied
// emulated headers that are listed in http_headers but not client_emulated_headers
// - i.e. names that would have been forwarded before v6.9.0 and are now dropped.
func droppedEmulatedHeaderNames(emulatedHeaders map[string]string, allowedHeaders, allowedEmulatedHeaders []string) []string {
	var dropped []string
	for k := range emulatedHeaders {
		lk := strings.ToLower(k)
		if !slices.Contains(allowedEmulatedHeaders, lk) && slices.Contains(allowedHeaders, lk) {
			dropped = append(dropped, lk)
		}
	}
	return dropped
}

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
