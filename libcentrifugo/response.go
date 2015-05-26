package libcentrifugo

// response represents an answer Centrifugo sends
// to client or API request commands
type response struct {
	Body   interface{} `json:"body"`
	Error  *string     `json:"error"`
	Method string      `json:"method"`
	err    error
}

// Err set an error message on the response
// and updates the 'err' field in the response.
// Overrides any previous set error.
func (r *response) Err(err error) {
	if err == nil {
		r.Error = nil
		r.err = nil
		return
	}
	e := err.Error()
	r.Error = &e
	r.err = err
}

func newResponse(method string) *response {
	return &response{
		Method: method,
	}
}

// multiResponse is a slice of responses in execution
// order - from first executed to last one
type multiResponse []*response
