package proxy

import (
	"context"

	"github.com/centrifugal/centrifugo/v3/internal/proxyproto"
)

// HTTPConnectProxy ...
type HTTPConnectProxy struct {
	endpoint   string
	httpCaller HTTPCaller
	config     Config
}

var _ ConnectProxy = (*HTTPConnectProxy)(nil)

// NewHTTPConnectProxy ...
func NewHTTPConnectProxy(endpoint string, config Config) (*HTTPConnectProxy, error) {
	return &HTTPConnectProxy{
		endpoint:   endpoint,
		httpCaller: NewHTTPCaller(proxyHTTPClient(config.ConnectTimeout)),
		config:     config,
	}, nil
}

// Protocol ...
func (p *HTTPConnectProxy) Protocol() string {
	return "http"
}

// UseBase64 ...
func (p *HTTPConnectProxy) UseBase64() bool {
	return p.config.BinaryEncoding
}

// ProxyConnect proxies connect control to application backend.
func (p *HTTPConnectProxy) ProxyConnect(ctx context.Context, req *proxyproto.ConnectRequest) (*proxyproto.ConnectResponse, error) {
	data, err := p.config.HTTPConfig.Encoder.EncodeConnectRequest(req)
	if err != nil {
		return nil, err
	}
	respData, err := p.httpCaller.CallHTTP(ctx, p.endpoint, httpRequestHeaders(ctx, p.config), data)
	if err != nil {
		return nil, err
	}
	return p.config.HTTPConfig.Decoder.DecodeConnectResponse(respData)
}
