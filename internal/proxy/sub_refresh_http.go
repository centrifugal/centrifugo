package proxy

import (
	"context"
	"time"

	"github.com/centrifugal/centrifugo/v5/internal/proxyproto"
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
	return &HTTPSubRefreshProxy{
		config:     p,
		httpCaller: NewHTTPCaller(proxyHTTPClient(time.Duration(p.Timeout))),
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
	return p.config.BinaryEncoding
}

// IncludeMeta ...
func (p *HTTPSubRefreshProxy) IncludeMeta() bool {
	return p.config.IncludeConnectionMeta
}
