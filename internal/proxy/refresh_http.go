package proxy

import (
	"context"
	"fmt"

	"github.com/centrifugal/centrifugo/v6/internal/proxyproto"
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
	httpClient, err := proxyHTTPClient(p, "refresh_proxy")
	if err != nil {
		return nil, fmt.Errorf("error creating HTTP client: %w", err)
	}
	return &HTTPRefreshProxy{
		config:     p,
		httpCaller: NewHTTPCaller(httpClient),
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
		return transformRefreshResponse(err, p.config.HTTP.StatusToCodeTransforms)
	}
	return httpDecoder.DecodeRefreshResponse(respData)
}

// Name ...
func (p *HTTPRefreshProxy) Name() string {
	return "default"
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
