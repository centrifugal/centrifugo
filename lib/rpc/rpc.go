package rpc

import (
	"context"

	"github.com/centrifugal/centrifugo/lib/proto"
)

// Request ...
type Request struct {
	Method string
	Params *proto.Raw
}

// Response ...
type Response struct {
	Error  *proto.Error
	Result *proto.Raw
}

// Handler must handle incoming command from client.
type Handler func(context.Context, *Request) (*Response, *proto.Disconnect)
