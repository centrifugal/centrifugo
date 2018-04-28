package proto

// NewMessage returns initialized async push message.
func NewMessage(data Raw) *Push {
	return &Push{
		Type: PushTypeMessage,
		Data: data,
	}
}

// NewPublication returns initialized async publication message.
func NewPublication(ch string, data Raw) *Push {
	return &Push{
		Type:    PushTypePublication,
		Channel: ch,
		Data:    data,
	}
}

// NewJoin returns initialized async join message.
func NewJoin(ch string, data Raw) *Push {
	return &Push{
		Type:    PushTypeJoin,
		Channel: ch,
		Data:    data,
	}
}

// NewLeave returns initialized async leave message.
func NewLeave(ch string, data Raw) *Push {
	return &Push{
		Type:    PushTypeLeave,
		Channel: ch,
		Data:    data,
	}
}

// NewUnsub returns initialized async unsubscribe message.
func NewUnsub(ch string, data Raw) *Push {
	return &Push{
		Type:    PushTypeUnsub,
		Channel: ch,
		Data:    data,
	}
}
