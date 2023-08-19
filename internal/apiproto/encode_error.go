package apiproto

import "encoding/json"

func EncodeError(err *Error) ([]byte, error) {
	return json.Marshal(err)
}
