package apiproto

import (
	"bytes"
	"encoding/json"
)

// ReplyEncoder ...
type ReplyEncoder interface {
	Reset()
	Encode(*Reply) error
	Finish() []byte
}

var _ ReplyEncoder = (*JSONReplyEncoder)(nil)

// JSONReplyEncoder ...
type JSONReplyEncoder struct {
	count  int
	buffer bytes.Buffer
}

// NewJSONReplyEncoder ...
func NewJSONReplyEncoder() *JSONReplyEncoder {
	return &JSONReplyEncoder{}
}

// Reset ...
func (e *JSONReplyEncoder) Reset() {
	e.count = 0
	e.buffer.Reset()
}

// Encode ...
func (e *JSONReplyEncoder) Encode(r *Reply) error {
	if e.count > 0 {
		e.buffer.WriteString("\n")
	}
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}
	e.buffer.Write(data)
	e.count++
	return nil
}

// Finish ...
func (e *JSONReplyEncoder) Finish() []byte {
	data := e.buffer.Bytes()
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)
	return dataCopy
}

// ResultEncoder ...
type ResultEncoder interface {
	EncodePublish(*PublishResult) ([]byte, error)
	EncodeBroadcast(*BroadcastResult) ([]byte, error)
	EncodeSubscribe(*SubscribeResult) ([]byte, error)
	EncodeUnsubscribe(*UnsubscribeResult) ([]byte, error)
	EncodeDisconnect(*DisconnectResult) ([]byte, error)
	EncodePresence(*PresenceResult) ([]byte, error)
	EncodePresenceStats(*PresenceStatsResult) ([]byte, error)
	EncodeHistory(*HistoryResult) ([]byte, error)
	EncodeHistoryRemove(*HistoryRemoveResult) ([]byte, error)
	EncodeInfo(*InfoResult) ([]byte, error)
	EncodeRPC(*RPCResult) ([]byte, error)
	EncodeRefresh(*RefreshResult) ([]byte, error)
	EncodeChannels(*ChannelsResult) ([]byte, error)
}

var _ ResultEncoder = (*JSONResultEncoder)(nil)

// JSONResultEncoder ...
type JSONResultEncoder struct{}

// NewJSONEncoder ...
func NewJSONEncoder() *JSONResultEncoder {
	return &JSONResultEncoder{}
}

// EncodePublish ...
func (e *JSONResultEncoder) EncodePublish(res *PublishResult) ([]byte, error) {
	return json.Marshal(res)
}

// EncodeBroadcast ...
func (e *JSONResultEncoder) EncodeBroadcast(res *BroadcastResult) ([]byte, error) {
	return json.Marshal(res)
}

// EncodeSubscribe ...
func (e *JSONResultEncoder) EncodeSubscribe(res *SubscribeResult) ([]byte, error) {
	//nolint:staticcheck
	return json.Marshal(res)
}

// EncodeUnsubscribe ...
func (e *JSONResultEncoder) EncodeUnsubscribe(res *UnsubscribeResult) ([]byte, error) {
	//nolint:staticcheck
	return json.Marshal(res)
}

// EncodeDisconnect ...
func (e *JSONResultEncoder) EncodeDisconnect(res *DisconnectResult) ([]byte, error) {
	//nolint:staticcheck
	return json.Marshal(res)
}

// EncodePresence ...
func (e *JSONResultEncoder) EncodePresence(res *PresenceResult) ([]byte, error) {
	return json.Marshal(res)
}

// EncodePresenceStats ...
func (e *JSONResultEncoder) EncodePresenceStats(res *PresenceStatsResult) ([]byte, error) {
	return json.Marshal(res)
}

// EncodeHistory ...
func (e *JSONResultEncoder) EncodeHistory(res *HistoryResult) ([]byte, error) {
	return json.Marshal(res)
}

// EncodeHistoryRemove ...
func (e *JSONResultEncoder) EncodeHistoryRemove(res *HistoryRemoveResult) ([]byte, error) {
	//nolint:staticcheck
	return json.Marshal(res)
}

// EncodeInfo ...
func (e *JSONResultEncoder) EncodeInfo(res *InfoResult) ([]byte, error) {
	return json.Marshal(res)
}

// EncodeRPC ...
func (e *JSONResultEncoder) EncodeRPC(res *RPCResult) ([]byte, error) {
	return json.Marshal(res)
}

// EncodeRefresh ...
func (e *JSONResultEncoder) EncodeRefresh(res *RefreshResult) ([]byte, error) {
	//nolint:staticcheck
	return json.Marshal(res)
}

// EncodeChannels ...
func (e *JSONResultEncoder) EncodeChannels(res *ChannelsResult) ([]byte, error) {
	//nolint:staticcheck
	return json.Marshal(res)
}
