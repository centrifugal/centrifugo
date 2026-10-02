package tools

import (
	"github.com/centrifugal/protocol/cfjson"
)

// UnmarshalJSONPtr decodes data into *p the way encoding/json decodes into a
// pointer: null sets *p to nil, anything else is decoded into *p, which is
// allocated if it is nil.
func UnmarshalJSONPtr[T any, PT interface {
	*T
	cfjson.Decoder
}](data []byte, p **T) error {
	if i := cfjson.SkipSpace(data, 0); len(data)-i >= 4 && string(data[i:i+4]) == "null" {
		if cfjson.SkipSpace(data, i+4) != len(data) {
			return cfjson.ErrTrailingData
		}
		*p = nil
		return nil
	}
	if *p == nil {
		*p = new(T)
	}
	return cfjson.Unmarshal(data, PT(*p), 0)
}
