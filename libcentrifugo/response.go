package libcentrifugo

import (
	"encoding/json"
)

// response represents an answer Centrifuge sends
// to client or API request commands
type response struct {
	Body   interface{} `json:"body"`
	Error  error       `json:"error"`
	Method string      `json:"method"`
}

func newResponse(method string) *response {
	return &response{
		Body:   nil,
		Error:  nil,
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
