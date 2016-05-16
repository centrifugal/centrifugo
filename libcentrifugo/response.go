package libcentrifugo

import (
	"bytes"
	"runtime"

	"github.com/centrifugal/centrifugo/libcentrifugo/encode"
	"github.com/oxtoacart/bpool"
)

// clientMessageResponse uses strong type for body instead of interface{} - helps to
// reduce allocations when marshaling. Also it does not have error - because message
// client response never contains it.
type clientMessageResponse struct {
	Method string  `json:"method"`
	Body   Message `json:"body"`
}

// newClientMessage returns initialized client message response.
func newClientMessage() *clientMessageResponse {
	return &clientMessageResponse{
		Method: "message",
	}
}

var bufpool *bpool.BufferPool

func init() {
	// Initialize buffer pool, this must be reasonably large because we can encode
	// messages into client JSON responses in different goroutines (and we already do
	// this when using Memory Engine).
	bufpool = bpool.NewBufferPool(runtime.NumCPU())
}

func writeClientInfo(buf *bytes.Buffer, info *ClientInfo) {
	buf.WriteString(`{`)

	if info.DefaultInfo != nil {
		buf.WriteString(`"default_info":`)
		buf.Write(*info.DefaultInfo)
		buf.WriteString(",")
	}

	if info.ChannelInfo != nil {
		buf.WriteString(`"channel_info":`)
		buf.Write(*info.ChannelInfo)
		buf.WriteString(`,`)
	}

	buf.WriteString(`"user":`)
	encode.EncodeJSONString(buf, info.User, true)
	buf.WriteString(`,`)

	buf.WriteString(`"client":`)
	encode.EncodeJSONString(buf, info.Client, true)

	buf.WriteString(`}`)
}

func writeMessage(buf *bytes.Buffer, message *Message) {
	buf.WriteString(`{"uid":"`)
	buf.WriteString(message.UID)
	buf.WriteString(`","timestamp":"`)
	buf.WriteString(message.Timestamp)
	buf.WriteString(`",`)

	if message.Client != "" {
		buf.WriteString(`"client":`)
		encode.EncodeJSONString(buf, message.Client, true)
		buf.WriteString(`,`)
	}

	if message.Info != nil {
		buf.WriteString(`"info":`)
		writeClientInfo(buf, message.Info)
		buf.WriteString(`,`)
	}

	buf.WriteString(`"channel":`)
	encode.EncodeJSONString(buf, message.Channel, true)
	buf.WriteString(`,"data":`)
	buf.Write(*message.Data)
	buf.WriteString(`}`)
}

func (m *clientMessageResponse) Marshal() ([]byte, error) {
	buf := bufpool.Get()
	defer bufpool.Put(buf)
	buf.WriteString(`{"method":"message","body":`)
	writeMessage(buf, &m.Body)
	buf.WriteString(`}`)
	return buf.Bytes(), nil
}

type clientJoinResponse struct {
	Method string      `json:"method"`
	Body   JoinMessage `json:"body"`
}

func newClientJoinMessage() *clientJoinResponse {
	return &clientJoinResponse{
		Method: "join",
	}
}

func writeJoin(buf *bytes.Buffer, message *JoinMessage) {
	buf.WriteString(`{`)
	buf.WriteString(`"channel":`)
	encode.EncodeJSONString(buf, message.Channel, true)
	buf.WriteString(`,"data":`)
	writeClientInfo(buf, &message.Data)
	buf.WriteString(`}`)
}

func (m *clientJoinResponse) Marshal() ([]byte, error) {
	buf := bufpool.Get()
	defer bufpool.Put(buf)
	buf.WriteString(`{"method":"join","body":`)
	writeJoin(buf, &m.Body)
	buf.WriteString(`}`)
	return buf.Bytes(), nil
}

type clientLeaveResponse struct {
	Method string       `json:"method"`
	Body   LeaveMessage `json:"body"`
}

func newClientLeaveMessage() *clientLeaveResponse {
	return &clientLeaveResponse{
		Method: "leave",
	}
}

func writeLeave(buf *bytes.Buffer, message *LeaveMessage) {
	buf.WriteString(`{`)
	buf.WriteString(`"channel":`)
	encode.EncodeJSONString(buf, message.Channel, true)
	buf.WriteString(`,"data":`)
	writeClientInfo(buf, &message.Data)
	buf.WriteString(`}`)
}

func (m *clientLeaveResponse) Marshal() ([]byte, error) {
	buf := bufpool.Get()
	defer bufpool.Put(buf)
	buf.WriteString(`{"method":"leave","body":`)
	writeLeave(buf, &m.Body)
	buf.WriteString(`}`)
	return buf.Bytes(), nil
}

type errorAdvice string

const (
	errorAdviceNone  errorAdvice = ""
	errorAdviceFix   errorAdvice = "fix"
	errorAdviceRetry errorAdvice = "retry"
)

type clientError struct {
	err    error
	Advice errorAdvice `json:"advice,omitempty"`
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

// presenceBody represents body of response in case of successful presence command.
type presenceBody struct {
	Channel Channel               `json:"channel"`
	Data    map[ConnID]ClientInfo `json:"data"`
}

// historyBody represents body of response in case of successful history command.
type historyBody struct {
	Channel Channel   `json:"channel"`
	Data    []Message `json:"data"`
}

// channelsBody represents body of response in case of successful channels command.
type channelsBody struct {
	Data []Channel `json:"data"`
}

// connectBody represents body of response in case of successful connect command.
type connectBody struct {
	Version string `json:"version"`
	Client  ConnID `json:"client"`
	Expires bool   `json:"expires"`
	Expired bool   `json:"expired"`
	TTL     int64  `json:"ttl"`
}

// subscribeBody represents body of response in case of successful subscribe command.
type subscribeBody struct {
	Channel   Channel   `json:"channel"`
	Status    bool      `json:"status"`
	Last      MessageID `json:"last"`
	Messages  []Message `json:"messages"`
	Recovered bool      `json:"recovered"`
}

// unsubscribeBody represents body of response in case of successful unsubscribe command.
type unsubscribeBody struct {
	Channel Channel `json:"channel"`
	Status  bool    `json:"status"`
}

// publishBody represents body of response in case of successful publish command.
type publishBody struct {
	Channel Channel `json:"channel"`
	Status  bool    `json:"status"`
}

// disconnectBody represents body of disconnect response when we want to tell
// client to disconnect. Optionally we can give client an advice to continue
// reconnecting after receiving this message.
type disconnectBody struct {
	Reason    string `json:"reason"`
	Reconnect bool   `json:"reconnect"`
}

// pingBody represents body of response in case of successful ping command.
type pingBody struct {
	Data string `json:"data"`
}

// statsBody represents body of response in case of successful stats command.
type statsBody struct {
	Data serverStats `json:"data"`
}

// nodeBody represents body of response in case of successful node command.
type nodeBody struct {
	Data nodeInfo `json:"data"`
}

type adminMessageBody struct {
	Message Message `json:"message"`
}
