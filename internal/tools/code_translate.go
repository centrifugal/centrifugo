package tools

import (
	"net/http"

	"github.com/centrifugal/centrifuge"
)

type ConnectCodeToHTTPStatus struct {
	Enabled    bool                               `mapstructure:"enabled" json:"enabled"`
	Transforms []ConnectCodeToHTTPStatusTransform `mapstructure:"transforms" json:"transforms"`
}

type ConnectCodeToHTTPStatusTransform struct {
	Code uint32                              `mapstructure:"code" json:"code"`
	To   TransformedConnectErrorHttpResponse `mapstructure:"to" json:"to"`
}

type TransformedConnectErrorHttpResponse struct {
	Status int    `mapstructure:"status_code" json:"status_code"`
	Body   string `mapstructure:"body" json:"body"`
}

func ConnectErrorToToHTTPResponse(err error, transforms []ConnectCodeToHTTPStatusTransform) (TransformedConnectErrorHttpResponse, bool) {
	var code uint32
	var body string
	switch t := err.(type) {
	case *centrifuge.Disconnect:
		code = t.Code
		body = t.Reason
	case centrifuge.Disconnect:
		code = t.Code
		body = t.Reason
	case *centrifuge.Error:
		code = t.Code
		body = t.Message
	default:
	}
	if code > 0 {
		for _, t := range transforms {
			if t.Code != code {
				continue
			}
			if t.To.Body == "" {
				t.To.Body = body
			}
			return t.To, true
		}
	}
	return TransformedConnectErrorHttpResponse{
		Status: http.StatusInternalServerError,
		Body:   http.StatusText(http.StatusInternalServerError),
	}, false
}
