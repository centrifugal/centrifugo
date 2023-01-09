package proxy

import (
	"context"
	"time"

	"github.com/centrifugal/centrifugo/v4/internal/proxyproto"
)

// SubRefreshRequestHTTP ...
type SubRefreshRequestHTTP struct {
	baseRequestHTTP

	UserID  string `json:"user"`
	Channel string `json:"channel"`
}

// HTTPSubRefreshProxy ...
type HTTPSubRefreshProxy struct {
	proxy      Proxy
	httpCaller HTTPCaller
}

var _ SubRefreshProxy = (*HTTPSubRefreshProxy)(nil)

// NewHTTPSubRefreshProxy ...
func NewHTTPSubRefreshProxy(p Proxy) (*HTTPSubRefreshProxy, error) {
	return &HTTPSubRefreshProxy{
		proxy:      p,
		httpCaller: NewHTTPCaller(proxyHTTPClient(time.Duration(p.Timeout))),
	}, nil
}

// ProxySubRefresh proxies refresh to application backend.
func (p *HTTPSubRefreshProxy) ProxySubRefresh(ctx context.Context, req *proxyproto.SubRefreshRequest) (*proxyproto.SubRefreshResponse, error) {
	data, err := httpEncoder.EncodeSubRefreshRequest(req)
	if err != nil {
		return nil, err
	}
	respData, err := p.httpCaller.CallHTTP(ctx, p.proxy.Endpoint, httpRequestHeaders(ctx, p.proxy), data)
	if err != nil {
		return nil, err
	}
	return httpDecoder.DecodeSubRefreshResponse(respData)
}

// Protocol ...
func (p *HTTPSubRefreshProxy) Protocol() string {
	return "http"
}

// UseBase64 ...
func (p *HTTPSubRefreshProxy) UseBase64() bool {
	return p.proxy.BinaryEncoding
}

// IncludeMeta ...
func (p *HTTPSubRefreshProxy) IncludeMeta() bool {
	return p.proxy.IncludeConnectionMeta
}
