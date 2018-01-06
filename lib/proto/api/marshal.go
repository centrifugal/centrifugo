package api

import "encoding/json"

// ResponseEncoder ...
type ResponseEncoder interface {
	Encode(*Response) ([]byte, error)
}

// JSONResponseEncoder ...
type JSONResponseEncoder struct{}

// NewJSONResponseEncoder ...
func NewJSONResponseEncoder() *JSONResponseEncoder {
	return &JSONResponseEncoder{}
}

// Encode ...
func (e *JSONResponseEncoder) Encode(response *Response) ([]byte, error) {
	return json.Marshal(response)
}

// ProtobufResponseEncoder ...
type ProtobufResponseEncoder struct{}

// NewProtobufResponseEncoder ...
func NewProtobufResponseEncoder() *ProtobufResponseEncoder {
	return &ProtobufResponseEncoder{}
}

// Encode ...
func (e *ProtobufResponseEncoder) Encode(response *Response) ([]byte, error) {
	return response.Marshal()
}

// ResultEncoder ...
type ResultEncoder interface {
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

// JSONResultEncoder ...
type JSONResultEncoder struct{}

// NewJSONResultEncoder ...
func NewJSONResultEncoder() *JSONResultEncoder {
	return &JSONResultEncoder{}
}

// EncodePublishResult ...
func (e *JSONResultEncoder) EncodePublishResult(res *PublishResult) ([]byte, error) {
	return json.Marshal(res)
}

// EncodeBroadcastResult ...
func (e *JSONResultEncoder) EncodeBroadcastResult(res *BroadcastResult) ([]byte, error) {
	return json.Marshal(res)
}

// EncodeUnsubscribeResult ...
func (e *JSONResultEncoder) EncodeUnsubscribeResult(res *UnsubscribeResult) ([]byte, error) {
	return json.Marshal(res)
}

// EncodeDisconnectResult ...
func (e *JSONResultEncoder) EncodeDisconnectResult(res *DisconnectResult) ([]byte, error) {
	return json.Marshal(res)
}

// EncodePresenceResult ...
func (e *JSONResultEncoder) EncodePresenceResult(res *PresenceResult) ([]byte, error) {
	return json.Marshal(res)
}

// EncodePresenceStatsResult ...
func (e *JSONResultEncoder) EncodePresenceStatsResult(res *PresenceStatsResult) ([]byte, error) {
	return json.Marshal(res)
}

// EncodeHistoryResult ...
func (e *JSONResultEncoder) EncodeHistoryResult(res *HistoryResult) ([]byte, error) {
	return json.Marshal(res)
}

// EncodeChannelsResult ...
func (e *JSONResultEncoder) EncodeChannelsResult(res *ChannelsResult) ([]byte, error) {
	return json.Marshal(res)
}

// EncodeInfoResult ...
func (e *JSONResultEncoder) EncodeInfoResult(res *InfoResult) ([]byte, error) {
	return json.Marshal(res)
}

// EncodeNodeResult ...
func (e *JSONResultEncoder) EncodeNodeResult(res *NodeResult) ([]byte, error) {
	return json.Marshal(res)
}

// ProtobufResultEncoder ...
type ProtobufResultEncoder struct{}

// NewProtobufResultEncoder ...
func NewProtobufResultEncoder() *ProtobufResultEncoder {
	return &ProtobufResultEncoder{}
}

// EncodePublishResult ...
func (e *ProtobufResultEncoder) EncodePublishResult(res *PublishResult) ([]byte, error) {
	return res.Marshal()
}

// EncodeBroadcastResult ...
func (e *ProtobufResultEncoder) EncodeBroadcastResult(res *BroadcastResult) ([]byte, error) {
	return res.Marshal()
}

// EncodeUnsubscribeResult ...
func (e *ProtobufResultEncoder) EncodeUnsubscribeResult(res *UnsubscribeResult) ([]byte, error) {
	return res.Marshal()
}

// EncodeDisconnectResult ...
func (e *ProtobufResultEncoder) EncodeDisconnectResult(res *DisconnectResult) ([]byte, error) {
	return res.Marshal()
}

// EncodePresenceResult ...
func (e *ProtobufResultEncoder) EncodePresenceResult(res *PresenceResult) ([]byte, error) {
	return res.Marshal()
}

// EncodePresenceStatsResult ...
func (e *ProtobufResultEncoder) EncodePresenceStatsResult(res *PresenceStatsResult) ([]byte, error) {
	return res.Marshal()
}

// EncodeHistoryResult ...
func (e *ProtobufResultEncoder) EncodeHistoryResult(res *HistoryResult) ([]byte, error) {
	return res.Marshal()
}

// EncodeChannelsResult ...
func (e *ProtobufResultEncoder) EncodeChannelsResult(res *ChannelsResult) ([]byte, error) {
	return res.Marshal()
}

// EncodeInfoResult ...
func (e *ProtobufResultEncoder) EncodeInfoResult(res *InfoResult) ([]byte, error) {
	return res.Marshal()
}
