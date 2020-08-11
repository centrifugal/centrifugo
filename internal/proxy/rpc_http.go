package proxy

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"

	"github.com/centrifugal/centrifuge"
	"github.com/centrifugal/centrifugo/internal/middleware"
)

// HTTPRPCProxy ...
type HTTPRPCProxy struct {
	httpCaller HTTPCaller
	options    Options
}

// RPCRequestHTTP ...
type RPCRequestHTTP struct {
	baseRequestHTTP

	UserID string          `json:"user"`
	Method string          `json:"method,omitempty"`
	Data   json.RawMessage `json:"data,omitempty"`
	// Base64Data to proxy binary data.
	Base64Data string `json:"b64data,omitempty"`
}

// NewHTTPRPCProxy ...
func NewHTTPRPCProxy(endpoint string, httpClient *http.Client, opts ...Option) *HTTPRPCProxy {
	options := &Options{}
	for _, opt := range opts {
		opt(options)
	}
	return &HTTPRPCProxy{
		httpCaller: NewHTTPCaller(endpoint, httpClient),
		options:    *options,
	}
}

// ProxyRPC ...
func (p *HTTPRPCProxy) ProxyRPC(ctx context.Context, req RPCRequest) (*RPCReply, error) {
	httpRequest := middleware.HeadersFromContext(ctx)

	rpcHTTPReq := RPCRequestHTTP{
		baseRequestHTTP: baseRequestHTTP{
			Protocol:  string(req.Transport.Protocol()),
			Encoding:  string(req.Transport.Encoding()),
			Transport: req.Transport.Name(),
			ClientID:  req.ClientID,
		},
		UserID: req.UserID,
		Method: req.Method,
	}

	if req.Transport.Encoding() == centrifuge.EncodingTypeJSON {
		rpcHTTPReq.Data = req.Data
	} else {
		rpcHTTPReq.Base64Data = base64.StdEncoding.EncodeToString(req.Data)
	}

	data, err := json.Marshal(rpcHTTPReq)
	if err != nil {
		return nil, err
	}

	respData, err := p.httpCaller.CallHTTP(ctx, getProxyHeader(httpRequest, p.options.ExtraHeaders), data)
	if err != nil {
		return nil, err
	}

	var res RPCReply
	err = json.Unmarshal(respData, &res)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

// Protocol ...
func (p *HTTPRPCProxy) Protocol() string {
	return "http"
}
