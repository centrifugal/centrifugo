package clientproto

import (
	"github.com/centrifugal/protocol"
)

// NewMessagePush returns initialized async push message.
func NewMessagePush(data protocol.Raw) *protocol.Push {
	return &protocol.Push{
		Type: protocol.PushTypeMessage,
		Data: data,
	}
}

// NewPublicationPush returns initialized async publication message.
func NewPublicationPush(ch string, data protocol.Raw) *protocol.Push {
	return &protocol.Push{
		Type:    protocol.PushTypePublication,
		Channel: ch,
		Data:    data,
	}
}

// NewJoinPush returns initialized async join message.
func NewJoinPush(ch string, data protocol.Raw) *protocol.Push {
	return &protocol.Push{
		Type:    protocol.PushTypeJoin,
		Channel: ch,
		Data:    data,
	}
}

// NewLeavePush returns initialized async leave message.
func NewLeavePush(ch string, data protocol.Raw) *protocol.Push {
	return &protocol.Push{
		Type:    protocol.PushTypeLeave,
		Channel: ch,
		Data:    data,
	}
}

// NewUnsubPush returns initialized async unsubscribe message.
func NewUnsubPush(ch string, data protocol.Raw) *protocol.Push {
	return &protocol.Push{
		Type:    protocol.PushTypeUnsub,
		Channel: ch,
		Data:    data,
	}
}

// NewSubPush returns initialized async subscribe message.
func NewSubPush(ch string, data protocol.Raw) *protocol.Push {
	return &protocol.Push{
		Type:    protocol.PushTypeSub,
		Channel: ch,
		Data:    data,
	}
}

// ConnectResponse ...
type ConnectResponse struct {
	Error  *protocol.Error         `json:"error,omitempty"`
	Result *protocol.ConnectResult `json:"result,omitempty"`
}

// RefreshResponse ...
type RefreshResponse struct {
	Error  *protocol.Error         `json:"error,omitempty"`
	Result *protocol.RefreshResult `json:"result,omitempty"`
}

// SubscribeResponse ...
type SubscribeResponse struct {
	Error  *protocol.Error           `json:"error,omitempty"`
	Result *protocol.SubscribeResult `json:"result,omitempty"`
}

// SubRefreshResponse ...
type SubRefreshResponse struct {
	Error  *protocol.Error            `json:"error,omitempty"`
	Result *protocol.SubRefreshResult `json:"result,omitempty"`
}

// UnsubscribeResponse ...
type UnsubscribeResponse struct {
	Error  *protocol.Error             `json:"error,omitempty"`
	Result *protocol.UnsubscribeResult `json:"result,omitempty"`
}

// PingResponse ...
type PingResponse struct {
	Error  *protocol.Error      `json:"error,omitempty"`
	Result *protocol.PingResult `json:"result,omitempty"`
}

// PresenceResponse ...
type PresenceResponse struct {
	Error  *protocol.Error          `json:"error,omitempty"`
	Result *protocol.PresenceResult `json:"result,omitempty"`
}

// PresenceStatsResponse ...
type PresenceStatsResponse struct {
	Error  *protocol.Error               `json:"error,omitempty"`
	Result *protocol.PresenceStatsResult `json:"result,omitempty"`
}

// HistoryResponse ...
type HistoryResponse struct {
	Error  *protocol.Error         `json:"error,omitempty"`
	Result *protocol.HistoryResult `json:"result,omitempty"`
}

// RPCResponse ...
type RPCResponse struct {
	Error  *protocol.Error     `json:"error,omitempty"`
	Result *protocol.RPCResult `json:"result,omitempty"`
}

// PublishResponse ...
type PublishResponse struct {
	Error  *protocol.Error         `json:"error,omitempty"`
	Result *protocol.PublishResult `json:"result,omitempty"`
}
