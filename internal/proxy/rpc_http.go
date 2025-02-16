package proxy

import (
	"context"
	"fmt"

	"github.com/centrifugal/centrifugo/v6/internal/proxyproto"
)

// HTTPRPCProxy ...
type HTTPRPCProxy struct {
	config     Config
	httpCaller HTTPCaller
}

var _ RPCProxy = (*HTTPRPCProxy)(nil)

// NewHTTPRPCProxy ...
func NewHTTPRPCProxy(p Config) (*HTTPRPCProxy, error) {
	httpClient, err := proxyHTTPClient(p, "rpc_proxy")
	if err != nil {
		return nil, fmt.Errorf("error creating HTTP client: %w", err)
	}
	return &HTTPRPCProxy{
		config:     p,
		httpCaller: NewHTTPCaller(httpClient),
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
		return transformRPCResponse(err, p.config.HTTP.StatusToCodeTransforms)
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
