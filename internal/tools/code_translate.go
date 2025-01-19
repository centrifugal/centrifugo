package tools

import (
	"net/http"

	"github.com/centrifugal/centrifugo/v6/internal/configtypes"

	"github.com/centrifugal/centrifuge"
)

func ConnectErrorToToHTTPResponse(err error, transforms []configtypes.ConnectCodeToHTTPResponseTransform) (configtypes.TransformedConnectErrorHttpResponse, bool) {
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
	return configtypes.TransformedConnectErrorHttpResponse{
		StatusCode: http.StatusInternalServerError,
		Body:       http.StatusText(http.StatusInternalServerError),
	}, false
}
