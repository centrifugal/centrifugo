package proxy

import (
	"context"
	"encoding/json"

	"github.com/centrifugal/centrifuge"
)

// RefreshRequest ...
type RefreshRequest struct {
	ClientID  string
	UserID    string
	Transport centrifuge.TransportInfo
}

// RefreshCredentials ...
type RefreshCredentials struct {
	Expired    bool            `json:"expired"`
	ExpireAt   int64           `json:"expire_at"`
	Info       json.RawMessage `json:"info"`
	Base64Info string          `json:"b64info"`
}

// RefreshReply ...
type RefreshReply struct {
	Result     *RefreshCredentials    `json:"result"`
	Error      *centrifuge.Error      `json:"error"`
	Disconnect *centrifuge.Disconnect `json:"disconnect"`
}

// RefreshProxy allows to send refresh requests.
type RefreshProxy interface {
	ProxyRefresh(context.Context, RefreshRequest) (*RefreshReply, error)
	// Protocol for metrics and logging.
	Protocol() string
}
