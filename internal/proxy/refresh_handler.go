package proxy

import (
	"context"
	"encoding/base64"

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
		refreshResp, err := h.config.Proxy.ProxyRefresh(context.Background(), RefreshRequest{
			ClientID:  client.ID(),
			UserID:    client.UserID(),
			Transport: client.Transport(),
		})
		if err != nil {
			// TODO: add retries.
			node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error proxying refresh", map[string]interface{}{"error": err.Error()}))
			return centrifuge.RefreshReply{}
		}

		// TODO: add Disconnect to RefreshReply.

		var info []byte
		if client.Transport().Encoding() == "json" {
			info = refreshResp.Credentials.Info
		} else {
			if refreshResp.Credentials.Base64Info != "" {
				decodedInfo, err := base64.StdEncoding.DecodeString(refreshResp.Credentials.Base64Info)
				if err != nil {
					node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error decoding base64 info", map[string]interface{}{"client": client.ID(), "error": err.Error()}))
					return centrifuge.RefreshReply{}
				}
				info = decodedInfo
			}
		}

		return centrifuge.RefreshReply{
			ExpireAt: refreshResp.Credentials.ExpireAt,
			Info:     centrifuge.Raw(info),
		}
	}
}
