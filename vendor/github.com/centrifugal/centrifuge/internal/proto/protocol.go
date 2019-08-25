package proto

import "sync"

// ProtocolType determines connection protocol protoTypeoding type.
type ProtocolType string

const (
	// ProtocolTypeJSON means JSON protocol.
	ProtocolTypeJSON ProtocolType = "json"
	// ProtocolTypeProtobuf means protobuf protocol.
	ProtocolTypeProtobuf ProtocolType = "protobuf"
)

// GetPushEncoder ...
func GetPushEncoder(protoType ProtocolType) PushEncoder {
	if protoType == ProtocolTypeJSON {
		return NewJSONPushEncoder()
	}
	return NewProtobufPushEncoder()
}

var (
	jsonReplyEncoderPool     sync.Pool
	protobufReplyEncoderPool sync.Pool
)

// GetReplyEncoder ...
func GetReplyEncoder(protoType ProtocolType) ReplyEncoder {
	if protoType == ProtocolTypeJSON {
		e := jsonReplyEncoderPool.Get()
		if e == nil {
			return NewJSONReplyEncoder()
		}
		protoTypeoder := e.(ReplyEncoder)
		protoTypeoder.Reset()
		return protoTypeoder
	}
	e := protobufReplyEncoderPool.Get()
	if e == nil {
		return NewProtobufReplyEncoder()
	}
	protoTypeoder := e.(ReplyEncoder)
	protoTypeoder.Reset()
	return protoTypeoder
}

// PutReplyEncoder ...
func PutReplyEncoder(protoType ProtocolType, e ReplyEncoder) {
	if protoType == ProtocolTypeJSON {
		jsonReplyEncoderPool.Put(e)
		return
	}
	protobufReplyEncoderPool.Put(e)
}

// GetCommandDecoder ...
func GetCommandDecoder(protoType ProtocolType, data []byte) CommandDecoder {
	if protoType == ProtocolTypeJSON {
		return NewJSONCommandDecoder(data)
	}
	return NewProtobufCommandDecoder(data)
}

// PutCommandDecoder ...
func PutCommandDecoder(protoType ProtocolType, e CommandDecoder) {
	return
}

// GetResultEncoder ...
func GetResultEncoder(protoType ProtocolType) ResultEncoder {
	if protoType == ProtocolTypeJSON {
		return NewJSONResultEncoder()
	}
	return NewProtobufResultEncoder()
}

// PutResultEncoder ...
func PutResultEncoder(protoType ProtocolType, e ReplyEncoder) {
	return
}

// GetParamsDecoder ...
func GetParamsDecoder(protoType ProtocolType) ParamsDecoder {
	if protoType == ProtocolTypeJSON {
		return NewJSONParamsDecoder()
	}
	return NewProtobufParamsDecoder()
}

// PutParamsDecoder ...
func PutParamsDecoder(protoType ProtocolType, e ParamsDecoder) {
	return
}
