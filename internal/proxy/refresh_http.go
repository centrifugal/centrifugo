package proxy

import (
	"context"

	"github.com/centrifugal/centrifugo/v5/internal/proxyproto"
)

// RefreshRequestHTTP ...
type RefreshRequestHTTP struct {
	baseRequestHTTP

	UserID string `json:"user"`
}

// HTTPRefreshProxy ...
type HTTPRefreshProxy struct {
	config     Config
	httpCaller HTTPCaller
}

var _ RefreshProxy = (*HTTPRefreshProxy)(nil)

// NewHTTPRefreshProxy ...
func NewHTTPRefreshProxy(p Config) (*HTTPRefreshProxy, error) {
	return &HTTPRefreshProxy{
		config:     p,
		httpCaller: NewHTTPCaller(proxyHTTPClient(p.Timeout)),
	}, nil
}

// ProxyRefresh proxies refresh to application backend.
func (p *HTTPRefreshProxy) ProxyRefresh(ctx context.Context, req *proxyproto.RefreshRequest) (*proxyproto.RefreshResponse, error) {
	data, err := httpEncoder.EncodeRefreshRequest(req)
	if err != nil {
		return nil, err
	}
	respData, err := p.httpCaller.CallHTTP(ctx, p.config.Endpoint, httpRequestHeaders(ctx, p.config), data)
	if err != nil {
		return nil, err
	}
	return httpDecoder.DecodeRefreshResponse(respData)
}

// Name ...
func (p *HTTPRefreshProxy) Name() string {
	return p.config.Name
}

// Protocol ...
func (p *HTTPRefreshProxy) Protocol() string {
	return "http"
}

// UseBase64 ...
func (p *HTTPRefreshProxy) UseBase64() bool {
	return p.config.BinaryEncoding
}

// IncludeMeta ...
func (p *HTTPRefreshProxy) IncludeMeta() bool {
	return p.config.IncludeConnectionMeta
}
