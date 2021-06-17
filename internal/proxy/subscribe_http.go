package proxy

import (
	"context"

	"github.com/centrifugal/centrifugo/v3/internal/proxy/proxyproto"
)

// HTTPSubscribeProxy ...
type HTTPSubscribeProxy struct {
	endpoint   string
	httpCaller HTTPCaller
	config     Config
}

var _ SubscribeProxy = (*HTTPSubscribeProxy)(nil)

// NewHTTPSubscribeProxy ...
func NewHTTPSubscribeProxy(endpoint string, config Config) (*HTTPSubscribeProxy, error) {
	return &HTTPSubscribeProxy{
		endpoint:   endpoint,
		httpCaller: NewHTTPCaller(proxyHTTPClient(config.SubscribeTimeout)),
		config:     config,
	}, nil
}

// ProxySubscribe proxies Subscribe to application backend.
func (p *HTTPSubscribeProxy) ProxySubscribe(ctx context.Context, req *proxyproto.SubscribeRequest) (*proxyproto.SubscribeResponse, error) {
	data, err := p.config.HTTPConfig.Encoder.EncodeSubscribeRequest(req)
	if err != nil {
		return nil, err
	}
	respData, err := p.httpCaller.CallHTTP(ctx, p.endpoint, httpRequestHeaders(ctx, p.config), data)
	if err != nil {
		return nil, err
	}
	return p.config.HTTPConfig.Decoder.DecodeSubscribeResponse(respData)
}

// Protocol ...
func (p *HTTPSubscribeProxy) Protocol() string {
	return "http"
}

// UseBase64 ...
func (p *HTTPSubscribeProxy) UseBase64() bool {
	return p.config.BinaryEncoding
}
