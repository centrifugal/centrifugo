package proxy

import (
	"context"
	"encoding/base64"

	"github.com/centrifugal/centrifuge"
)

// ConnectHandlerConfig ...
type ConnectHandlerConfig struct {
	Proxy ConnectProxy
}

// ConnectHandler ...
type ConnectHandler struct {
	config ConnectHandlerConfig
}

// NewConnectHandler ...
func NewConnectHandler(c ConnectHandlerConfig) *ConnectHandler {
	return &ConnectHandler{
		config: c,
	}
}

// Handle returns connecting handler func.
func (h *ConnectHandler) Handle(node *centrifuge.Node) func(ctx context.Context, t centrifuge.Transport, e centrifuge.ConnectEvent) centrifuge.ConnectReply {
	return func(ctx context.Context, t centrifuge.Transport, e centrifuge.ConnectEvent) centrifuge.ConnectReply {
		if e.Token != "" {
			// As soon as token provided we do not try to proxy connect to application backend.
			return centrifuge.ConnectReply{
				Credentials: nil,
			}
		}

		connectResp, err := h.config.Proxy.ProxyConnect(context.Background(), ConnectRequest{
			Transport: t,
			Data:      e.Data,
		})
		if err != nil {
			node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error proxing connect", map[string]interface{}{"client": e.ClientID, "error": err.Error()}))
			return centrifuge.ConnectReply{
				Error: centrifuge.ErrorInternal,
			}
		}
		if connectResp.Disconnect != nil {
			return centrifuge.ConnectReply{
				Disconnect: connectResp.Disconnect,
			}
		}
		if connectResp.Error != nil {
			return centrifuge.ConnectReply{
				Error: connectResp.Error,
			}
		}

		var info []byte
		if t.Encoding() == "json" {
			info = connectResp.Credentials.Info
		} else {
			if connectResp.Credentials.Base64Info != "" {
				decodedInfo, err := base64.StdEncoding.DecodeString(connectResp.Credentials.Base64Info)
				if err != nil {
					node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error decoding base64 info", map[string]interface{}{"client": e.ClientID, "error": err.Error()}))
					return centrifuge.ConnectReply{
						Error: centrifuge.ErrorInternal,
					}
				}
				info = decodedInfo
			}
		}

		return centrifuge.ConnectReply{
			Credentials: &centrifuge.Credentials{
				UserID:   connectResp.Credentials.UserID,
				ExpireAt: connectResp.Credentials.ExpireAt,
				Info:     info,
			},
		}
	}
}
