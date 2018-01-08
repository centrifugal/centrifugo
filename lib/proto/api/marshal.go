package api

import "encoding/json"

// Encoder ...
type Encoder interface {
	EncodeResponse(*Response) ([]byte, error)
	EncodePublishResult(*PublishResult) ([]byte, error)
	EncodeBroadcastResult(*BroadcastResult) ([]byte, error)
	EncodeUnsubscribeResult(*UnsubscribeResult) ([]byte, error)
	EncodeDisconnectResult(*DisconnectResult) ([]byte, error)
	EncodePresenceResult(*PresenceResult) ([]byte, error)
	EncodePresenceStatsResult(*PresenceStatsResult) ([]byte, error)
	EncodeHistoryResult(*HistoryResult) ([]byte, error)
	EncodeChannelsResult(*ChannelsResult) ([]byte, error)
	EncodeInfoResult(*InfoResult) ([]byte, error)
}

// JSONEncoder ...
type JSONEncoder struct{}

// NewJSONEncoder ...
func NewJSONEncoder() *JSONEncoder {
	return &JSONEncoder{}
}

// EncodeResponse ...
func (e *JSONEncoder) EncodeResponse(response *Response) ([]byte, error) {
	return json.Marshal(response)
}

// EncodePublishResult ...
func (e *JSONEncoder) EncodePublishResult(res *PublishResult) ([]byte, error) {
	return json.Marshal(res)
}

// EncodeBroadcastResult ...
func (e *JSONEncoder) EncodeBroadcastResult(res *BroadcastResult) ([]byte, error) {
	return json.Marshal(res)
}

// EncodeUnsubscribeResult ...
func (e *JSONEncoder) EncodeUnsubscribeResult(res *UnsubscribeResult) ([]byte, error) {
	return json.Marshal(res)
}

// EncodeDisconnectResult ...
func (e *JSONEncoder) EncodeDisconnectResult(res *DisconnectResult) ([]byte, error) {
	return json.Marshal(res)
}

// EncodePresenceResult ...
func (e *JSONEncoder) EncodePresenceResult(res *PresenceResult) ([]byte, error) {
	return json.Marshal(res)
}

// EncodePresenceStatsResult ...
func (e *JSONEncoder) EncodePresenceStatsResult(res *PresenceStatsResult) ([]byte, error) {
	return json.Marshal(res)
}

// EncodeHistoryResult ...
func (e *JSONEncoder) EncodeHistoryResult(res *HistoryResult) ([]byte, error) {
	return json.Marshal(res)
}

// EncodeChannelsResult ...
func (e *JSONEncoder) EncodeChannelsResult(res *ChannelsResult) ([]byte, error) {
	return json.Marshal(res)
}

// EncodeInfoResult ...
func (e *JSONEncoder) EncodeInfoResult(res *InfoResult) ([]byte, error) {
	return json.Marshal(res)
}

// EncodeNodeResult ...
func (e *JSONEncoder) EncodeNodeResult(res *NodeResult) ([]byte, error) {
	return json.Marshal(res)
}

// ProtobufEncoder ...
type ProtobufEncoder struct{}

// NewProtobufEncoder ...
func NewProtobufEncoder() *ProtobufEncoder {
	return &ProtobufEncoder{}
}

// EncodeResponse ...
func (e *ProtobufEncoder) EncodeResponse(response *Response) ([]byte, error) {
	return response.Marshal()
}

// EncodePublishResult ...
func (e *ProtobufEncoder) EncodePublishResult(res *PublishResult) ([]byte, error) {
	return res.Marshal()
}

// EncodeBroadcastResult ...
func (e *ProtobufEncoder) EncodeBroadcastResult(res *BroadcastResult) ([]byte, error) {
	return res.Marshal()
}

// EncodeUnsubscribeResult ...
func (e *ProtobufEncoder) EncodeUnsubscribeResult(res *UnsubscribeResult) ([]byte, error) {
	return res.Marshal()
}

// EncodeDisconnectResult ...
func (e *ProtobufEncoder) EncodeDisconnectResult(res *DisconnectResult) ([]byte, error) {
	return res.Marshal()
}

// EncodePresenceResult ...
func (e *ProtobufEncoder) EncodePresenceResult(res *PresenceResult) ([]byte, error) {
	return res.Marshal()
}

// EncodePresenceStatsResult ...
func (e *ProtobufEncoder) EncodePresenceStatsResult(res *PresenceStatsResult) ([]byte, error) {
	return res.Marshal()
}

// EncodeHistoryResult ...
func (e *ProtobufEncoder) EncodeHistoryResult(res *HistoryResult) ([]byte, error) {
	return res.Marshal()
}

// EncodeChannelsResult ...
func (e *ProtobufEncoder) EncodeChannelsResult(res *ChannelsResult) ([]byte, error) {
	return res.Marshal()
}

// EncodeInfoResult ...
func (e *ProtobufEncoder) EncodeInfoResult(res *InfoResult) ([]byte, error) {
	return res.Marshal()
}
