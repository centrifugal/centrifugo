package proxy

import (
	"context"
	"encoding/json"
	"net/http"
)

// HTTPRPCProxy ...
type HTTPRPCProxy struct {
	Endpoint   string
	HTTPClient *http.Client
	httpCaller HTTPCaller
}

// RPCRequestHTTP ...
type RPCRequestHTTP struct {
	UserID     string `json:"user_id"`
	Data       []byte `json:"data,omitempty"`
	Base64Data string `json:"b64data,omitempty"`
}

// NewHTTPRPCProxy ...
func NewHTTPRPCProxy(endpoint string, httpClient *http.Client) *HTTPRPCProxy {
	return &HTTPRPCProxy{
		Endpoint:   endpoint,
		HTTPClient: httpClient,
		httpCaller: NewHTTPCaller(endpoint, httpClient),
	}
}

// ProxyRPC ...
func (p *HTTPRPCProxy) ProxyRPC(ctx context.Context, req RPCRequest) (*RPCResult, error) {
	httpRequest := req.Transport.Info().Request

	rpcHTTPReq := RPCRequestHTTP{
		UserID: req.UserID,
		Data:   req.Data,
	}

	data, err := json.Marshal(rpcHTTPReq)
	if err != nil {
		return nil, err
	}

	respData, err := p.httpCaller.CallHTTP(ctx, getProxyHeader(httpRequest), data)
	if err != nil {
		return nil, err
	}
	var res RPCResult
	err = json.Unmarshal(respData, &res)
	if err != nil {
		return nil, err
	}
	return &res, nil
}
