package apiproto

import (
	"sync"
)

var (
	jsonReplyEncoderPool   sync.Pool
	jsonCommandDecoderPool sync.Pool
)

// GetReplyEncoder ...
func GetReplyEncoder() ReplyEncoder {
	e := jsonReplyEncoderPool.Get()
	if e == nil {
		return NewJSONReplyEncoder()
	}
	return e.(ReplyEncoder)
}

// PutReplyEncoder ...
func PutReplyEncoder(e ReplyEncoder) {
	e.Reset()
	jsonReplyEncoderPool.Put(e)
}

// GetCommandDecoder ...
func GetCommandDecoder(data []byte) CommandDecoder {
	e := jsonCommandDecoderPool.Get()
	if e == nil {
		return NewJSONCommandDecoder(data)
	}
	decoder := e.(CommandDecoder)
	decoder.Reset(data)
	return decoder
}

// PutCommandDecoder ...
func PutCommandDecoder(d CommandDecoder) {
	jsonCommandDecoderPool.Put(d)
}

// GetParamsDecoder ...
func GetParamsDecoder() RequestDecoder {
	return NewJSONRequestDecoder()
}

// PutParamsDecoder ...
func PutParamsDecoder(_ RequestDecoder) {}

// GetResultEncoder ...
func GetResultEncoder() ResultEncoder {
	return NewJSONResultEncoder()
}

// PutResultEncoder ...
func PutResultEncoder(_ ResultEncoder) {}
