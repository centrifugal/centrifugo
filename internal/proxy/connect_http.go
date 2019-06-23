package proxy

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
)

// ConnectRequestHTTP ...
type ConnectRequestHTTP struct {
	ClientID  string          `json:"client"`
	Transport string          `json:"transport"`
	Encoding  string          `json:"encoding"`
	Data      json.RawMessage `json:"data,omitempty"`
	// Base64Data to proxy protobuf data.
	Base64Data string `json:"b64data,omitempty"`
}

// HTTPConnectProxy ...
type HTTPConnectProxy struct {
	httpCaller HTTPCaller
}

// NewHTTPConnectProxy ...
func NewHTTPConnectProxy(endpoint string, httpClient *http.Client) *HTTPConnectProxy {
	return &HTTPConnectProxy{
		httpCaller: NewHTTPCaller(endpoint, httpClient),
	}
}

// ProxyConnect proxies connect control to application backend.
func (p *HTTPConnectProxy) ProxyConnect(ctx context.Context, req ConnectRequest) (*ConnectResult, error) {
	httpRequest := req.Transport.Info().Request

	connectHTTPReq := ConnectRequestHTTP{
		ClientID:  req.ClientID,
		Transport: req.Transport.Name(),
		Encoding:  string(req.Transport.Encoding()),
	}

	if req.Transport.Encoding() == "json" {
		connectHTTPReq.Data = json.RawMessage(req.Data)
	} else {
		connectHTTPReq.Base64Data = base64.StdEncoding.EncodeToString(req.Data)
	}

	data, err := json.Marshal(connectHTTPReq)
	if err != nil {
		return nil, err
	}

	respData, err := p.httpCaller.CallHTTP(ctx, getProxyHeader(httpRequest), data)
	if err != nil {
		return nil, err
	}
	var res ConnectResult
	err = json.Unmarshal(respData, &res)
	if err != nil {
		return nil, err
	}
	return &res, nil
}
