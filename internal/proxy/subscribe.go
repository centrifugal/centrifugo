package proxy

import (
	"context"
	"encoding/json"

	"github.com/centrifugal/centrifuge"
)

// SubscribeRequest ...
type SubscribeRequest struct {
	ClientID  string
	UserID    string
	Channel   string
	Transport centrifuge.TransportInfo
	Token     string
}

// SubscribeResult ...
type SubscribeResult struct {
	Info       json.RawMessage `json:"info"`
	Base64Info string          `json:"b64info"`
}

// SubscribeReply ...
type SubscribeReply struct {
	Result     *SubscribeResult       `json:"result"`
	Error      *centrifuge.Error      `json:"error"`
	Disconnect *centrifuge.Disconnect `json:"disconnect"`
}

// SubscribeProxy allows to send Subscribe requests.
type SubscribeProxy interface {
	ProxySubscribe(context.Context, SubscribeRequest) (*SubscribeReply, error)
	// Protocol for metrics and logging.
	Protocol() string
}
