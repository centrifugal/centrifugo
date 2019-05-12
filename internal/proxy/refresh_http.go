package proxy

import (
	"context"
	"encoding/json"
	"net/http"
)

// RefreshRequestHTTP ...
type RefreshRequestHTTP struct {
	UserID string `json:"user_id"`
}

// HTTPRefreshProxy ...
type HTTPRefreshProxy struct {
	httpCaller HTTPCaller
}

// NewHTTPRefreshProxy ...
func NewHTTPRefreshProxy(endpoint string, httpClient *http.Client) *HTTPRefreshProxy {
	return &HTTPRefreshProxy{
		httpCaller: NewHTTPCaller(endpoint, httpClient),
	}
}

// ProxyRefresh proxies refresh to application backend.
func (p *HTTPRefreshProxy) ProxyRefresh(ctx context.Context, req RefreshRequest) (*RefreshResult, error) {
	httpRequest := req.Transport.Info().Request

	refreshHTTPReq := RefreshRequestHTTP{
		UserID: req.UserID,
	}

	data, err := json.Marshal(refreshHTTPReq)
	if err != nil {
		return nil, err
	}

	respData, err := p.httpCaller.CallHTTP(ctx, getProxyHeader(httpRequest), data)
	if err != nil {
		return nil, err
	}
	var res RefreshResult
	err = json.Unmarshal(respData, &res)
	if err != nil {
		return nil, err
	}
	return &res, nil
}
