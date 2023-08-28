package proxy

import (
	"context"
	"time"

	"github.com/centrifugal/centrifugo/v5/internal/proxyproto"
)

// HTTPUnsubscribeProxy ...
type HTTPUnsubscribeProxy struct {
	proxy      Proxy
	httpCaller HTTPCaller
}

var _ UnsubscribeProxy = (*HTTPUnsubscribeProxy)(nil)

// NewHTTPUnsubscribeProxy ...
func NewHTTPUnsubscribeProxy(p Proxy) (*HTTPUnsubscribeProxy, error) {
	return &HTTPUnsubscribeProxy{
		proxy:      p,
		httpCaller: NewHTTPCaller(proxyHTTPClient(time.Duration(p.Timeout))),
	}, nil
}

// ProxyUnsubscribe proxies Unsubscribe to application backend.
func (p *HTTPUnsubscribeProxy) ProxyUnsubscribe(ctx context.Context, req *proxyproto.UnsubscribeRequest) (*proxyproto.UnsubscribeResponse, error) {
	data, err := httpEncoder.EncodeUnsubscribeRequest(req)
	if err != nil {
		return nil, err
	}
	respData, err := p.httpCaller.CallHTTP(ctx, p.proxy.Endpoint, httpRequestHeaders(ctx, p.proxy), data)
	if err != nil {
		return nil, err
	}
	return httpDecoder.DecodeUnsubscribeResponse(respData)
}

// Protocol ...
func (p *HTTPUnsubscribeProxy) Protocol() string {
	return "http"
}

// UseBase64 ...
func (p *HTTPUnsubscribeProxy) UseBase64() bool {
	return p.proxy.BinaryEncoding
}

// IncludeMeta ...
func (p *HTTPUnsubscribeProxy) IncludeMeta() bool {
	return p.proxy.IncludeConnectionMeta
}
