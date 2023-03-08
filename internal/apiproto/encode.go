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

// ResponseEncoder ...
type ResponseEncoder interface {
	EncodePublish(*PublishResponse) ([]byte, error)
	EncodeBroadcast(*BroadcastResponse) ([]byte, error)
	EncodeSubscribe(*SubscribeResponse) ([]byte, error)
	EncodeUnsubscribe(*UnsubscribeResponse) ([]byte, error)
	EncodeDisconnect(*DisconnectResponse) ([]byte, error)
	EncodePresence(*PresenceResponse) ([]byte, error)
	EncodePresenceStats(*PresenceStatsResponse) ([]byte, error)
	EncodeHistory(*HistoryResponse) ([]byte, error)
	EncodeHistoryRemove(*HistoryRemoveResponse) ([]byte, error)
	EncodeInfo(*InfoResponse) ([]byte, error)
	EncodeRPC(*RPCResponse) ([]byte, error)
	EncodeRefresh(*RefreshResponse) ([]byte, error)
	EncodeChannels(*ChannelsResponse) ([]byte, error)
	EncodeBatch(response *BatchResponse) ([]byte, error)
}
