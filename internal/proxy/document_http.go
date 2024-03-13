package proxy

import (
	"context"
	"time"

	"github.com/centrifugal/centrifugo/v5/internal/proxyproto"
)

// HTTPDocumentProxy ...
type HTTPDocumentProxy struct {
	config     Config
	httpCaller HTTPCaller
}

var _ DocumentProxy = (*HTTPDocumentProxy)(nil)

// NewHTTPDocumentProxy ...
func NewHTTPDocumentProxy(p Config) (*HTTPDocumentProxy, error) {
	return &HTTPDocumentProxy{
		config:     p,
		httpCaller: NewHTTPCaller(proxyHTTPClient(time.Duration(p.Timeout))),
	}, nil
}

// Protocol ...
func (p *HTTPDocumentProxy) Protocol() string {
	return "http"
}

// UseBase64 ...
func (p *HTTPDocumentProxy) UseBase64() bool {
	return p.config.BinaryEncoding
}

// LoadDocuments ...
func (p *HTTPDocumentProxy) LoadDocuments(ctx context.Context, req *proxyproto.LoadDocumentsRequest) (*proxyproto.LoadDocumentsResponse, error) {
	data, err := httpEncoder.EncodeLoadDocumentsRequest(req)
	if err != nil {
		return nil, err
	}
	respData, err := p.httpCaller.CallHTTP(ctx, p.config.Endpoint, httpRequestHeaders(ctx, p.config), data)
	if err != nil {
		return nil, err
	}
	return httpDecoder.DecodeLoadDocumentsResponse(respData)
}
