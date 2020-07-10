package proxy

import (
	"context"
	"encoding/json"

	"github.com/centrifugal/centrifuge"
)

// RPCRequest ...
type RPCRequest struct {
	Method    string
	Data      []byte
	ClientID  string
	UserID    string
	Transport centrifuge.TransportInfo
}

// RPCData ...
type RPCData struct {
	Data       json.RawMessage `json:"data"`
	Base64Data string          `json:"b64data"`
}

// RPCReply ...
type RPCReply struct {
	Result     *RPCData               `json:"result"`
	Error      *centrifuge.Error      `json:"error"`
	Disconnect *centrifuge.Disconnect `json:"disconnect"`
}

// RPCProxy allows to proxy RPC requests to application backend.
type RPCProxy interface {
	ProxyRPC(context.Context, RPCRequest) (*RPCReply, error)
	// Protocol for metrics and logging.
	Protocol() string
}
