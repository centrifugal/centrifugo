package apiproto

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"io"
)

// CommandDecoder ...
type CommandDecoder interface {
	Reset([]byte) error
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

// Reset ...
func (d *JSONCommandDecoder) Reset(data []byte) error {
	d.decoder = json.NewDecoder(bytes.NewReader(data))
	return nil
}

// Decode ...
func (d *JSONCommandDecoder) Decode() (*Command, error) {
	var c Command
	err := d.decoder.Decode(&c)
	if err != nil {
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

// Reset ...
func (d *ProtobufCommandDecoder) Reset(data []byte) error {
	d.data = data
	d.offset = 0
	return nil
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

// Decoder ...
type Decoder interface {
	DecodePublish([]byte) (*PublishRequest, error)
	DecodeBroadcast([]byte) (*BroadcastRequest, error)
	DecodeUnsubscribe([]byte) (*UnsubscribeRequest, error)
	DecodeDisconnect([]byte) (*DisconnectRequest, error)
	DecodePresence([]byte) (*PresenceRequest, error)
	DecodePresenceStats([]byte) (*PresenceStatsRequest, error)
	DecodeHistory([]byte) (*HistoryRequest, error)
	DecodeChannels([]byte) (*ChannelsRequest, error)
	DecodeInfo([]byte) (*InfoRequest, error)
}

// JSONDecoder ...
type JSONDecoder struct{}

// NewJSONDecoder ...
func NewJSONDecoder() *JSONDecoder {
	return &JSONDecoder{}
}

// DecodePublish ...
func (d *JSONDecoder) DecodePublish(data []byte) (*PublishRequest, error) {
	var p PublishRequest
	err := json.Unmarshal(data, &p)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodeBroadcast ...
func (d *JSONDecoder) DecodeBroadcast(data []byte) (*BroadcastRequest, error) {
	var p BroadcastRequest
	err := json.Unmarshal(data, &p)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodeUnsubscribe ...
func (d *JSONDecoder) DecodeUnsubscribe(data []byte) (*UnsubscribeRequest, error) {
	var p UnsubscribeRequest
	err := json.Unmarshal(data, &p)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodeDisconnect ...
func (d *JSONDecoder) DecodeDisconnect(data []byte) (*DisconnectRequest, error) {
	var p DisconnectRequest
	err := json.Unmarshal(data, &p)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodePresence ...
func (d *JSONDecoder) DecodePresence(data []byte) (*PresenceRequest, error) {
	var p PresenceRequest
	err := json.Unmarshal(data, &p)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodePresenceStats ...
func (d *JSONDecoder) DecodePresenceStats(data []byte) (*PresenceStatsRequest, error) {
	var p PresenceStatsRequest
	err := json.Unmarshal(data, &p)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodeHistory ...
func (d *JSONDecoder) DecodeHistory(data []byte) (*HistoryRequest, error) {
	var p HistoryRequest
	err := json.Unmarshal(data, &p)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodeChannels ...
func (d *JSONDecoder) DecodeChannels(data []byte) (*ChannelsRequest, error) {
	var p ChannelsRequest
	err := json.Unmarshal(data, &p)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodeInfo ...
func (d *JSONDecoder) DecodeInfo(data []byte) (*InfoRequest, error) {
	var p InfoRequest
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

// DecodePublish ...
func (d *ProtobufDecoder) DecodePublish(data []byte) (*PublishRequest, error) {
	var p PublishRequest
	err := p.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodeBroadcast ...
func (d *ProtobufDecoder) DecodeBroadcast(data []byte) (*BroadcastRequest, error) {
	var p BroadcastRequest
	err := p.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodeUnsubscribe ...
func (d *ProtobufDecoder) DecodeUnsubscribe(data []byte) (*UnsubscribeRequest, error) {
	var p UnsubscribeRequest
	err := p.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodeDisconnect ...
func (d *ProtobufDecoder) DecodeDisconnect(data []byte) (*DisconnectRequest, error) {
	var p DisconnectRequest
	err := p.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodePresence ...
func (d *ProtobufDecoder) DecodePresence(data []byte) (*PresenceRequest, error) {
	var p PresenceRequest
	err := p.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodePresenceStats ...
func (d *ProtobufDecoder) DecodePresenceStats(data []byte) (*PresenceStatsRequest, error) {
	var p PresenceStatsRequest
	err := p.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodeHistory ...
func (d *ProtobufDecoder) DecodeHistory(data []byte) (*HistoryRequest, error) {
	var p HistoryRequest
	err := p.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodeChannels ...
func (d *ProtobufDecoder) DecodeChannels(data []byte) (*ChannelsRequest, error) {
	var p ChannelsRequest
	err := p.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodeInfo ...
func (d *ProtobufDecoder) DecodeInfo(data []byte) (*InfoRequest, error) {
	var p InfoRequest
	err := p.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &p, nil
}
