package proxy

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
)

// HTTPRPCProxy ...
type HTTPRPCProxy struct {
	httpCaller HTTPCaller
}

// RPCRequestHTTP ...
type RPCRequestHTTP struct {
	Encoding string          `json:"encoding"`
	ClientID string          `json:"client"`
	UserID   string          `json:"user"`
	Data     json.RawMessage `json:"data,omitempty"`
	// Base64Data to proxy protobuf data.
	Base64Data string `json:"b64data,omitempty"`
}

// NewHTTPRPCProxy ...
func NewHTTPRPCProxy(endpoint string, httpClient *http.Client) *HTTPRPCProxy {
	return &HTTPRPCProxy{
		httpCaller: NewHTTPCaller(endpoint, httpClient),
	}
}

// ProxyRPC ...
func (p *HTTPRPCProxy) ProxyRPC(ctx context.Context, req RPCRequest) (*RPCReply, error) {
	httpRequest := req.Transport.Info().Request

	rpcHTTPReq := RPCRequestHTTP{
		Encoding: string(req.Transport.Encoding()),
		ClientID: req.ClientID,
		UserID:   req.UserID,
	}

	if req.Transport.Encoding() == "json" {
		rpcHTTPReq.Data = json.RawMessage(req.Data)
	} else {
		rpcHTTPReq.Base64Data = base64.StdEncoding.EncodeToString(req.Data)
	}

	data, err := json.Marshal(rpcHTTPReq)
	if err != nil {
		return nil, err
	}

	respData, err := p.httpCaller.CallHTTP(ctx, getProxyHeader(httpRequest), data)
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
