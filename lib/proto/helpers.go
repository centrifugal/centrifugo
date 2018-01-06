package proto

import (
	"github.com/nats-io/nuid"
)

// NewPublicationMessage returns initialized async data message.
func NewPublicationMessage(ch string, data Raw) *Message {
	return &Message{
		Type:    MessageTypePublication,
		UID:     nuid.Next(),
		Channel: ch,
		Data:    data,
	}
}

// NewJoinMessage returns initialized async join message.
func NewJoinMessage(ch string, data Raw) *Message {
	return &Message{
		Type:    MessageTypeJoin,
		UID:     nuid.Next(),
		Channel: ch,
		Data:    data,
	}
}

// NewLeaveMessage returns initialized async leave message.
func NewLeaveMessage(ch string, data Raw) *Message {
	return &Message{
		Type:    MessageTypeLeave,
		UID:     nuid.Next(),
		Channel: ch,
		Data:    data,
	}
}

// NewUnsubscribeMessage returns initialized async unsubscribe message.
func NewUnsubscribeMessage(ch string, data Raw) *Message {
	return &Message{
		Type:    MessageTypeUnsubscribe,
		UID:     nuid.Next(),
		Channel: ch,
		Data:    data,
	}
}
