package client

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"io"
)

// CommandDecoder ...
type CommandDecoder interface {
	Decode() (*Command, error)
}

// JSONCommandDecoder ...
type JSONCommandDecoder struct {
	decoder *json.Decoder
}

// NewJSONCommandDecoder ...
func NewJSONCommandDecoder(data []byte) *JSONCommandDecoder {
	return &JSONCommandDecoder{
		decoder: json.NewDecoder(bytes.NewReader(data)),
	}
}

// Decode ...
func (d *JSONCommandDecoder) Decode() (*Command, error) {
	var c Command
	err := d.decoder.Decode(&c)
	if err == io.EOF {
		return nil, err
	}
	return &c, nil
}

// ProtobufCommandDecoder ...
type ProtobufCommandDecoder struct {
	data   []byte
	offset int
}

// NewProtobufCommandDecoder ...
func NewProtobufCommandDecoder(data []byte) *ProtobufCommandDecoder {
	return &ProtobufCommandDecoder{
		data: data,
	}
}

// Decode ...
func (d *ProtobufCommandDecoder) Decode() (*Command, error) {
	if d.offset < len(d.data) {
		var c Command
		l, n := binary.Uvarint(d.data[d.offset:])
		cmdBytes := d.data[d.offset+n : d.offset+n+int(l)]
		err := c.Unmarshal(cmdBytes)
		if err != nil {
			return nil, err
		}
		d.offset = d.offset + n + int(l)
		return &c, nil
	}
	return nil, io.EOF
}

// ParamsDecoder ...
type ParamsDecoder interface {
	DecodeConnect([]byte) (*Connect, error)
	DecodeRefresh([]byte) (*Refresh, error)
	DecodeSubscribe([]byte) (*Subscribe, error)
	DecodeUnsubscribe([]byte) (*Unsubscribe, error)
	DecodePublish([]byte) (*Publish, error)
	DecodePresence([]byte) (*Presence, error)
	DecodePresenceStats([]byte) (*PresenceStats, error)
	DecodeHistory([]byte) (*History, error)
	DecodePing([]byte) (*Ping, error)
}

// JSONParamsDecoder ...
type JSONParamsDecoder struct{}

// NewJSONParamsDecoder ...
func NewJSONParamsDecoder() *JSONParamsDecoder {
	return &JSONParamsDecoder{}
}

// DecodeConnect ...
func (d *JSONParamsDecoder) DecodeConnect(data []byte) (*Connect, error) {
	var p Connect
	err := json.Unmarshal(data, &p)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodeRefresh ...
func (d *JSONParamsDecoder) DecodeRefresh(data []byte) (*Refresh, error) {
	var p Refresh
	err := json.Unmarshal(data, &p)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodeSubscribe ...
func (d *JSONParamsDecoder) DecodeSubscribe(data []byte) (*Subscribe, error) {
	var p Subscribe
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

// DecodePublish ...
func (d *JSONParamsDecoder) DecodePublish(data []byte) (*Publish, error) {
	var p Publish
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

// DecodePing ...
func (d *JSONParamsDecoder) DecodePing(data []byte) (*Ping, error) {
	var p Ping
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

// DecodeConnect ...
func (d *ProtobufParamsDecoder) DecodeConnect(data []byte) (*Connect, error) {
	var p Connect
	err := p.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodeRefresh ...
func (d *ProtobufParamsDecoder) DecodeRefresh(data []byte) (*Refresh, error) {
	var p Refresh
	err := p.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodeSubscribe ...
func (d *ProtobufParamsDecoder) DecodeSubscribe(data []byte) (*Subscribe, error) {
	var p Subscribe
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

// DecodePublish ...
func (d *ProtobufParamsDecoder) DecodePublish(data []byte) (*Publish, error) {
	var p Publish
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

// DecodePing ...
func (d *ProtobufParamsDecoder) DecodePing(data []byte) (*Ping, error) {
	var p Ping
	err := p.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &p, nil
}
