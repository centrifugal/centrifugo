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
// emulated_headers. Single global limiter - at most one line per interval
// across all proxies.
var droppedEmulatedHeaderLogLimiter = tools.NewIntervalLimiter(time.Minute)

// logDroppedEmulatedHeadersHint emits a one-off INFO hint listing every emulated
// header dropped for this request. It is only called when at least one header was
// dropped, and the body runs at most once per limiter interval, so it is
// effectively free on the hot path. allowedHeaders/allowedEmulatedHeaders are the
// lowercased http_headers / emulated_headers allow lists.
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
	log.Info().Strs("headers", dropped).Msg("client sent emulation headers matching http_headers but not emulated_headers - not forwarded to proxy (since v6.9.0); values are client-controlled, see headers emulation docs")
}

// droppedEmulatedHeaderNames returns the lowercased names of client-supplied
// emulated headers that are listed in http_headers but not emulated_headers
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

// normalizeProxyHeaderNames lowercases configured header/metadata name allow
// lists so matching against incoming header names is case-insensitive. It must
// be called for every proxy before use.
func normalizeProxyHeaderNames(p *Config) {
	for i, header := range p.HttpHeaders {
		p.HttpHeaders[i] = strings.ToLower(header)
	}
	for i, header := range p.EmulatedHeaders {
		p.EmulatedHeaders[i] = strings.ToLower(header)
	}
}

// emulatedHeaderAllowList returns the set of header names that may be sourced
// from the client headers emulation map (the client connect frame) - i.e. the
// emulated_headers allow list. The http_headers allow list is reserved for
// transport-level headers and is never sourced from emulation.
//
// A header name may legitimately appear in both http_headers and emulated_headers
// (e.g. an Authorization token a browser sends via emulation and a native client
// sends as a real transport header): the transport value is preferred and the
// emulated value is used as a fallback, so this overlap is not flagged.
func emulatedHeaderAllowList(p Config) []string {
	return p.EmulatedHeaders
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
	normalizeProxyHeaderNames(&p)
	if isHttpEndpoint(p.Endpoint) {
		return NewHTTPConnectProxy(p)
	}
	return NewGRPCConnectProxy(name, p)
}

func GetRefreshProxy(name string, p Config) (RefreshProxy, error) {
	normalizeProxyHeaderNames(&p)
	if isHttpEndpoint(p.Endpoint) {
		return NewHTTPRefreshProxy(p)
	}
	return NewGRPCRefreshProxy(name, p)
}

func GetRpcProxy(name string, p Config) (RPCProxy, error) {
	normalizeProxyHeaderNames(&p)
	if isHttpEndpoint(p.Endpoint) {
		return NewHTTPRPCProxy(p)
	}
	return NewGRPCRPCProxy(name, p)
}

func GetSubRefreshProxy(name string, p Config) (SubRefreshProxy, error) {
	normalizeProxyHeaderNames(&p)
	if isHttpEndpoint(p.Endpoint) {
		return NewHTTPSubRefreshProxy(p)
	}
	return NewGRPCSubRefreshProxy(name, p)
}

func GetPublishProxy(name string, p Config) (PublishProxy, error) {
	normalizeProxyHeaderNames(&p)
	if isHttpEndpoint(p.Endpoint) {
		return NewHTTPPublishProxy(p)
	}
	return NewGRPCPublishProxy(name, p)
}

func GetSubscribeProxy(name string, p Config) (SubscribeProxy, error) {
	normalizeProxyHeaderNames(&p)
	if isHttpEndpoint(p.Endpoint) {
		return NewHTTPSubscribeProxy(p)
	}
	return NewGRPCSubscribeProxy(name, p)
}

func GetMapPublishProxy(name string, p Config) (MapPublishProxy, error) {
	normalizeProxyHeaderNames(&p)
	if isHttpEndpoint(p.Endpoint) {
		return NewHTTPMapPublishProxy(p)
	}
	return NewGRPCMapPublishProxy(name, p)
}

func GetMapRemoveProxy(name string, p Config) (MapRemoveProxy, error) {
	normalizeProxyHeaderNames(&p)
	if isHttpEndpoint(p.Endpoint) {
		return NewHTTPMapRemoveProxy(p)
	}
	return NewGRPCMapRemoveProxy(name, p)
}

func GetSharedPollRefreshProxy(name string, p Config) (SharedPollRefreshProxy, error) {
	normalizeProxyHeaderNames(&p)
	if isHttpEndpoint(p.Endpoint) {
		return NewHTTPSharedPollRefreshProxy(p)
	}
	return NewGRPCSharedPollRefreshProxy(name, p)
}

type PerCallData struct {
	Meta json.RawMessage
}
