package libcentrifugo

// response represents an answer Centrifugo sends
// to client or API request commands
type response struct {
	UID    string      `json:"uid,omitempty"`
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

// PresenseBody represents body of response in case of successful presence command.
type PresenceBody struct {
	Channel Channel               `json:"channel"`
	Data    map[ConnID]ClientInfo `json:"data"`
}

// HistoryBody represents body of response in case of successful history command.
type HistoryBody struct {
	Channel Channel   `json:"channel"`
	Data    []Message `json:"data"`
}

// ChannelsBody represents body of response in case of successful channels command.
type ChannelsBody struct {
	Data []Channel `json:"data"`
}

// JoinLeaveBody represents body of response when join or leave async response sent to client.
type JoinLeaveBody struct {
	Channel Channel    `json:"channel"`
	Data    ClientInfo `json:"data"`
}

// ConnectBody represents body of response in case of successful connect command.
type ConnectBody struct {
	Version string `json:"version"`
	Client  ConnID `json:"client"`
	Expires bool   `json:"expires"`
	Expired bool   `json:"expired"`
	TTL     int64  `json:"ttl"`
}

// SubscribeBody represents body of response in case of successful subscribe command.
type SubscribeBody struct {
	Channel Channel `json:"channel"`
	Status  bool    `json:"status"`
}

// UnsubscribeBody represents body of response in case of successful unsubscribe command.
type UnsubscribeBody struct {
	Channel Channel `json:"channel"`
	Status  bool    `json:"status"`
}

// PublishBody represents body of response in case of successful publish command.
type PublishBody struct {
	Channel Channel `json:"channel"`
	Status  bool    `json:"status"`
}

// PingBody represents body of response in case of successful ping command.
type PingBody struct {
	Data string `json:"data"`
}

// StatsBody represents body of response in case of successful stats command.
type StatsBody struct {
	Data Stats `json:"data"`
}

type adminMessageBody struct {
	Message Message `json:"message"`
}
