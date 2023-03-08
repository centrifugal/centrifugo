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

// RequestDecoder ...
type RequestDecoder interface {
	DecodeBatch([]byte) (*BatchRequest, error)
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
	DecodeRefresh([]byte) (*RefreshRequest, error)
	DecodeChannels([]byte) (*ChannelsRequest, error)
}
