package proto

import (
	"encoding/json"
)

// MessageEncoder ...
type MessageEncoder interface {
	Encode(*Message) ([]byte, error)
	EncodePublication(*Publication) ([]byte, error)
	EncodeJoin(*Join) ([]byte, error)
	EncodeLeave(*Leave) ([]byte, error)
}

// JSONMessageEncoder ...
type JSONMessageEncoder struct {
}

// NewJSONMessageEncoder ...
func NewJSONMessageEncoder() *JSONMessageEncoder {
	return &JSONMessageEncoder{}
}

// Encode ...
func (e *JSONMessageEncoder) Encode(message *Message) ([]byte, error) {
	return json.Marshal(message)
}

// EncodePublication ...
func (e *JSONMessageEncoder) EncodePublication(message *Publication) ([]byte, error) {
	return json.Marshal(message)
}

// EncodeJoin ...
func (e *JSONMessageEncoder) EncodeJoin(message *Join) ([]byte, error) {
	return json.Marshal(message)
}

// EncodeLeave ...
func (e *JSONMessageEncoder) EncodeLeave(message *Leave) ([]byte, error) {
	return json.Marshal(message)
}

// ProtobufMessageEncoder ...
type ProtobufMessageEncoder struct {
}

// NewProtobufMessageEncoder ...
func NewProtobufMessageEncoder() *ProtobufMessageEncoder {
	return &ProtobufMessageEncoder{}
}

// Encode ...
func (e *ProtobufMessageEncoder) Encode(message *Message) ([]byte, error) {
	return message.Marshal()
}

// EncodePublication ...
func (e *ProtobufMessageEncoder) EncodePublication(message *Publication) ([]byte, error) {
	return message.Marshal()
}

// EncodeJoin ...
func (e *ProtobufMessageEncoder) EncodeJoin(message *Join) ([]byte, error) {
	return message.Marshal()
}

// EncodeLeave ...
func (e *ProtobufMessageEncoder) EncodeLeave(message *Leave) ([]byte, error) {
	return message.Marshal()
}
