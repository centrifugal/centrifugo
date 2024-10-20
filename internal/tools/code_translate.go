package tools

import (
	"net/http"

	"github.com/centrifugal/centrifuge"
)

type ConnectCodeToHTTPStatus struct {
	Enabled    bool                               `mapstructure:"enabled" json:"enabled"`
	Translates []ConnectCodeToHTTPStatusTranslate `mapstructure:"translates" json:"translates"`
}

type ConnectCodeToHTTPStatusTranslate struct {
	Code         uint32 `mapstructure:"code" json:"code"`
	ToStatusCode int    `mapstructure:"to_status_code" json:"to_status_code"`
}

type TranslatedHttpResponse struct {
	Status int    `json:"status"`
	Body   []byte `json:"body"`
}

func TranslateToHTTPResponse(err error, translates []ConnectCodeToHTTPStatusTranslate) (TranslatedHttpResponse, bool) {
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
		for _, t := range translates {
			if t.Code != code {
				continue
			}
			return TranslatedHttpResponse{
				Status: t.ToStatusCode,
				Body:   []byte(body),
			}, true
		}
	}
	return TranslatedHttpResponse{
		Status: http.StatusInternalServerError,
		Body:   []byte(http.StatusText(http.StatusInternalServerError)),
	}, false
}
