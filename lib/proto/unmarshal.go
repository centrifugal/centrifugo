package proto

import "encoding/json"

// MessageDecoder ...
type MessageDecoder interface {
	Decode([]byte) (*Message, error)
	DecodePublication([]byte) (*Publication, error)
	DecodeJoin([]byte) (*Join, error)
	DecodeLeave([]byte) (*Leave, error)
}

// JSONMessageDecoder ...
type JSONMessageDecoder struct {
}

// NewJSONMessageDecoder ...
func NewJSONMessageDecoder() *JSONMessageDecoder {
	return &JSONMessageDecoder{}
}

// Decode ...
func (e *JSONMessageDecoder) Decode(data []byte) (*Message, error) {
	var m Message
	err := json.Unmarshal(data, &m)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// DecodePublication ...
func (e *JSONMessageDecoder) DecodePublication(data []byte) (*Publication, error) {
	var m Publication
	err := json.Unmarshal(data, &m)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// DecodeJoin ...
func (e *JSONMessageDecoder) DecodeJoin(data []byte) (*Join, error) {
	var m Join
	err := json.Unmarshal(data, &m)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// DecodeLeave  ...
func (e *JSONMessageDecoder) DecodeLeave(data []byte) (*Leave, error) {
	var m Leave
	err := json.Unmarshal(data, &m)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// ProtobufMessageDecoder ...
type ProtobufMessageDecoder struct {
}

// NewProtobufMessageDecoder ...
func NewProtobufMessageDecoder() *ProtobufMessageDecoder {
	return &ProtobufMessageDecoder{}
}

// Decode ...
func (e *ProtobufMessageDecoder) Decode(data []byte) (*Message, error) {
	var m Message
	err := m.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// DecodePublication ...
func (e *ProtobufMessageDecoder) DecodePublication(data []byte) (*Publication, error) {
	var m Publication
	err := m.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// DecodeJoin ...
func (e *ProtobufMessageDecoder) DecodeJoin(data []byte) (*Join, error) {
	var m Join
	err := m.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// DecodeLeave  ...
func (e *ProtobufMessageDecoder) DecodeLeave(data []byte) (*Leave, error) {
	var m Leave
	err := m.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &m, nil
}
