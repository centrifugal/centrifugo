package proto

// NewPublicationMessage returns initialized async data message.
func NewPublicationMessage(ch string, data Raw) *Message {
	return &Message{
		Type:    MessageTypePublication,
		Channel: ch,
		Data:    data,
	}
}

// NewJoinMessage returns initialized async join message.
func NewJoinMessage(ch string, data Raw) *Message {
	return &Message{
		Type:    MessageTypeJoin,
		Channel: ch,
		Data:    data,
	}
}

// NewLeaveMessage returns initialized async leave message.
func NewLeaveMessage(ch string, data Raw) *Message {
	return &Message{
		Type:    MessageTypeLeave,
		Channel: ch,
		Data:    data,
	}
}

// NewUnsubscribeMessage returns initialized async unsubscribe message.
func NewUnsubscribeMessage(ch string, data Raw) *Message {
	return &Message{
		Type:    MessageTypeUnsubscribe,
		Channel: ch,
		Data:    data,
	}
}
