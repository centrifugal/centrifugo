package proxy

import (
	"context"

	"github.com/centrifugal/centrifuge"
)

// PublishRequest ...
type PublishRequest struct {
	ClientID  string
	UserID    string
	Channel   string
	Data      centrifuge.Raw
	Transport centrifuge.TransportInfo
}

type PublishResult struct{}

// PublishReply ...
type PublishReply struct {
	Result     *PublishResult         `json:"result"`
	Error      *centrifuge.Error      `json:"error"`
	Disconnect *centrifuge.Disconnect `json:"disconnect"`
}

// PublishProxy allows to send Publish requests.
type PublishProxy interface {
	ProxyPublish(context.Context, PublishRequest) (*PublishReply, error)
	// Protocol for metrics and logging.
	Protocol() string
}
