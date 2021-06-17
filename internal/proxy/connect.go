package proxy

import (
	"context"
	"encoding/json"

	"github.com/centrifugal/centrifugo/v3/internal/proxy/proxyproto"

	"github.com/centrifugal/centrifuge"
)

// ConnectRequest ...
type ConnectRequest struct {
	ClientID  string
	Transport centrifuge.TransportInfo
	Data      []byte
	Name      string
	Version   string
}

// BoolValue allows override boolean option.
type BoolValue struct {
	Value bool `json:"value,omitempty"`
}

// SubscribeOptionOverride to override configured behaviour.
type SubscribeOptionOverride struct {
	// Presence turns on participating in channel presence.
	Presence *BoolValue `json:"presence,omitempty"`
	// JoinLeave enables sending Join and Leave messages for this client in channel.
	JoinLeave *BoolValue `json:"join_leave,omitempty"`
	// Position on says that client will additionally sync its position inside
	// a stream to prevent message loss. Make sure you are enabling Position in channels
	// that maintain Publication history stream. When Position is on  Centrifuge will
	// include StreamPosition information to subscribe response - for a client to be able
	// to manually track its position inside a stream.
	Position *BoolValue `json:"position,omitempty"`
	// Recover turns on recovery option for a channel. In this case client will try to
	// recover missed messages automatically upon resubscribe to a channel after reconnect
	// to a server. This option also enables client position tracking inside a stream
	// (like Position option) to prevent occasional message loss. Make sure you are using
	// Recover in channels that maintain Publication history stream.
	Recover *BoolValue `json:"recover,omitempty"`
}

// SubscribeOptions define per-subscription options.
type SubscribeOptions struct {
	// ExpireAt defines time in future when subscription should expire,
	// zero value means no expiration.
	ExpireAt int64 `json:"expire_at,omitempty"`
	// Info defines custom channel information, zero value means no channel information.
	Info json.RawMessage `json:"info,omitempty"`
	// Base64Info is like Info but for binary.
	Base64Info string `json:"b64info,omitempty"`
	// Data to send to a client with Subscribe Push.
	Data json.RawMessage `json:"data,omitempty"`
	// Base64Data is like Data but for binary data.
	Base64Data string `json:"b64data,omitempty"`
	// Override channel options can contain channel options overrides.
	Override *SubscribeOptionOverride `json:"override,omitempty"`
}

// ConnectCredentials ...
type ConnectCredentials struct {
	UserID     string                      `json:"user"`
	ExpireAt   int64                       `json:"expire_at"`
	Info       json.RawMessage             `json:"info"`
	Base64Info string                      `json:"b64info"`
	Data       json.RawMessage             `json:"data"`
	Base64Data string                      `json:"b64data"`
	Channels   []string                    `json:"channels"`
	Subs       map[string]SubscribeOptions `json:"subs,omitempty"`
}

// ConnectReply ...
type ConnectReply struct {
	Result     *ConnectCredentials    `json:"result"`
	Error      *centrifuge.Error      `json:"error"`
	Disconnect *centrifuge.Disconnect `json:"disconnect"`
}

// ConnectProxy allows to proxy connect requests to application backend to
// authenticate client connection.
type ConnectProxy interface {
	ProxyConnect(context.Context, *proxyproto.ConnectRequest) (*proxyproto.ConnectResponse, error)
	// Protocol for metrics and logging.
	Protocol() string
	// UseBase64 for bytes in requests from Centrifugo to application backend.
	UseBase64() bool
}
