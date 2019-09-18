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
func (h *RPCHandler) Handle(ctx context.Context, node *centrifuge.Node, client *centrifuge.Client) func(e centrifuge.RPCEvent) centrifuge.RPCReply {
	return func(e centrifuge.RPCEvent) centrifuge.RPCReply {
		rpcRep, err := h.config.Proxy.ProxyRPC(ctx, RPCRequest{
			Data:      e.Data,
			ClientID:  client.ID(),
			UserID:    client.UserID(),
			Transport: client.Transport(),
		})
		if err != nil {
			node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error proxying RPC", map[string]interface{}{"error": err.Error()}))
			return centrifuge.RPCReply{
				Error: centrifuge.ErrorInternal,
			}
		}
		if rpcRep.Disconnect != nil {
			return centrifuge.RPCReply{
				Disconnect: rpcRep.Disconnect,
			}
		}
		if rpcRep.Error != nil {
			return centrifuge.RPCReply{
				Error: rpcRep.Error,
			}
		}

		rpcData := rpcRep.Result
		var data []byte
		if rpcData != nil {
			if client.Transport().Encoding() == "json" {
				data = rpcData.Data
			} else {
				if rpcData.Base64Data != "" {
					decodedData, err := base64.StdEncoding.DecodeString(rpcData.Base64Data)
					if err != nil {
						node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error decoding base64 data", map[string]interface{}{"client": client.ID(), "error": err.Error()}))
						return centrifuge.RPCReply{
							Error: centrifuge.ErrorInternal,
						}
					}
					data = decodedData
				}
			}
		}

		return centrifuge.RPCReply{
			Data: centrifuge.Raw(data),
		}
	}
}
