package apiproto

import "encoding/json"

var _ ParamsDecoder = (*JSONParamsDecoder)(nil)

// JSONParamsDecoder ...
type JSONParamsDecoder struct{}

// NewJSONParamsDecoder ...
func NewJSONParamsDecoder() *JSONParamsDecoder {
	return &JSONParamsDecoder{}
}

// DecodeBatch ...
func (d *JSONParamsDecoder) DecodeBatch(data []byte) (*BatchRequest, error) {
	var p BatchRequest
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
	//nolint:staticcheck
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

// DecodeRefresh ...
func (d *JSONParamsDecoder) DecodeRefresh(data []byte) (*RefreshRequest, error) {
	var p RefreshRequest
	err := json.Unmarshal(data, &p)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DecodeChannels ...
func (d *JSONParamsDecoder) DecodeChannels(data []byte) (*ChannelsRequest, error) {
	var p ChannelsRequest
	err := json.Unmarshal(data, &p)
	if err != nil {
		return nil, err
	}
	return &p, nil
}
