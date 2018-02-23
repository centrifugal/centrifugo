package proto

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"io"
)

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

// ParamsDecoder ...
type ParamsDecoder interface {
	DecodeConnect([]byte) (*ConnectRequest, error)
	DecodeRefresh([]byte) (*RefreshRequest, error)
	DecodeSubscribe([]byte) (*SubscribeRequest, error)
	DecodeUnsubscribe([]byte) (*UnsubscribeRequest, error)
	DecodePublish([]byte) (*PublishRequest, error)
	DecodePresence([]byte) (*PresenceRequest, error)
	DecodePresenceStats([]byte) (*PresenceStatsRequest, error)
	DecodeHistory([]byte) (*HistoryRequest, error)
	DecodePing([]byte) (*PingRequest, error)
}

// JSONParamsDecoder ...
type JSONParamsDecoder struct{}

// NewJSONParamsDecoder ...
func NewJSONParamsDecoder() *JSONParamsDecoder {
	return &JSONParamsDecoder{}
}

// DecodeConnect ...
func (d *JSONParamsDecoder) DecodeConnect(data []byte) (*ConnectRequest, error) {
	var p ConnectRequest
	err := json.Unmarshal(data, &p)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodeRefresh ...
func (d *JSONParamsDecoder) DecodeRefresh(data []byte) (*RefreshRequest, error) {
	var p RefreshRequest
	err := json.Unmarshal(data, &p)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodeSubscribe ...
func (d *JSONParamsDecoder) DecodeSubscribe(data []byte) (*SubscribeRequest, error) {
	var p SubscribeRequest
	err := json.Unmarshal(data, &p)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodeUnsubscribe ...
func (d *JSONParamsDecoder) DecodeUnsubscribe(data []byte) (*UnsubscribeRequest, error) {
	var p UnsubscribeRequest
	err := json.Unmarshal(data, &p)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodePublish ...
func (d *JSONParamsDecoder) DecodePublish(data []byte) (*PublishRequest, error) {
	var p PublishRequest
	err := json.Unmarshal(data, &p)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodePresence ...
func (d *JSONParamsDecoder) DecodePresence(data []byte) (*PresenceRequest, error) {
	var p PresenceRequest
	err := json.Unmarshal(data, &p)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodePresenceStats ...
func (d *JSONParamsDecoder) DecodePresenceStats(data []byte) (*PresenceStatsRequest, error) {
	var p PresenceStatsRequest
	err := json.Unmarshal(data, &p)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodeHistory ...
func (d *JSONParamsDecoder) DecodeHistory(data []byte) (*HistoryRequest, error) {
	var p HistoryRequest
	err := json.Unmarshal(data, &p)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodePing ...
func (d *JSONParamsDecoder) DecodePing(data []byte) (*PingRequest, error) {
	var p PingRequest
	if data != nil {
		err := json.Unmarshal(data, &p)
		if err != nil {
			return nil, err
		}
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
func (d *ProtobufParamsDecoder) DecodeConnect(data []byte) (*ConnectRequest, error) {
	var p ConnectRequest
	err := p.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodeRefresh ...
func (d *ProtobufParamsDecoder) DecodeRefresh(data []byte) (*RefreshRequest, error) {
	var p RefreshRequest
	err := p.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodeSubscribe ...
func (d *ProtobufParamsDecoder) DecodeSubscribe(data []byte) (*SubscribeRequest, error) {
	var p SubscribeRequest
	err := p.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodeUnsubscribe ...
func (d *ProtobufParamsDecoder) DecodeUnsubscribe(data []byte) (*UnsubscribeRequest, error) {
	var p UnsubscribeRequest
	err := p.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodePublish ...
func (d *ProtobufParamsDecoder) DecodePublish(data []byte) (*PublishRequest, error) {
	var p PublishRequest
	err := p.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodePresence ...
func (d *ProtobufParamsDecoder) DecodePresence(data []byte) (*PresenceRequest, error) {
	var p PresenceRequest
	err := p.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodePresenceStats ...
func (d *ProtobufParamsDecoder) DecodePresenceStats(data []byte) (*PresenceStatsRequest, error) {
	var p PresenceStatsRequest
	err := p.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodeHistory ...
func (d *ProtobufParamsDecoder) DecodeHistory(data []byte) (*HistoryRequest, error) {
	var p HistoryRequest
	err := p.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodePing ...
func (d *ProtobufParamsDecoder) DecodePing(data []byte) (*PingRequest, error) {
	var p PingRequest
	err := p.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &p, nil
}
