package proxy

import (
	"context"
	"time"

	"github.com/centrifugal/centrifugo/v5/internal/proxyproto"
)

// HTTPSubscribeProxy ...
type HTTPSubscribeProxy struct {
	proxy      Proxy
	httpCaller HTTPCaller
}

var _ SubscribeProxy = (*HTTPSubscribeProxy)(nil)

// NewHTTPSubscribeProxy ...
func NewHTTPSubscribeProxy(p Proxy) (*HTTPSubscribeProxy, error) {
	return &HTTPSubscribeProxy{
		proxy:      p,
		httpCaller: NewHTTPCaller(proxyHTTPClient(time.Duration(p.Timeout))),
	}, nil
}

// ProxySubscribe proxies Subscribe to application backend.
func (p *HTTPSubscribeProxy) ProxySubscribe(ctx context.Context, req *proxyproto.SubscribeRequest) (*proxyproto.SubscribeResponse, error) {
	data, err := httpEncoder.EncodeSubscribeRequest(req)
	if err != nil {
		return nil, err
	}
	respData, err := p.httpCaller.CallHTTP(ctx, p.proxy.Endpoint, httpRequestHeaders(ctx, p.proxy), data)
	if err != nil {
		return nil, err
	}
	return httpDecoder.DecodeSubscribeResponse(respData)
}

// Protocol ...
func (p *HTTPSubscribeProxy) Protocol() string {
	return "http"
}

// UseBase64 ...
func (p *HTTPSubscribeProxy) UseBase64() bool {
	return p.proxy.BinaryEncoding
}

// IncludeMeta ...
func (p *HTTPSubscribeProxy) IncludeMeta() bool {
	return p.proxy.IncludeConnectionMeta
}
