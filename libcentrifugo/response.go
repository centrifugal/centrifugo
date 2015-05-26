package libcentrifugo

// response represents an answer Centrifugo sends
// to client or API request commands
type response struct {
	Body   interface{} `json:"body"`
	Error  *string     `json:"error"`
	Method string      `json:"method"`
	err    error       // Use response.Err() to set.
}

// Err set an error message on the response
// and updates the 'err' field in the response.
// If an error has already been set it will be kept.
// Will return true if an error has been set previously,
// or if an error is sent.
func (r *response) Err(err error) bool {
	if r.err != nil {
		return true
	}
	if err == nil {
		return false
	}
	//TODO: Add logging here? (klauspost)
	e := err.Error()
	r.Error = &e
	r.err = err
	return true
}

func newResponse(method string) *response {
	return &response{
		Method: method,
	}
}

// multiResponse is a slice of responses in execution
// order - from first executed to last one
type multiResponse []*response
