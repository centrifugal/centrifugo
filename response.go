package main

import (
	"encoding/json"
)

// response represents an answer Centrifuge sends
// to client or API request commands
type response struct {
	Body   interface{}
	Error  string
	Method string
}

// multiResponse is a slice of responses in execution
// order - from first executed to last one
type multiResponse []response

// toJson converts response into JSON
func (r *response) toJson() ([]byte, error) {
	return json.Marshal(r)
}

// toJson converts multiResponse into JSON
func (mr *multiResponse) toJson() ([]byte, error) {
	return json.Marshal(mr)
}
