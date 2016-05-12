package libcentrifugo

import (
	"bytes"
	"runtime"

	"github.com/centrifugal/centrifugo/libcentrifugo/encode"
	"github.com/oxtoacart/bpool"
)

// ClientMessageResponse uses strong type for body instead of interface{} - helps to
// reduce allocations when marshaling. Also it does not have error - because message
// client response never contains it.
type ClientMessageResponse struct {
	Method string  `json:"method"`
	Body   Message `json:"body"`
}

// newClientMessage returns initialized client message response.
func newClientMessage() *ClientMessageResponse {
	return &ClientMessageResponse{
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
	buf.WriteString("{")

	if info.DefaultInfo != nil {
		buf.WriteString("\"default_info\":")
		buf.Write(*info.DefaultInfo)
		buf.WriteString(",")
	}

	if info.ChannelInfo != nil {
		buf.WriteString("\"channel_info\":")
		buf.Write(*info.ChannelInfo)
		buf.WriteString(",")
	}

	buf.WriteString("\"user\":")
	encode.EncodeJSONString(buf, info.User, true)
	buf.WriteString(",")

	buf.WriteString("\"client\":")
	encode.EncodeJSONString(buf, info.Client, true)

	buf.WriteString("}")
}

func writeMessage(buf *bytes.Buffer, message *Message) {
	buf.WriteString("{")
	buf.WriteString("\"uid\":")
	encode.EncodeJSONString(buf, message.UID, true)
	buf.WriteString(",")

	buf.WriteString("\"timestamp\":")
	encode.EncodeJSONString(buf, message.Timestamp, true)
	buf.WriteString(",")

	if message.Client != "" {
		buf.WriteString("\"client\":")
		encode.EncodeJSONString(buf, message.Client, true)
		buf.WriteString(",")
	}

	if message.Info != nil {
		buf.WriteString("\"info\":")
		writeClientInfo(buf, message.Info)
		buf.WriteString(",")
	}

	buf.WriteString("\"channel\":")
	encode.EncodeJSONString(buf, message.Channel, true)
	buf.WriteString(",")

	buf.WriteString("\"data\":")
	buf.Write(*message.Data)
	buf.WriteString("}")
}

func (m *ClientMessageResponse) Marshal() ([]byte, error) {
	buf := bufpool.Get()
	defer bufpool.Put(buf)
	buf.WriteString("{\"method\":\"message\",\"body\":")
	writeMessage(buf, &m.Body)
	buf.WriteString("}")
	return buf.Bytes(), nil
}

type ClientJoinResponse struct {
	Method string           `json:"method"`
	Body   JoinLeaveMessage `json:"body"`
}

func newClientJoinMessage() *ClientJoinResponse {
	return &ClientJoinResponse{
		Method: "join",
	}
}

func writeJoinLeave(buf *bytes.Buffer, message *JoinLeaveMessage) {
	buf.WriteString("{")

	buf.WriteString("\"channel\":")
	encode.EncodeJSONString(buf, message.Channel, true)
	buf.WriteString(",")

	buf.WriteString("\"data\":")
	writeClientInfo(buf, &message.Data)

	buf.WriteString("}")
}

func (m *ClientJoinResponse) Marshal() ([]byte, error) {
	buf := bufpool.Get()
	defer bufpool.Put(buf)
	buf.WriteString("{\"method\":\"join\",\"body\":")
	writeJoinLeave(buf, &m.Body)
	buf.WriteString("}")
	return buf.Bytes(), nil
}

type ClientLeaveResponse struct {
	Method string           `json:"method"`
	Body   JoinLeaveMessage `json:"body"`
}

func newClientLeaveMessage() *ClientLeaveResponse {
	return &ClientLeaveResponse{
		Method: "leave",
	}
}

func (m *ClientLeaveResponse) Marshal() ([]byte, error) {
	buf := bufpool.Get()
	defer bufpool.Put(buf)
	buf.WriteString("{\"method\":\"leave\",\"body\":")
	writeJoinLeave(buf, &m.Body)
	buf.WriteString("}")
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

// NodeBody represents body of response in case of successful node command.
type NodeBody struct {
	Data NodeInfo `json:"data"`
}

type adminMessageBody struct {
	Message Message `json:"message"`
}
