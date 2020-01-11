package api

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
)

// ReplyEncoder ...
type ReplyEncoder interface {
	Reset()
	Encode(*Reply) error
	Finish() []byte
}

// JSONReplyEncoder ...
type JSONReplyEncoder struct {
	buffer bytes.Buffer
}

// NewJSONReplyEncoder ...
func NewJSONReplyEncoder() *JSONReplyEncoder {
	return &JSONReplyEncoder{}
}

// Reset ...
func (e *JSONReplyEncoder) Reset() {
	e.buffer.Reset()
}

// Encode ...
func (e *JSONReplyEncoder) Encode(r *Reply) error {
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}
	e.buffer.Write(data)
	e.buffer.WriteString("\n")
	return nil
}

// Finish ...
func (e *JSONReplyEncoder) Finish() []byte {
	data := e.buffer.Bytes()
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)
	return dataCopy
}

// ProtobufReplyEncoder ...
type ProtobufReplyEncoder struct {
	buffer bytes.Buffer
}

// NewProtobufReplyEncoder ...
func NewProtobufReplyEncoder() *ProtobufReplyEncoder {
	return &ProtobufReplyEncoder{}
}

// Encode ...
func (e *ProtobufReplyEncoder) Encode(r *Reply) error {
	replyBytes, err := r.Marshal()
	if err != nil {
		return err
	}
	bs := make([]byte, 8)
	n := binary.PutUvarint(bs, uint64(len(replyBytes)))
	e.buffer.Write(bs[:n])
	e.buffer.Write(replyBytes)
	return nil
}

// Reset ...
func (e *ProtobufReplyEncoder) Reset() {
	e.buffer.Reset()
}

// Finish ...
func (e *ProtobufReplyEncoder) Finish() []byte {
	data := e.buffer.Bytes()
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)
	return dataCopy
}

// Encoder ...
type Encoder interface {
	EncodePublish(*PublishResult) ([]byte, error)
	EncodeBroadcast(*BroadcastResult) ([]byte, error)
	EncodeUnsubscribe(*UnsubscribeResult) ([]byte, error)
	EncodeDisconnect(*DisconnectResult) ([]byte, error)
	EncodePresence(*PresenceResult) ([]byte, error)
	EncodePresenceStats(*PresenceStatsResult) ([]byte, error)
	EncodeHistory(*HistoryResult) ([]byte, error)
	EncodeHistoryRemove(*HistoryRemoveResult) ([]byte, error)
	EncodeChannels(*ChannelsResult) ([]byte, error)
	EncodeInfo(*InfoResult) ([]byte, error)
}

// JSONEncoder ...
type JSONEncoder struct{}

// NewJSONEncoder ...
func NewJSONEncoder() *JSONEncoder {
	return &JSONEncoder{}
}

// EncodePublish ...
func (e *JSONEncoder) EncodePublish(res *PublishResult) ([]byte, error) {
	return json.Marshal(res)
}

// EncodeBroadcast ...
func (e *JSONEncoder) EncodeBroadcast(res *BroadcastResult) ([]byte, error) {
	return json.Marshal(res)
}

// EncodeUnsubscribe ...
func (e *JSONEncoder) EncodeUnsubscribe(res *UnsubscribeResult) ([]byte, error) {
	return json.Marshal(res)
}

// EncodeDisconnect ...
func (e *JSONEncoder) EncodeDisconnect(res *DisconnectResult) ([]byte, error) {
	return json.Marshal(res)
}

// EncodePresence ...
func (e *JSONEncoder) EncodePresence(res *PresenceResult) ([]byte, error) {
	return json.Marshal(res)
}

// EncodePresenceStats ...
func (e *JSONEncoder) EncodePresenceStats(res *PresenceStatsResult) ([]byte, error) {
	return json.Marshal(res)
}

// EncodeHistory ...
func (e *JSONEncoder) EncodeHistory(res *HistoryResult) ([]byte, error) {
	return json.Marshal(res)
}

// EncodeHistoryRemove ...
func (e *JSONEncoder) EncodeHistoryRemove(res *HistoryRemoveResult) ([]byte, error) {
	return json.Marshal(res)
}

// EncodeChannels ...
func (e *JSONEncoder) EncodeChannels(res *ChannelsResult) ([]byte, error) {
	return json.Marshal(res)
}

// EncodeInfo ...
func (e *JSONEncoder) EncodeInfo(res *InfoResult) ([]byte, error) {
	return json.Marshal(res)
}

// ProtobufEncoder ...
type ProtobufEncoder struct{}

// NewProtobufEncoder ...
func NewProtobufEncoder() *ProtobufEncoder {
	return &ProtobufEncoder{}
}

// EncodePublish ...
func (e *ProtobufEncoder) EncodePublish(res *PublishResult) ([]byte, error) {
	return res.Marshal()
}

// EncodeBroadcast ...
func (e *ProtobufEncoder) EncodeBroadcast(res *BroadcastResult) ([]byte, error) {
	return res.Marshal()
}

// EncodeUnsubscribe ...
func (e *ProtobufEncoder) EncodeUnsubscribe(res *UnsubscribeResult) ([]byte, error) {
	return res.Marshal()
}

// EncodeDisconnect ...
func (e *ProtobufEncoder) EncodeDisconnect(res *DisconnectResult) ([]byte, error) {
	return res.Marshal()
}

// EncodePresence ...
func (e *ProtobufEncoder) EncodePresence(res *PresenceResult) ([]byte, error) {
	return res.Marshal()
}

// EncodePresenceStats ...
func (e *ProtobufEncoder) EncodePresenceStats(res *PresenceStatsResult) ([]byte, error) {
	return res.Marshal()
}

// EncodeHistory ...
func (e *ProtobufEncoder) EncodeHistory(res *HistoryResult) ([]byte, error) {
	return res.Marshal()
}

// EncodeHistoryRemove ...
func (e *ProtobufEncoder) EncodeHistoryRemove(res *HistoryRemoveResult) ([]byte, error) {
	return res.Marshal()
}

// EncodeChannels ...
func (e *ProtobufEncoder) EncodeChannels(res *ChannelsResult) ([]byte, error) {
	return res.Marshal()
}

// EncodeInfo ...
func (e *ProtobufEncoder) EncodeInfo(res *InfoResult) ([]byte, error) {
	return res.Marshal()
}
