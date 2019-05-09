package proxy

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"strings"
)

// HTTPCaller ...
type HTTPCaller interface {
	CallHTTP(context.Context, http.Header, []byte) ([]byte, error)
}

type httpCaller struct {
	Endpoint   string
	HTTPClient *http.Client
}

// NewHTTPCaller ...
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
	respData, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading HTTP body: %v", err)
	}
	return respData, nil
}

func getProxyHeader(request *http.Request) http.Header {
	header := http.Header{}
	copyHeader(header, request.Header)
	if clientIP, _, err := net.SplitHostPort(request.RemoteAddr); err == nil {
		appendHostToXForwardHeader(header, clientIP)
	}
	return header
}

var proxyHeaders = []string{
	"User-Agent",
	"Cookie",
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

func appendHostToXForwardHeader(header http.Header, host string) {
	// If we aren't the first proxy retain prior
	// X-Forwarded-For information as a comma+space
	// separated list and fold multiple headers into one.
	if prior, ok := header["X-Forwarded-For"]; ok {
		host = strings.Join(prior, ", ") + ", " + host
	}
	header.Set("X-Forwarded-For", host)
}
