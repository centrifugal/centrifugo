package proxy

import (
	"context"
	"fmt"

	"github.com/centrifugal/centrifugo/v6/internal/proxyproto"
)

// SubRefreshRequestHTTP ...
type SubRefreshRequestHTTP struct {
	baseRequestHTTP

	UserID  string `json:"user"`
	Channel string `json:"channel"`
}

// HTTPSubRefreshProxy ...
type HTTPSubRefreshProxy struct {
	config     Config
	httpCaller HTTPCaller
}

var _ SubRefreshProxy = (*HTTPSubRefreshProxy)(nil)

// NewHTTPSubRefreshProxy ...
func NewHTTPSubRefreshProxy(p Config) (*HTTPSubRefreshProxy, error) {
	httpClient, err := proxyHTTPClient(p, "sub_refresh_proxy")
	if err != nil {
		return nil, fmt.Errorf("error creating HTTP client: %w", err)
	}
	return &HTTPSubRefreshProxy{
		config:     p,
		httpCaller: NewHTTPCaller(httpClient),
	}, nil
}

// ProxySubRefresh proxies refresh to application backend.
func (p *HTTPSubRefreshProxy) ProxySubRefresh(ctx context.Context, req *proxyproto.SubRefreshRequest) (*proxyproto.SubRefreshResponse, error) {
	data, err := httpEncoder.EncodeSubRefreshRequest(req)
	if err != nil {
		return nil, err
	}
	respData, err := p.httpCaller.CallHTTP(ctx, p.config.Endpoint, httpRequestHeaders(ctx, p.config), data)
	if err != nil {
		return transformSubRefreshResponse(err, p.config.HTTP.StatusToCodeTransforms)
	}
	return httpDecoder.DecodeSubRefreshResponse(respData)
}

// Protocol ...
func (p *HTTPSubRefreshProxy) Protocol() string {
	return "http"
}

// UseBase64 ...
func (p *HTTPSubRefreshProxy) UseBase64() bool {
	return p.config.BinaryEncoding
}

// IncludeMeta ...
func (p *HTTPSubRefreshProxy) IncludeMeta() bool {
	return p.config.IncludeConnectionMeta
}
