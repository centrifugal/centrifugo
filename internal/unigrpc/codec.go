package unigrpc

import (
	"fmt"

	"google.golang.org/protobuf/proto"
)

type rawFrame []byte

// RawCodec allows sending Protobuf encoded Pushes without
// additional wrapping and marshaling.
type RawCodec struct{}

func (c *RawCodec) Marshal(v any) ([]byte, error) {
	out, ok := v.(rawFrame)
	if !ok {
		vv, ok := v.(proto.Message)
		if !ok {
			return nil, fmt.Errorf("failed to marshal, message is %T, want proto.Message", v)
		}
		return proto.Marshal(vv)
	}
	return out, nil
}

func (c *RawCodec) Unmarshal(data []byte, v any) error {
	vv, ok := v.(proto.Message)
	if !ok {
		return fmt.Errorf("failed to unmarshal, message is %T, want proto.Message", v)
	}
	return proto.Unmarshal(data, vv)
}

func (c *RawCodec) String() string {
	return "proto"
}

func (c *RawCodec) Name() string {
	return "proto"
}
