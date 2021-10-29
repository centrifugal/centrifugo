package proxy

import (
	"context"
	"time"

	"github.com/centrifugal/centrifugo/v3/internal/proxyproto"
)

// HTTPConnectProxy ...
type HTTPConnectProxy struct {
	proxy      Proxy
	httpCaller HTTPCaller
}

var _ ConnectProxy = (*HTTPConnectProxy)(nil)

// NewHTTPConnectProxy ...
func NewHTTPConnectProxy(p Proxy) (*HTTPConnectProxy, error) {
	return &HTTPConnectProxy{
		proxy:      p,
		httpCaller: NewHTTPCaller(proxyHTTPClient(time.Duration(p.Timeout))),
	}, nil
}

// Protocol ...
func (p *HTTPConnectProxy) Protocol() string {
	return "http"
}

// UseBase64 ...
func (p *HTTPConnectProxy) UseBase64() bool {
	return p.proxy.BinaryEncoding
}

// ProxyConnect proxies connect control to application backend.
func (p *HTTPConnectProxy) ProxyConnect(ctx context.Context, req *proxyproto.ConnectRequest) (*proxyproto.ConnectResponse, error) {
	data, err := httpEncoder.EncodeConnectRequest(req)
	if err != nil {
		return nil, err
	}
	respData, err := p.httpCaller.CallHTTP(ctx, p.proxy.Endpoint, httpRequestHeaders(ctx, p.proxy), data)
	if err != nil {
		return nil, err
	}
	return httpDecoder.DecodeConnectResponse(respData)
}
