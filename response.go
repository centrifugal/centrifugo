package main

import (
	"encoding/json"
)

type response struct {
	Body   interface{}
	Error  string
	Uid    string
	Method string
}

type multiResponse []response

func (r *response) toJson() ([]byte, error) {
	return json.Marshal(r)
}

func (mr *multiResponse) toJson() ([]byte, error) {
	return json.Marshal(mr)
}
