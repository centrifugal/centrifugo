package proxy

import (
	"context"
	"encoding/base64"

	"github.com/centrifugal/centrifuge"
)

// RPCHandlerConfig ...
type RPCHandlerConfig struct {
	Proxy RPCProxy
}

// RPCHandler ...
type RPCHandler struct {
	config RPCHandlerConfig
}

// NewRPCHandler ...
func NewRPCHandler(c RPCHandlerConfig) *RPCHandler {
	return &RPCHandler{
		config: c,
	}
}

// Handle RPC.
func (h *RPCHandler) Handle(node *centrifuge.Node, client *centrifuge.Client) func(e centrifuge.RPCEvent) centrifuge.RPCReply {
	return func(e centrifuge.RPCEvent) centrifuge.RPCReply {
		rpcResp, err := h.config.Proxy.ProxyRPC(context.Background(), RPCRequest{
			Data:      e.Data,
			UserID:    client.UserID(),
			Transport: client.Transport(),
		})
		if err != nil {
			node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error proxing RPC", map[string]interface{}{"error": err.Error()}))
			return centrifuge.RPCReply{
				Error: centrifuge.ErrorInternal,
			}
		}
		if rpcResp.Disconnect != nil {
			return centrifuge.RPCReply{
				Disconnect: rpcResp.Disconnect,
			}
		}
		if rpcResp.Error != nil {
			return centrifuge.RPCReply{
				Error: rpcResp.Error,
			}
		}

		var data []byte
		if client.Transport().Encoding() == "json" {
			data = rpcResp.Data
		} else {
			if rpcResp.Base64Data != "" {
				decodedData, err := base64.StdEncoding.DecodeString(rpcResp.Base64Data)
				if err != nil {
					node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error decoding base64 data", map[string]interface{}{"client": client.ID(), "error": err.Error()}))
					return centrifuge.RPCReply{
						Error: centrifuge.ErrorInternal,
					}
				}
				data = decodedData
			}
		}

		return centrifuge.RPCReply{
			Data: centrifuge.Raw(data),
		}
	}
}
