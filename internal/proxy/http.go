package proxy

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/centrifugal/centrifugo/v4/internal/proxyproto"
)

type baseRequestHTTP struct {
	ClientID  string `json:"client"`
	Transport string `json:"transport"`
	Protocol  string `json:"protocol"`
	Encoding  string `json:"encoding"`
}

var httpEncoder = &proxyproto.JSONEncoder{}
var httpDecoder = &proxyproto.JSONDecoder{}

// DefaultMaxIdleConnsPerHost is a reasonable value for all HTTP clients.
const DefaultMaxIdleConnsPerHost = 255

// HTTPCaller is responsible for calling HTTP.
type HTTPCaller interface {
	CallHTTP(context.Context, string, http.Header, []byte) ([]byte, error)
}

type httpCaller struct {
	Endpoint   string
	HTTPClient *http.Client
}

// NewHTTPCaller creates new HTTPCaller.
func NewHTTPCaller(httpClient *http.Client) HTTPCaller {
	return &httpCaller{
		HTTPClient: httpClient,
	}
}

func proxyHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			MaxIdleConnsPerHost: DefaultMaxIdleConnsPerHost,
		},
		Timeout: timeout,
	}
}

type statusCodeError struct {
	Code int
}

func (e *statusCodeError) Error() string {
	return fmt.Sprintf("unexpected HTTP status code: %d", e.Code)
}

func (c *httpCaller) CallHTTP(ctx context.Context, endpoint string, header http.Header, reqData []byte) ([]byte, error) {
	req, err := http.NewRequest("POST", endpoint, bytes.NewReader(reqData))
	if err != nil {
		return nil, fmt.Errorf("error constructing HTTP request: %w", err)
	}
	req.Header = header
	resp, err := c.HTTPClient.Do(req.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("HTTP request error: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, &statusCodeError{resp.StatusCode}
	}
	respData, err := io.ReadAll(resp.Body)
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

func copyHeader(dst, src http.Header, extraHeaders []string) {
	for k, vv := range src {
		if !stringInSlice(k, extraHeaders) {
			continue
		}
		dst[k] = vv
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
