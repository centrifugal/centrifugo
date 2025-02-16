package proxy

import (
	"context"
	"fmt"

	"github.com/centrifugal/centrifugo/v6/internal/proxyproto"
)

// HTTPSubscribeProxy ...
type HTTPSubscribeProxy struct {
	config     Config
	httpCaller HTTPCaller
}

var _ SubscribeProxy = (*HTTPSubscribeProxy)(nil)

// NewHTTPSubscribeProxy ...
func NewHTTPSubscribeProxy(p Config) (*HTTPSubscribeProxy, error) {
	httpClient, err := proxyHTTPClient(p, "subscribe_proxy")
	if err != nil {
		return nil, fmt.Errorf("error creating HTTP client: %w", err)
	}
	return &HTTPSubscribeProxy{
		config:     p,
		httpCaller: NewHTTPCaller(httpClient),
	}, nil
}

// ProxySubscribe proxies Subscribe to application backend.
func (p *HTTPSubscribeProxy) ProxySubscribe(ctx context.Context, req *proxyproto.SubscribeRequest) (*proxyproto.SubscribeResponse, error) {
	data, err := httpEncoder.EncodeSubscribeRequest(req)
	if err != nil {
		return nil, err
	}
	respData, err := p.httpCaller.CallHTTP(ctx, p.config.Endpoint, httpRequestHeaders(ctx, p.config), data)
	if err != nil {
		return transformSubscribeResponse(err, p.config.HTTP.StatusToCodeTransforms)
	}
	return httpDecoder.DecodeSubscribeResponse(respData)
}

// Protocol ...
func (p *HTTPSubscribeProxy) Protocol() string {
	return "http"
}

// UseBase64 ...
func (p *HTTPSubscribeProxy) UseBase64() bool {
	return p.config.BinaryEncoding
}

// IncludeMeta ...
func (p *HTTPSubscribeProxy) IncludeMeta() bool {
	return p.config.IncludeConnectionMeta
}
