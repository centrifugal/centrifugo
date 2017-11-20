package admin

import "github.com/centrifugal/centrifugo/libcentrifugo/raw"

// AdminInfoBody represents response to admin info command.
type AdminInfoBody struct {
	Data map[string]interface{} `json:"data"`
}

// AdminConnectResponse represents response to admin connect command.
type AdminConnectResponse struct {
	apiResponse
	Body bool `json:"body"`
}

// NewAdminConnectResponse initializes AdminConnectResponse.
func NewAdminConnectResponse(body bool) *AdminConnectResponse {
	return &AdminConnectResponse{
		apiResponse: apiResponse{
			Method: "connect",
		},
		Body: body,
	}
}

// AdminInfoResponse represents response to admin info command.
type AdminInfoResponse struct {
	apiResponse
	Body AdminInfoBody `json:"body"`
}

// NewAdminInfoResponse initializes AdminInfoResponse.
func NewAdminInfoResponse(body AdminInfoBody) *AdminInfoResponse {
	return &AdminInfoResponse{
		apiResponse: apiResponse{
			Method: "info",
		},
		Body: body,
	}
}

// AdminPingResponse represents response to admin ping command.
type AdminPingResponse struct {
	apiResponse
	Body string `json:"body"`
}

// NewAdminPingResponse initializes AdminPingResponse.
func NewAdminPingResponse(body string) *AdminPingResponse {
	return &AdminPingResponse{
		apiResponse: apiResponse{
			Method: "ping",
		},
		Body: body,
	}
}

// AdminMessageResponse is a new message for admin that watches.
type AdminMessageResponse struct {
	apiResponse
	Body raw.Raw `json:"body"`
}

// NewAdminMessageResponse initializes AdminMessageResponse.
func NewAdminMessageResponse(body raw.Raw) *AdminMessageResponse {
	return &AdminMessageResponse{
		apiResponse: apiResponse{
			Method: "message",
		},
		Body: body,
	}
}
