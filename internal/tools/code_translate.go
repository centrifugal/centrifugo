package tools

import (
	"errors"
	"net/http"

	"github.com/centrifugal/centrifuge"
)

type TransformDisconnect struct {
	Code   uint32 `mapstructure:"code" json:"code"`
	Reason string `mapstructure:"reason" json:"reason"`
}

type UniConnectCodeToDisconnectTransform struct {
	Code uint32              `mapstructure:"code" json:"code"`
	To   TransformDisconnect `mapstructure:"to" json:"to"`
}

func (t UniConnectCodeToDisconnectTransform) Validate() error {
	if t.Code == 0 {
		return errors.New("no code specified")
	}
	if t.To.Code == 0 {
		return errors.New("no disconnect code specified")
	}
	if !IsASCII(t.To.Reason) {
		return errors.New("disconnect reason must be ASCII")
	}
	return nil
}

type ConnectCodeToHTTPStatus struct {
	Enabled    bool                               `mapstructure:"enabled" json:"enabled"`
	Transforms []ConnectCodeToHTTPStatusTransform `mapstructure:"transforms" json:"transforms"`
}

type ConnectCodeToHTTPStatusTransform struct {
	Code uint32                              `mapstructure:"code" json:"code"`
	To   TransformedConnectErrorHttpResponse `mapstructure:"to" json:"to"`
}

func (t ConnectCodeToHTTPStatusTransform) Validate() error {
	if t.Code == 0 {
		return errors.New("no code specified")
	}
	if t.To.Status == 0 {
		return errors.New("no status_code specified")
	}
	return nil
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
