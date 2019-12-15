package proxy

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
)

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

func (c *httpCaller) CallHTTP(ctx context.Context, header http.Header, reqData []byte) ([]byte, error) {
	req, err := http.NewRequest("POST", c.Endpoint, bytes.NewReader(reqData))
	if err != nil {
		return nil, fmt.Errorf("error constructing HTTP request: %v", err)
	}
	req.Header = header
	resp, err := c.HTTPClient.Do(req.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("HTTP request error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected HTTP status code: %d", resp.StatusCode)
	}
	respData, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading HTTP body: %v", err)
	}
	return respData, nil
}

func getProxyHeader(allHeader http.Header) http.Header {
	proxyHeader := http.Header{}
	copyHeader(proxyHeader, allHeader)
	return proxyHeader
}

var proxyHeaders = []string{
	"User-Agent",
	"Cookie",
	"Authorization",
	"X-Forwarded-For",
	"X-Real-Ip",
	"X-Request-Id",
}

func copyHeader(dst, src http.Header) {
	for k, vv := range src {
		if !stringInSlice(k, proxyHeaders) {
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
