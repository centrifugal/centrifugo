package proxy

import (
	"context"

	"github.com/centrifugal/centrifugo/v5/internal/proxyproto"
)

// HTTPConnectProxy ...
type HTTPConnectProxy struct {
	config     Config
	httpCaller HTTPCaller
}

var _ ConnectProxy = (*HTTPConnectProxy)(nil)

// NewHTTPConnectProxy ...
func NewHTTPConnectProxy(p Config) (*HTTPConnectProxy, error) {
	return &HTTPConnectProxy{
		config:     p,
		httpCaller: NewHTTPCaller(proxyHTTPClient(p.Timeout)),
	}, nil
}

// Protocol ...
func (p *HTTPConnectProxy) Protocol() string {
	return "http"
}

func (p *HTTPConnectProxy) Name() string {
	return p.config.Name
}

// UseBase64 ...
func (p *HTTPConnectProxy) UseBase64() bool {
	return p.config.BinaryEncoding
}

// ProxyConnect proxies connect control to application backend.
func (p *HTTPConnectProxy) ProxyConnect(ctx context.Context, req *proxyproto.ConnectRequest) (*proxyproto.ConnectResponse, error) {
	data, err := httpEncoder.EncodeConnectRequest(req)
	if err != nil {
		return nil, err
	}
	respData, err := p.httpCaller.CallHTTP(ctx, p.config.Endpoint, httpRequestHeaders(ctx, p.config), data)
	if err != nil {
		return nil, err
	}
	return httpDecoder.DecodeConnectResponse(respData)
}
