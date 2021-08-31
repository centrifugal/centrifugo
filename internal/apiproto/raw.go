package apiproto

import (
	"errors"
)

// Raw type used by Centrifugo as type for fields in structs which value
// we want to left untouched. For example custom application specific JSON
// payload data in published message. This is very similar to json.RawMessage
// type.
type Raw []byte

// MarshalJSON returns *r as the JSON encoding of r.
func (r Raw) MarshalJSON() ([]byte, error) {
	if r == nil {
		return []byte("null"), nil
	}
	return r, nil
}

// UnmarshalJSON sets *r to a copy of data.
func (r *Raw) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("raw: UnmarshalJSON on nil pointer")
	}
	*r = append((*r)[0:0], data...)
	return nil
}
