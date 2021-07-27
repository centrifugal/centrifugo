package proxy

import (
	"context"

	"github.com/centrifugal/centrifugo/v3/internal/proxyproto"
)

// RefreshRequestHTTP ...
type RefreshRequestHTTP struct {
	baseRequestHTTP

	UserID string `json:"user"`
}

// HTTPRefreshProxy ...
type HTTPRefreshProxy struct {
	endpoint   string
	httpCaller HTTPCaller
	config     Config
}

var _ RefreshProxy = (*HTTPRefreshProxy)(nil)

// NewHTTPRefreshProxy ...
func NewHTTPRefreshProxy(endpoint string, config Config) (*HTTPRefreshProxy, error) {
	return &HTTPRefreshProxy{
		endpoint:   endpoint,
		httpCaller: NewHTTPCaller(proxyHTTPClient(config.RefreshTimeout)),
		config:     config,
	}, nil
}

// ProxyRefresh proxies refresh to application backend.
func (p *HTTPRefreshProxy) ProxyRefresh(ctx context.Context, req *proxyproto.RefreshRequest) (*proxyproto.RefreshResponse, error) {
	data, err := p.config.HTTPConfig.Encoder.EncodeRefreshRequest(req)
	if err != nil {
		return nil, err
	}
	respData, err := p.httpCaller.CallHTTP(ctx, p.endpoint, httpRequestHeaders(ctx, p.config), data)
	if err != nil {
		return nil, err
	}
	return p.config.HTTPConfig.Decoder.DecodeRefreshResponse(respData)
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
