package proxy

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
)

type baseRequestHTTP struct {
	ClientID  string `json:"client"`
	Transport string `json:"transport"`
	Protocol  string `json:"protocol"`
	Encoding  string `json:"encoding"`
}

// DefaultMaxIdleConnsPerHost is a reasonable value for all HTTP clients.
const DefaultMaxIdleConnsPerHost = 255

// HTTPCaller is responsible for calling HTTP.
type HTTPCaller interface {
	CallHTTP(context.Context, http.Header, []byte) ([]byte, error)
}

type httpCaller struct {
	Endpoint   string
	HTTPClient *http.Client
}

// NewHTTPCaller creates new HTTPCaller.
func NewHTTPCaller(endpoint string, httpClient *http.Client) HTTPCaller {
	return &httpCaller{
		Endpoint:   endpoint,
		HTTPClient: httpClient,
	}
}

type statusCodeError struct {
	Code int
}

func (e *statusCodeError) Error() string {
	return fmt.Sprintf("unexpected HTTP status code: %d", e.Code)
}

func (c *httpCaller) CallHTTP(ctx context.Context, header http.Header, reqData []byte) ([]byte, error) {
	req, err := http.NewRequest("POST", c.Endpoint, bytes.NewReader(reqData))
	if err != nil {
		return nil, fmt.Errorf("error constructing HTTP request: %w", err)
	}
	req.Header = header
	resp, err := c.HTTPClient.Do(req.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("HTTP request error: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, &statusCodeError{resp.StatusCode}
	}
	respData, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading HTTP body: %w", err)
	}
	return respData, nil
}

func getProxyHeader(allHeader http.Header, extraHeaders []string) http.Header {
	proxyHeader := http.Header{}
	copyHeader(proxyHeader, allHeader, extraHeaders)
	proxyHeader.Set("Content-Type", "application/json")
	return proxyHeader
}

var defaultProxyHeaders = []string{
	"Origin",
	"User-Agent",
	"Cookie",
	"Authorization",
	"X-Forwarded-For",
	"X-Real-Ip",
	"X-Request-Id",
}

func copyHeader(dst, src http.Header, extraHeaders []string) {
	for k, vv := range src {
		if !stringInSlice(k, defaultProxyHeaders) && !stringInSlice(k, extraHeaders) {
			continue
		}
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

func stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}
