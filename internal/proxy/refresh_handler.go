package proxy

import (
	"context"
	"encoding/base64"
	"time"

	"github.com/centrifugal/centrifuge"
)

// RefreshHandlerConfig ...
type RefreshHandlerConfig struct {
	Proxy RefreshProxy
}

// RefreshHandler ...
type RefreshHandler struct {
	config RefreshHandlerConfig
}

// NewRefreshHandler ...
func NewRefreshHandler(c RefreshHandlerConfig) *RefreshHandler {
	return &RefreshHandler{
		config: c,
	}
}

// Handle refresh.
func (h *RefreshHandler) Handle(node *centrifuge.Node) func(context.Context, *centrifuge.Client, centrifuge.RefreshEvent) centrifuge.RefreshReply {
	return func(ctx context.Context, client *centrifuge.Client, e centrifuge.RefreshEvent) centrifuge.RefreshReply {
		refreshRep, err := h.config.Proxy.ProxyRefresh(context.Background(), RefreshRequest{
			ClientID:  client.ID(),
			UserID:    client.UserID(),
			Transport: client.Transport(),
		})
		if err != nil {
			node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error proxying refresh", map[string]interface{}{"error": err.Error()}))
			// In case of an error give connection one more minute and then try to check again.
			return centrifuge.RefreshReply{
				ExpireAt: time.Now().Unix() + 60,
			}
		}

		credentials := refreshRep.Result
		if credentials == nil {
			// User will be disconnected.
			node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "no refresh credentials found", map[string]interface{}{}))
			return centrifuge.RefreshReply{}
		}

		var info []byte
		if client.Transport().Encoding() == "json" {
			info = credentials.Info
		} else {
			if credentials.Base64Info != "" {
				decodedInfo, err := base64.StdEncoding.DecodeString(credentials.Base64Info)
				if err != nil {
					node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error decoding base64 info", map[string]interface{}{"client": client.ID(), "error": err.Error()}))
					return centrifuge.RefreshReply{}
				}
				info = decodedInfo
			}
		}

		return centrifuge.RefreshReply{
			ExpireAt: credentials.ExpireAt,
			Info:     centrifuge.Raw(info),
		}
	}
}
