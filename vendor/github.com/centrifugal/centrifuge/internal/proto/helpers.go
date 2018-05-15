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
