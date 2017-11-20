package client

import (
	"encoding/json"

	"github.com/centrifugal/centrifugo/libcentrifugo/raw"
	"github.com/centrifugal/centrifugo/libcentrifugo/v2/proto"
)

// Error ...
type Error struct {
	Code    int
	Message string
}

// Response ...
type Response struct {
	ID     int     `json:"i,omitempty"`
	Err    *Error  `json:"e,omitempty"`
	Result raw.Raw `json:"r,omitempty"`
}

// NewResponse ...
func NewResponse(id int, err *Error, res raw.Raw) *Response {
	return &Response{
		ID:     id,
		Err:    err,
		Result: res,
	}
}

// NewAsyncResponse ...
func NewAsyncResponse(res raw.Raw) *Response {
	return &Response{
		Result: res,
	}
}

// MultiResponse is a slice of responses in execution order - from first
// executed to last one.
type MultiResponse []Response

// AsyncMessage ...
type AsyncMessage struct {
	Type AsyncMessageType `json:"t"`
	Data raw.Raw          `json:"d"`
}

// AsyncMessageType ...
type AsyncMessageType string

var (
	// AsyncDataMessage ...
	AsyncDataMessage AsyncMessageType = "m"
	// AsyncJoinMessage ...
	AsyncJoinMessage AsyncMessageType = "j"
	// AsyncLeaveMessage ...
	AsyncLeaveMessage AsyncMessageType = "l"
)

// NewDataMessage returns initialized async data message.
func NewDataMessage(msg *proto.Message) *AsyncMessage {
	data, _ := json.Marshal(msg)
	return &AsyncMessage{
		Type: AsyncDataMessage,
		Data: data,
	}
}

// NewJoinMessage returns initialized async join message.
func NewJoinMessage(msg *proto.JoinMessage) *AsyncMessage {
	data, _ := json.Marshal(msg)
	return &AsyncMessage{
		Type: AsyncJoinMessage,
		Data: data,
	}
}

// NewLeaveMessage returns initialized async leave message.
func NewLeaveMessage(msg *proto.JoinMessage) *AsyncMessage {
	data, _ := json.Marshal(msg)
	return &AsyncMessage{
		Type: AsyncJoinMessage,
		Data: data,
	}
}

// PresenceResult for presence command.
type PresenceResult struct {
	Data map[string]*proto.Info `json:"data"`
}

// HistoryResult for history command.
type HistoryResult struct {
	Data []proto.Message `json:"data"`
}

// ConnectResult for connect command.
type ConnectResult struct {
	Version string `json:"version"`
	Client  string `json:"client"`
	Expires bool   `json:"expires,omitempty"`
	Expired bool   `json:"expired,omitempty"`
	TTL     int64  `json:"ttl,omitempty"`
}

// SubscribeResult for subscribe command.
type SubscribeResult struct {
	Last      string          `json:"last,omitempty"`
	Messages  []proto.Message `json:"messages,omitempty"`
	Recovered bool            `json:"recovered,omitempty"`
}

// UnsubscribeResult for unsubscribe command.
type UnsubscribeResult struct{}

// PublishResult for publish command.
type PublishResult struct{}

// PingResult for ping command.
type PingResult struct {
	Data string `json:"data,omitempty"`
}
