package proxy

import (
	"context"
	"time"

	"github.com/centrifugal/centrifugo/v5/internal/proxyproto"
)

// HTTPRPCProxy ...
type HTTPRPCProxy struct {
	config     Config
	httpCaller HTTPCaller
}

var _ RPCProxy = (*HTTPRPCProxy)(nil)

// NewHTTPRPCProxy ...
func NewHTTPRPCProxy(p Config) (*HTTPRPCProxy, error) {
	return &HTTPRPCProxy{
		config:     p,
		httpCaller: NewHTTPCaller(proxyHTTPClient(time.Duration(p.Timeout))),
	}, nil
}

// ProxyRPC ...
func (p *HTTPRPCProxy) ProxyRPC(ctx context.Context, req *proxyproto.RPCRequest) (*proxyproto.RPCResponse, error) {
	data, err := httpEncoder.EncodeRPCRequest(req)
	if err != nil {
		return nil, err
	}
	respData, err := p.httpCaller.CallHTTP(ctx, p.config.Endpoint, httpRequestHeaders(ctx, p.config), data)
	if err != nil {
		return nil, err
	}
	return httpDecoder.DecodeRPCResponse(respData)
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
