package proxy

import (
	"context"
	"time"

	"github.com/centrifugal/centrifugo/v5/internal/proxyproto"
)

// HTTPCacheEmptyProxy ...
type HTTPCacheEmptyProxy struct {
	config     Config
	httpCaller HTTPCaller
}

var _ CacheEmptyProxy = (*HTTPCacheEmptyProxy)(nil)

// NewHTTPCacheEmptyProxy ...
func NewHTTPCacheEmptyProxy(p Config) (*HTTPCacheEmptyProxy, error) {
	return &HTTPCacheEmptyProxy{
		config:     p,
		httpCaller: NewHTTPCaller(proxyHTTPClient(time.Duration(p.Timeout))),
	}, nil
}

// Protocol ...
func (p *HTTPCacheEmptyProxy) Protocol() string {
	return "http"
}

// UseBase64 ...
func (p *HTTPCacheEmptyProxy) UseBase64() bool {
	return p.config.BinaryEncoding
}

// NotifyCacheEmpty ...
func (p *HTTPCacheEmptyProxy) NotifyCacheEmpty(ctx context.Context, req *proxyproto.NotifyCacheEmptyRequest) (*proxyproto.NotifyCacheEmptyResponse, error) {
	data, err := httpEncoder.EncodeNotifyCacheEmptyRequest(req)
	if err != nil {
		return nil, err
	}
	respData, err := p.httpCaller.CallHTTP(ctx, p.config.Endpoint, httpRequestHeaders(ctx, p.config), data)
	if err != nil {
		return nil, err
	}
	return httpDecoder.DecodeNotifyCacheEmptyResponse(respData)
}
