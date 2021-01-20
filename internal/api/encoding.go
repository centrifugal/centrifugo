package api

import "sync"

// Encoding determines connection protocol encoding in use.
type Encoding string

const (
	// EncodingJSON means JSON protocol.
	EncodingJSON Encoding = "json"
	// EncodingProtobuf means protobuf protocol.
	EncodingProtobuf Encoding = "protobuf"
)

var (
	jsonReplyEncoderPool     sync.Pool
	protobufReplyEncoderPool sync.Pool
)

// GetReplyEncoder ...
func GetReplyEncoder(enc Encoding) ReplyEncoder {
	if enc == EncodingJSON {
		e := jsonReplyEncoderPool.Get()
		if e == nil {
			return NewJSONReplyEncoder()
		}
		encoder := e.(ReplyEncoder)
		encoder.Reset()
		return encoder
	}
	e := protobufReplyEncoderPool.Get()
	if e == nil {
		return NewProtobufReplyEncoder()
	}
	encoder := e.(ReplyEncoder)
	encoder.Reset()
	return encoder
}

// PutReplyEncoder ...
func PutReplyEncoder(enc Encoding, e ReplyEncoder) {
	if enc == EncodingJSON {
		jsonReplyEncoderPool.Put(e)
	}
	protobufReplyEncoderPool.Put(e)
}

// GetCommandDecoder ...
func GetCommandDecoder(enc Encoding, data []byte) CommandDecoder {
	if enc == EncodingJSON {
		return NewJSONCommandDecoder(data)
	}
	return NewProtobufCommandDecoder(data)
}

// PutCommandDecoder ...
func PutCommandDecoder(_ Encoding, _ CommandDecoder) {}

// GetDecoder ...
func GetDecoder(enc Encoding) Decoder {
	if enc == EncodingJSON {
		return NewJSONDecoder()
	}
	return NewProtobufDecoder()
}

// PutDecoder ...
func PutDecoder(_ Encoding, _ Decoder) {}

// GetEncoder ...
func GetEncoder(enc Encoding) Encoder {
	if enc == EncodingJSON {
		return NewJSONEncoder()
	}
	return NewProtobufEncoder()
}

// PutEncoder ...
func PutEncoder(_ Encoding, _ Encoder) {}
