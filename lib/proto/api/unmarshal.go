package api

import (
	"encoding/json"

	"github.com/centrifugal/centrifugo/lib/proto"
)

// commandFromJSON tries to extract single APICommand encoded as JSON.
func commandFromJSON(msg []byte) (*Command, error) {
	var cmd Command
	err := json.Unmarshal(msg, &cmd)
	if err != nil {
		return nil, err
	}
	return &cmd, nil
}

var (
	arrayJSONPrefix  byte = '['
	objectJSONPrefix byte = '{'
)

// CommandsFromJSON tries to extract slice of APICommand encoded as JSON.
// This function understands both single object and array of commands JSON
// looking at first byte of msg.
func CommandsFromJSON(msg []byte) ([]*Command, error) {
	var cmds []*Command

	if len(msg) == 0 {
		return cmds, nil
	}

	firstByte := msg[0]

	switch firstByte {
	case objectJSONPrefix:
		// single command request
		command, err := commandFromJSON(msg)
		if err != nil {
			return nil, err
		}
		cmds = append(cmds, command)
	case arrayJSONPrefix:
		// array of commands received
		err := json.Unmarshal(msg, &cmds)
		if err != nil {
			return nil, err
		}
	default:
		return nil, proto.ErrInvalidData
	}
	return cmds, nil
}

// RequestDecoder ...
type RequestDecoder interface {
	Decode([]byte) (*Request, error)
}

// JSONRequestDecoder ...
type JSONRequestDecoder struct{}

// NewJSONRequestDecoder ...
func NewJSONRequestDecoder() *JSONRequestDecoder {
	return &JSONRequestDecoder{}
}

// Decode ...
func (e *JSONRequestDecoder) Decode(data []byte) (*Request, error) {
	var m Request
	err := json.Unmarshal(data, &m)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// ProtobufRequestDecoder ...
type ProtobufRequestDecoder struct{}

// NewProtobufRequestDecoder ...
func NewProtobufRequestDecoder() *ProtobufRequestDecoder {
	return &ProtobufRequestDecoder{}
}

// Decode ...
func (e *ProtobufRequestDecoder) Decode(data []byte) (*Request, error) {
	var m Request
	err := m.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// ParamsDecoder ...
type ParamsDecoder interface {
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

// JSONParamsDecoder ...
type JSONParamsDecoder struct{}

// NewJSONParamsDecoder ...
func NewJSONParamsDecoder() *JSONParamsDecoder {
	return &JSONParamsDecoder{}
}

// DecodePublish ...
func (d *JSONParamsDecoder) DecodePublish(data []byte) (*Publish, error) {
	var p Publish
	err := json.Unmarshal(data, &p)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodeBroadcast ...
func (d *JSONParamsDecoder) DecodeBroadcast(data []byte) (*Broadcast, error) {
	var p Broadcast
	err := json.Unmarshal(data, &p)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodeUnsubscribe ...
func (d *JSONParamsDecoder) DecodeUnsubscribe(data []byte) (*Unsubscribe, error) {
	var p Unsubscribe
	err := json.Unmarshal(data, &p)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodeDisconnect ...
func (d *JSONParamsDecoder) DecodeDisconnect(data []byte) (*Disconnect, error) {
	var p Disconnect
	err := json.Unmarshal(data, &p)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodePresence ...
func (d *JSONParamsDecoder) DecodePresence(data []byte) (*Presence, error) {
	var p Presence
	err := json.Unmarshal(data, &p)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodePresenceStats ...
func (d *JSONParamsDecoder) DecodePresenceStats(data []byte) (*PresenceStats, error) {
	var p PresenceStats
	err := json.Unmarshal(data, &p)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodeHistory ...
func (d *JSONParamsDecoder) DecodeHistory(data []byte) (*History, error) {
	var p History
	err := json.Unmarshal(data, &p)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodeInfo ...
func (d *JSONParamsDecoder) DecodeInfo(data []byte) (*Info, error) {
	var p Info
	err := json.Unmarshal(data, &p)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodeNode ...
func (d *JSONParamsDecoder) DecodeNode(data []byte) (*Node, error) {
	var p Node
	err := json.Unmarshal(data, &p)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// ProtobufParamsDecoder ...
type ProtobufParamsDecoder struct{}

// NewProtobufParamsDecoder ...
func NewProtobufParamsDecoder() *ProtobufParamsDecoder {
	return &ProtobufParamsDecoder{}
}

// DecodePublish ...
func (d *ProtobufParamsDecoder) DecodePublish(data []byte) (*Publish, error) {
	var p Publish
	err := p.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodeBroadcast ...
func (d *ProtobufParamsDecoder) DecodeBroadcast(data []byte) (*Broadcast, error) {
	var p Broadcast
	err := p.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodeUnsubscribe ...
func (d *ProtobufParamsDecoder) DecodeUnsubscribe(data []byte) (*Unsubscribe, error) {
	var p Unsubscribe
	err := p.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodeDisconnect ...
func (d *ProtobufParamsDecoder) DecodeDisconnect(data []byte) (*Disconnect, error) {
	var p Disconnect
	err := p.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodePresence ...
func (d *ProtobufParamsDecoder) DecodePresence(data []byte) (*Presence, error) {
	var p Presence
	err := p.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodePresenceStats ...
func (d *ProtobufParamsDecoder) DecodePresenceStats(data []byte) (*PresenceStats, error) {
	var p PresenceStats
	err := p.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodeHistory ...
func (d *ProtobufParamsDecoder) DecodeHistory(data []byte) (*History, error) {
	var p History
	err := p.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodeInfo ...
func (d *ProtobufParamsDecoder) DecodeInfo(data []byte) (*Info, error) {
	var p Info
	err := p.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodeNode ...
func (d *ProtobufParamsDecoder) DecodeNode(data []byte) (*Node, error) {
	var p Node
	err := p.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &p, nil
}
