package tools

import (
	"net/http"

	"github.com/centrifugal/centrifuge"
)

type ConnectCodeToHTTPStatusTranslates struct {
	Enabled    bool
	Translates []ConnectCodeToHTTPStatusTranslate
}

type ConnectCodeToHTTPStatusTranslate struct {
	Code         uint32
	ToStatusCode int
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
