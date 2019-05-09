package proxy

import (
	"context"
	"encoding/json"

	"github.com/centrifugal/centrifuge"
)

// RefreshRequest ...
type RefreshRequest struct {
	UserID    string
	Transport centrifuge.Transport
}

// RefreshCredentials ...
type RefreshCredentials struct {
	ExpireAt   int64           `json:"expire_at"`
	Info       json.RawMessage `json:"info"`
	Base64Info string          `json:"b64info"`
}

// RefreshResult ...
type RefreshResult struct {
	Credentials *RefreshCredentials    `json:"credentials"`
	Error       *centrifuge.Error      `json:"error"`
	Disconnect  *centrifuge.Disconnect `json:"disconnect"`
}

// RefreshProxy allows to send refresh requests.
type RefreshProxy interface {
	ProxyRefresh(context.Context, RefreshRequest) (*RefreshResult, error)
}
