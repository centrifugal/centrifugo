package apiproto

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
)

// UnmarshalJSON helps to unmarshal command method when set as string.
func (m *Command_MethodType) UnmarshalJSON(data []byte) error {
	if len(data) > 2 {
		if v, ok := Command_MethodType_value[strings.ToUpper(string(data[1:len(data)-1]))]; ok {
			*m = Command_MethodType(v)
			return nil
		}
	}
	val, err := strconv.Atoi(string(data))
	if err != nil {
		return err
	}
	*m = Command_MethodType(val)
	return nil
}

// CommandDecoder ...
type CommandDecoder interface {
	Reset([]byte)
	Decode() (*Command, error)
}

var _ CommandDecoder = (*JSONCommandDecoder)(nil)

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
func (d *JSONCommandDecoder) Reset(data []byte) {
	d.decoder = json.NewDecoder(bytes.NewReader(data))
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

// ParamsDecoder ...
type ParamsDecoder interface {
	DecodePublish([]byte) (*PublishRequest, error)
	DecodeBroadcast([]byte) (*BroadcastRequest, error)
	DecodeSubscribe([]byte) (*SubscribeRequest, error)
	DecodeUnsubscribe([]byte) (*UnsubscribeRequest, error)
	DecodeDisconnect([]byte) (*DisconnectRequest, error)
	DecodePresence([]byte) (*PresenceRequest, error)
	DecodePresenceStats([]byte) (*PresenceStatsRequest, error)
	DecodeHistory([]byte) (*HistoryRequest, error)
	DecodeHistoryRemove([]byte) (*HistoryRemoveRequest, error)
	DecodeInfo([]byte) (*InfoRequest, error)
	DecodeRPC([]byte) (*RPCRequest, error)
}

var _ ParamsDecoder = (*JSONParamsDecoder)(nil)

// JSONParamsDecoder ...
type JSONParamsDecoder struct{}

// NewJSONParamsDecoder ...
func NewJSONParamsDecoder() *JSONParamsDecoder {
	return &JSONParamsDecoder{}
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

// DecodeBroadcast ...
func (d *JSONParamsDecoder) DecodeBroadcast(data []byte) (*BroadcastRequest, error) {
	var p BroadcastRequest
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

// DecodeDisconnect ...
func (d *JSONParamsDecoder) DecodeDisconnect(data []byte) (*DisconnectRequest, error) {
	var p DisconnectRequest
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

// DecodeHistoryRemove ...
func (d *JSONParamsDecoder) DecodeHistoryRemove(data []byte) (*HistoryRemoveRequest, error) {
	var p HistoryRemoveRequest
	err := json.Unmarshal(data, &p)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodeInfo ...
func (d *JSONParamsDecoder) DecodeInfo(data []byte) (*InfoRequest, error) {
	var p InfoRequest
	err := json.Unmarshal(data, &p)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodeRPC ...
func (d *JSONParamsDecoder) DecodeRPC(data []byte) (*RPCRequest, error) {
	var p RPCRequest
	err := json.Unmarshal(data, &p)
	if err != nil {
		return nil, err
	}
	return &p, nil
}
