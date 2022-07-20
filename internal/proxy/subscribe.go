package proxy

import (
	"context"

	"github.com/centrifugal/centrifugo/v4/internal/proxyproto"

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
	SubscribeOptions
}

// SubscribeReply ...
type SubscribeReply struct {
	Result     *SubscribeResult       `json:"result"`
	Error      *centrifuge.Error      `json:"error"`
	Disconnect *centrifuge.Disconnect `json:"disconnect"`
}

// SubscribeProxy allows to send Subscribe requests.
type SubscribeProxy interface {
	ProxySubscribe(context.Context, *proxyproto.SubscribeRequest) (*proxyproto.SubscribeResponse, error)
	// Protocol for metrics and logging.
	Protocol() string
	// UseBase64 for bytes in requests from Centrifugo to application backend.
	UseBase64() bool
	// IncludeMeta ...
	IncludeMeta() bool
}
