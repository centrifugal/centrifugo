package proxy

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// RefreshRequestHTTP ...
type RefreshRequestHTTP struct {
	ClientID  string `json:"client"`
	UserID    string `json:"user"`
	Transport string `json:"transport"`
	Protocol  string `json:"protocol"`
	Encoding  string `json:"encoding"`
}

// HTTPRefreshProxy ...
type HTTPRefreshProxy struct {
	httpCaller HTTPCaller
	summary    prometheus.Observer
	errCount   prometheus.Counter
}

// NewHTTPRefreshProxy ...
func NewHTTPRefreshProxy(endpoint string, httpClient *http.Client) *HTTPRefreshProxy {
	return &HTTPRefreshProxy{
		httpCaller: NewHTTPCaller(endpoint, httpClient),
		summary:    proxyCallDurationSummary.WithLabelValues("http", "refresh"),
		errCount:   proxyCallErrorCount.WithLabelValues("http", "refresh"),
	}
}

// ProxyRefresh proxies refresh to application backend.
func (p *HTTPRefreshProxy) ProxyRefresh(ctx context.Context, req RefreshRequest) (*RefreshReply, error) {
	httpRequest := req.Transport.Meta().Request

	refreshHTTPReq := RefreshRequestHTTP{
		ClientID:  req.ClientID,
		UserID:    req.UserID,
		Transport: req.Transport.Name(),
		Protocol:  string(req.Transport.Protocol()),
		Encoding:  string(req.Transport.Encoding()),
	}

	data, err := json.Marshal(refreshHTTPReq)
	if err != nil {
		return nil, err
	}

	started := time.Now()
	respData, err := p.httpCaller.CallHTTP(ctx, getProxyHeader(httpRequest), data)
	p.summary.Observe(time.Since(started).Seconds())
	if err != nil {
		p.errCount.Inc()
		return nil, err
	}
	var res RefreshReply
	err = json.Unmarshal(respData, &res)
	if err != nil {
		return nil, err
	}
	return &res, nil
}
