package libcentrifugo

type errorAdvice string

const (
	errorAdviceNone  errorAdvice = ""
	errorAdviceFix   errorAdvice = "fix"
	errorAdviceRetry errorAdvice = "retry"
)

type clientError struct {
	err         error
	ErrorAdvice errorAdvice `json:"error_advice,omitempty"`
}

// clientResponse represents an answer Centrifugo sends to client request
// commands or protocol messages sent to client asynchronously.
type clientResponse struct {
	UID    string      `json:"uid,omitempty"`
	Body   interface{} `json:"body"`
	Method string      `json:"method"`
	Error  string      `json:"error,omitempty"` // Use clientResponse.Err() to set.
	clientError
}

// clientMessageResponse uses strong type for body instead of interface{} - helps to
// reduce allocations when marshaling.
type clientMessageResponse struct {
	clientResponse
	Body Message `json:"body"`
}

// newClientMessage returns initialized client message response.
func newClientMessage() *clientMessageResponse {
	return &clientMessageResponse{
		clientResponse: clientResponse{
			Method: "message",
		},
	}
}

// newClientResponse returns client response initialized with provided method.
// Setting other client response fields is a caller responsibility.
func newClientResponse(method string) *clientResponse {
	return &clientResponse{
		Method: method,
	}
}

// Err set a client error on the client response and updates the 'err'
// field in the response. If an error has already been set it will be kept.
func (r *clientResponse) Err(err clientError) {
	if r.clientError.err != nil {
		// error already set.
		return
	}
	r.clientError = err
	e := err.err.Error()
	r.Error = e
}

// multiClientResponse is a slice of responses in execution order - from first
// executed to last one
type multiClientResponse []*clientResponse

// response represents an answer Centrifugo sends to API request commands
type response struct {
	UID    string      `json:"uid,omitempty"`
	Body   interface{} `json:"body"`
	Error  *string     `json:"error"`
	Method string      `json:"method"`
	err    error       // Use response.Err() to set.
}

// Err set an error message on the response and updates the 'err' field in
// the response. If an error has already been set it will be kept.
func (r *response) Err(err error) {
	if r.err != nil {
		return
	}
	// TODO: Add logging here? (klauspost)
	e := err.Error()
	r.Error = &e
	r.err = err
	return
}

func newResponse(method string) *response {
	return &response{
		Method: method,
	}
}

// multiResponse is a slice of responses in execution
// order - from first executed to last one
type multiResponse []*response

// PresenceBody represents body of response in case of successful presence command.
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
	Channel   Channel   `json:"channel"`
	Status    bool      `json:"status"`
	Last      MessageID `json:"last"`
	Messages  []Message `json:"messages"`
	Recovered bool      `json:"recovered"`
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

// DisconnectBody represents body of disconnect response when we want to tell
// client to disconnect. Optionally we can give client an advice to continue
// reconnecting after receiving this message.
type DisconnectBody struct {
	Reason    string `json:"reason"`
	Reconnect bool   `json:"reconnect"`
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
