package proxy

import (
	"context"
	"time"

	"github.com/centrifugal/centrifugo/v4/internal/proxyproto"
)

// RefreshRequestHTTP ...
type RefreshRequestHTTP struct {
	baseRequestHTTP

	UserID string `json:"user"`
}

// HTTPRefreshProxy ...
type HTTPRefreshProxy struct {
	proxy      Proxy
	httpCaller HTTPCaller
}

var _ RefreshProxy = (*HTTPRefreshProxy)(nil)

// NewHTTPRefreshProxy ...
func NewHTTPRefreshProxy(p Proxy) (*HTTPRefreshProxy, error) {
	return &HTTPRefreshProxy{
		proxy:      p,
		httpCaller: NewHTTPCaller(proxyHTTPClient(time.Duration(p.Timeout))),
	}, nil
}

// ProxyRefresh proxies refresh to application backend.
func (p *HTTPRefreshProxy) ProxyRefresh(ctx context.Context, req *proxyproto.RefreshRequest) (*proxyproto.RefreshResponse, error) {
	data, err := httpEncoder.EncodeRefreshRequest(req)
	if err != nil {
		return nil, err
	}
	respData, err := p.httpCaller.CallHTTP(ctx, p.proxy.Endpoint, httpRequestHeaders(ctx, p.proxy), data)
	if err != nil {
		return nil, err
	}
	return httpDecoder.DecodeRefreshResponse(respData)
}

// Protocol ...
func (p *HTTPRefreshProxy) Protocol() string {
	return "http"
}

// UseBase64 ...
func (p *HTTPRefreshProxy) UseBase64() bool {
	return p.proxy.BinaryEncoding
}

// IncludeMeta ...
func (p *HTTPRefreshProxy) IncludeMeta() bool {
	return p.proxy.IncludeConnectionMeta
}
