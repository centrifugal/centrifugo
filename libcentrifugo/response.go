package libcentrifugo

import (
	"encoding/json"
)

// response represents an answer Centrifugo sends
// to client or API request commands
type response struct {
	Body   interface{}
	Error  error
	Method string
}

func newResponse(method string) *response {
	return &response{
		Method: method,
	}
}

// specific MarshalJSON implementation for response to correctly serialize error
func (r *response) MarshalJSON() ([]byte, error) {
	var err interface{}
	if r.Error != nil {
		err = r.Error.Error()
	} else {
		err = nil
	}
	return json.Marshal(map[string]interface{}{
		"body":   r.Body,
		"error":  err,
		"method": r.Method,
	})
}

// multiResponse is a slice of responses in execution
// order - from first executed to last one
type multiResponse []*response

// toJson converts response into JSON
func (r *response) toJson() ([]byte, error) {
	return json.Marshal(r)
}

// toJson converts multiResponse into JSON
func (mr *multiResponse) toJson() ([]byte, error) {
	return json.Marshal(mr)
}
