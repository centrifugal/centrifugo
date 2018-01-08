package api

import (
	"encoding/json"
)

// Decoder ...
type Decoder interface {
	DecodeRequest([]byte) (*Request, error)
	DecodePublish([]byte) (*Publish, error)
	DecodeBroadcast([]byte) (*Broadcast, error)
	DecodeUnsubscribe([]byte) (*Unsubscribe, error)
	DecodeDisconnect([]byte) (*Disconnect, error)
	DecodePresence([]byte) (*Presence, error)
	DecodePresenceStats([]byte) (*PresenceStats, error)
	DecodeHistory([]byte) (*History, error)
	DecodeInfo([]byte) (*Info, error)
	DecodeNode([]byte) (*Node, error)
}

// JSONDecoder ...
type JSONDecoder struct{}

// NewJSONDecoder ...
func NewJSONDecoder() *JSONDecoder {
	return &JSONDecoder{}
}

// DecodeRequest ...
func (d *JSONDecoder) DecodeRequest(data []byte) (*Request, error) {
	var m Request
	err := json.Unmarshal(data, &m)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// DecodePublish ...
func (d *JSONDecoder) DecodePublish(data []byte) (*Publish, error) {
	var p Publish
	err := json.Unmarshal(data, &p)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodeBroadcast ...
func (d *JSONDecoder) DecodeBroadcast(data []byte) (*Broadcast, error) {
	var p Broadcast
	err := json.Unmarshal(data, &p)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodeUnsubscribe ...
func (d *JSONDecoder) DecodeUnsubscribe(data []byte) (*Unsubscribe, error) {
	var p Unsubscribe
	err := json.Unmarshal(data, &p)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodeDisconnect ...
func (d *JSONDecoder) DecodeDisconnect(data []byte) (*Disconnect, error) {
	var p Disconnect
	err := json.Unmarshal(data, &p)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodePresence ...
func (d *JSONDecoder) DecodePresence(data []byte) (*Presence, error) {
	var p Presence
	err := json.Unmarshal(data, &p)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodePresenceStats ...
func (d *JSONDecoder) DecodePresenceStats(data []byte) (*PresenceStats, error) {
	var p PresenceStats
	err := json.Unmarshal(data, &p)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodeHistory ...
func (d *JSONDecoder) DecodeHistory(data []byte) (*History, error) {
	var p History
	err := json.Unmarshal(data, &p)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodeInfo ...
func (d *JSONDecoder) DecodeInfo(data []byte) (*Info, error) {
	var p Info
	err := json.Unmarshal(data, &p)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodeNode ...
func (d *JSONDecoder) DecodeNode(data []byte) (*Node, error) {
	var p Node
	err := json.Unmarshal(data, &p)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// ProtobufDecoder ...
type ProtobufDecoder struct{}

// NewProtobufDecoder ...
func NewProtobufDecoder() *ProtobufDecoder {
	return &ProtobufDecoder{}
}

// DecodeRequest ...
func (d *ProtobufDecoder) DecodeRequest(data []byte) (*Request, error) {
	var m Request
	err := m.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// DecodePublish ...
func (d *ProtobufDecoder) DecodePublish(data []byte) (*Publish, error) {
	var p Publish
	err := p.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodeBroadcast ...
func (d *ProtobufDecoder) DecodeBroadcast(data []byte) (*Broadcast, error) {
	var p Broadcast
	err := p.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodeUnsubscribe ...
func (d *ProtobufDecoder) DecodeUnsubscribe(data []byte) (*Unsubscribe, error) {
	var p Unsubscribe
	err := p.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodeDisconnect ...
func (d *ProtobufDecoder) DecodeDisconnect(data []byte) (*Disconnect, error) {
	var p Disconnect
	err := p.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodePresence ...
func (d *ProtobufDecoder) DecodePresence(data []byte) (*Presence, error) {
	var p Presence
	err := p.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodePresenceStats ...
func (d *ProtobufDecoder) DecodePresenceStats(data []byte) (*PresenceStats, error) {
	var p PresenceStats
	err := p.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodeHistory ...
func (d *ProtobufDecoder) DecodeHistory(data []byte) (*History, error) {
	var p History
	err := p.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodeInfo ...
func (d *ProtobufDecoder) DecodeInfo(data []byte) (*Info, error) {
	var p Info
	err := p.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodeNode ...
func (d *ProtobufDecoder) DecodeNode(data []byte) (*Node, error) {
	var p Node
	err := p.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &p, nil
}
