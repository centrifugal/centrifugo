package proxy

import (
	"context"

	"github.com/centrifugal/centrifugo/v3/internal/proxyproto"
)

// HTTPRPCProxy ...
type HTTPRPCProxy struct {
	endpoint   string
	httpCaller HTTPCaller
	config     Config
}

var _ RPCProxy = (*HTTPRPCProxy)(nil)

// NewHTTPRPCProxy ...
func NewHTTPRPCProxy(endpoint string, config Config) (*HTTPRPCProxy, error) {
	return &HTTPRPCProxy{
		endpoint:   endpoint,
		httpCaller: NewHTTPCaller(proxyHTTPClient(config.RPCTimeout)),
		config:     config,
	}, nil
}

// ProxyRPC ...
func (p *HTTPRPCProxy) ProxyRPC(ctx context.Context, req *proxyproto.RPCRequest) (*proxyproto.RPCResponse, error) {
	data, err := p.config.HTTPConfig.Encoder.EncodeRPCRequest(req)
	if err != nil {
		return nil, err
	}
	respData, err := p.httpCaller.CallHTTP(ctx, p.endpoint, httpRequestHeaders(ctx, p.config), data)
	if err != nil {
		return nil, err
	}
	return p.config.HTTPConfig.Decoder.DecodeRPCResponse(respData)
}

// Protocol ...
func (p *HTTPRPCProxy) Protocol() string {
	return "http"
}

// UseBase64 ...
func (p *HTTPRPCProxy) UseBase64() bool {
	return p.config.BinaryEncoding
}

// IncludeMeta ...
func (p *HTTPRPCProxy) IncludeMeta() bool {
	return p.config.IncludeConnectionMeta
}
