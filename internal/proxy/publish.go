package proxy

import (
	"context"
	"encoding/json"

	"github.com/centrifugal/centrifuge"
)

// PublishRequest ...
type PublishRequest struct {
	ClientID  string
	UserID    string
	Channel   string
	Data      []byte
	Transport centrifuge.TransportInfo
}

// PublishResult ...
type PublishResult struct {
	Data       json.RawMessage `json:"data"`
	Base64Data string          `json:"b64data"`
}

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
