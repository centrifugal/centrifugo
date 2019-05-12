package proxy

import (
	"context"
	"encoding/json"

	"github.com/centrifugal/centrifuge"
)

// RPCRequest ...
type RPCRequest struct {
	Data      centrifuge.Raw
	UserID    string
	Transport centrifuge.Transport
}

// RPCResult ...
type RPCResult struct {
	Data       json.RawMessage        `json:"data"`
	Base64Data string                 `json:"b64data"`
	Error      *centrifuge.Error      `json:"error"`
	Disconnect *centrifuge.Disconnect `json:"disconnect"`
}

// RPCProxy allows to proxy RPC requests to application backend.
type RPCProxy interface {
	ProxyRPC(context.Context, RPCRequest) (*RPCResult, error)
}
