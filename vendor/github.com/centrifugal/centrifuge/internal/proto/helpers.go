package proto

// NewMessagePush returns initialized async push message.
func NewMessagePush(data Raw) *Push {
	return &Push{
		Type: PushTypeMessage,
		Data: data,
	}
}

// NewPublicationPush returns initialized async publication message.
func NewPublicationPush(ch string, data Raw) *Push {
	return &Push{
		Type:    PushTypePublication,
		Channel: ch,
		Data:    data,
	}
}

// NewJoinPush returns initialized async join message.
func NewJoinPush(ch string, data Raw) *Push {
	return &Push{
		Type:    PushTypeJoin,
		Channel: ch,
		Data:    data,
	}
}

// NewLeavePush returns initialized async leave message.
func NewLeavePush(ch string, data Raw) *Push {
	return &Push{
		Type:    PushTypeLeave,
		Channel: ch,
		Data:    data,
	}
}

// NewUnsubPush returns initialized async unsubscribe message.
func NewUnsubPush(ch string, data Raw) *Push {
	return &Push{
		Type:    PushTypeUnsub,
		Channel: ch,
		Data:    data,
	}
}

// ConnectResponse ...
type ConnectResponse struct {
	Error  *Error         `json:"error,omitempty"`
	Result *ConnectResult `json:"result,omitempty"`
}

// RefreshResponse ...
type RefreshResponse struct {
	Error  *Error         `json:"error,omitempty"`
	Result *RefreshResult `json:"result,omitempty"`
}

// SubscribeResponse ...
type SubscribeResponse struct {
	Error  *Error           `json:"error,omitempty"`
	Result *SubscribeResult `json:"result,omitempty"`
}

// UnsubscribeResponse ...
type UnsubscribeResponse struct {
	Error  *Error             `json:"error,omitempty"`
	Result *UnsubscribeResult `json:"result,omitempty"`
}

// PingResponse ...
type PingResponse struct {
	Error  *Error      `json:"error,omitempty"`
	Result *PingResult `json:"result,omitempty"`
}

// PresenceResponse ...
type PresenceResponse struct {
	Error  *Error          `json:"error,omitempty"`
	Result *PresenceResult `json:"result,omitempty"`
}

// PresenceStatsResponse ...
type PresenceStatsResponse struct {
	Error  *Error               `json:"error,omitempty"`
	Result *PresenceStatsResult `json:"result,omitempty"`
}

// HistoryResponse ...
type HistoryResponse struct {
	Error  *Error         `json:"error,omitempty"`
	Result *HistoryResult `json:"result,omitempty"`
}

// RPCResponse ...
type RPCResponse struct {
	Error  *Error     `json:"error,omitempty"`
	Result *RPCResult `json:"result,omitempty"`
}

// PublishResponse ...
type PublishResponse struct {
	Error  *Error         `json:"error,omitempty"`
	Result *PublishResult `json:"result,omitempty"`
}
