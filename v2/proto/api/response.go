package server

import (
	"github.com/centrifugal/centrifugo/libcentrifugo/raw"
	"github.com/centrifugal/centrifugo/libcentrifugo/v2/proto"
)

// Error ...
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
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

// MultiResponse is a slice of responses in execution order - from first
// executed to last one.
type MultiResponse []Response

// PresenceResult for presence command.
type PresenceResult struct {
	Data map[string]*proto.Info `json:"data"`
}

// HistoryResult for history command.
type HistoryResult struct {
	Data []proto.Message `json:"data"`
}

// ChannelsResult for channels command.
type ChannelsResult struct {
	Data []string `json:"data"`
}

// StatsResult for stats command.
type StatsResult struct {
	Data Stats `json:"data"`
}

// NodeBody represents body of response in case of successful node command.
type NodeBody struct {
	Data NodeInfo `json:"data"`
}
