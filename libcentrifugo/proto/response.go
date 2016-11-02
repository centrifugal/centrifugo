package proto

import (
	"github.com/centrifugal/centrifugo/libcentrifugo/raw"
	"github.com/valyala/bytebufferpool"
)

// ClientMessageResponse can not have an error.
type ClientMessageResponse struct {
	Method string  `json:"method"`
	Body   Message `json:"body"`
}

// NewClientMessage returns initialized client message response.
func NewClientMessage(msg *Message) *ClientMessageResponse {
	return &ClientMessageResponse{
		Method: "message",
		Body:   *msg,
	}
}

func writeClientInfo(buf *bytebufferpool.ByteBuffer, info *ClientInfo) {
	buf.WriteString(`{`)

	if info.DefaultInfo != nil {
		buf.WriteString(`"default_info":`)
		buf.Write(info.DefaultInfo)
		buf.WriteString(",")
	}

	if info.ChannelInfo != nil {
		buf.WriteString(`"channel_info":`)
		buf.Write(info.ChannelInfo)
		buf.WriteString(`,`)
	}

	buf.WriteString(`"user":`)
	EncodeJSONString(buf, info.User, true)
	buf.WriteString(`,`)

	buf.WriteString(`"client":`)
	EncodeJSONString(buf, info.Client, true)

	buf.WriteString(`}`)
}

func writeMessage(buf *bytebufferpool.ByteBuffer, msg *Message) {
	buf.WriteString(`{"uid":"`)
	buf.WriteString(msg.UID)
	buf.WriteString(`","timestamp":"`)
	buf.WriteString(msg.Timestamp)
	buf.WriteString(`",`)

	if msg.Client != "" {
		buf.WriteString(`"client":`)
		EncodeJSONString(buf, msg.Client, true)
		buf.WriteString(`,`)
	}

	if msg.Info != nil {
		buf.WriteString(`"info":`)
		writeClientInfo(buf, msg.Info)
		buf.WriteString(`,`)
	}

	buf.WriteString(`"channel":`)
	EncodeJSONString(buf, msg.Channel, true)
	buf.WriteString(`,"data":`)
	buf.Write(msg.Data)
	buf.WriteString(`}`)
}

func (m *ClientMessageResponse) Marshal() ([]byte, error) {
	buf := bytebufferpool.Get()
	buf.WriteString(`{"method":"message","body":`)
	writeMessage(buf, &m.Body)
	buf.WriteString(`}`)
	c := make([]byte, buf.Len())
	copy(c, buf.Bytes())
	bytebufferpool.Put(buf)
	return c, nil
}

type ClientJoinResponse struct {
	Method string      `json:"method"`
	Body   JoinMessage `json:"body"`
}

func NewClientJoinMessage(msg *JoinMessage) *ClientJoinResponse {
	return &ClientJoinResponse{
		Method: "join",
		Body:   *msg,
	}
}

func writeJoin(buf *bytebufferpool.ByteBuffer, msg *JoinMessage) {
	buf.WriteString(`{`)
	buf.WriteString(`"channel":`)
	EncodeJSONString(buf, msg.Channel, true)
	buf.WriteString(`,"data":`)
	writeClientInfo(buf, &msg.Data)
	buf.WriteString(`}`)
}

func (m *ClientJoinResponse) Marshal() ([]byte, error) {
	buf := bytebufferpool.Get()
	buf.WriteString(`{"method":"join","body":`)
	writeJoin(buf, &m.Body)
	buf.WriteString(`}`)
	c := make([]byte, buf.Len())
	copy(c, buf.Bytes())
	bytebufferpool.Put(buf)
	return c, nil
}

type ClientLeaveResponse struct {
	Method string       `json:"method"`
	Body   LeaveMessage `json:"body"`
}

func NewClientLeaveMessage(msg *LeaveMessage) *ClientLeaveResponse {
	return &ClientLeaveResponse{
		Method: "leave",
		Body:   *msg,
	}
}

func writeLeave(buf *bytebufferpool.ByteBuffer, msg *LeaveMessage) {
	buf.WriteString(`{`)
	buf.WriteString(`"channel":`)
	EncodeJSONString(buf, msg.Channel, true)
	buf.WriteString(`,"data":`)
	writeClientInfo(buf, &msg.Data)
	buf.WriteString(`}`)
}

func (m *ClientLeaveResponse) Marshal() ([]byte, error) {
	buf := bytebufferpool.Get()
	buf.WriteString(`{"method":"leave","body":`)
	writeLeave(buf, &m.Body)
	buf.WriteString(`}`)
	c := make([]byte, buf.Len())
	copy(c, buf.Bytes())
	bytebufferpool.Put(buf)
	return c, nil
}

// PresenceBody represents body of response in case of successful presence command.
type PresenceBody struct {
	Channel string                `json:"channel"`
	Data    map[string]ClientInfo `json:"data"`
}

// HistoryBody represents body of response in case of successful history command.
type HistoryBody struct {
	Channel string    `json:"channel"`
	Data    []Message `json:"data"`
}

// ChannelsBody represents body of response in case of successful channels command.
type ChannelsBody struct {
	Data []string `json:"data"`
}

// ConnectBody represents body of response in case of successful connect command.
type ConnectBody struct {
	Version string `json:"version"`
	Client  string `json:"client"`
	Expires bool   `json:"expires"`
	Expired bool   `json:"expired"`
	TTL     int64  `json:"ttl"`
}

// SubscribeBody represents body of response in case of successful subscribe command.
type SubscribeBody struct {
	Channel   string    `json:"channel"`
	Status    bool      `json:"status"`
	Last      string    `json:"last"`
	Messages  []Message `json:"messages"`
	Recovered bool      `json:"recovered"`
}

// UnsubscribeBody represents body of response in case of successful unsubscribe command.
type UnsubscribeBody struct {
	Channel string `json:"channel"`
	Status  bool   `json:"status"`
}

// PublishBody represents body of response in case of successful publish command.
type PublishBody struct {
	Channel string `json:"channel"`
	Status  bool   `json:"status"`
}

// DisconnectBody represents body of disconnect response when we want to tell
// client to disconnect. Optionally we can give client an advice to continue
// reconnecting after receiving this
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
	Data ServerStats `json:"data"`
}

// NodeBody represents body of response in case of successful node command.
type NodeBody struct {
	Data NodeInfo `json:"data"`
}

type AdminInfoBody struct {
	Data map[string]interface{} `json:"data"`
}

type Response interface {
	SetErr(err ResponseError)
	SetUID(uid string)
}

type ErrorAdvice string

const (
	ErrorAdviceNone  ErrorAdvice = ""
	ErrorAdviceFix   ErrorAdvice = "fix"
	ErrorAdviceRetry ErrorAdvice = "retry"
)

type ResponseError struct {
	Err    error       `json:"-"`
	Advice ErrorAdvice `json:"advice,omitempty"`
}

// clientResponse represents an answer Centrifugo sends to client request
// commands or protocol messages sent to client asynchronously.
type clientResponse struct {
	UID    string `json:"uid,omitempty"`
	Method string `json:"method"`
	Error  string `json:"error,omitempty"`
	ResponseError
}

// Err set a client error on the client response and updates the 'err'
// field in the response. If an error has already been set it will be kept.
func (r *clientResponse) SetErr(err ResponseError) {
	if r.ResponseError.Err != nil {
		// error already set.
		return
	}
	r.ResponseError = err
	e := err.Err.Error()
	r.Error = e
}

func (r *clientResponse) SetUID(uid string) {
	r.UID = uid
}

type ClientConnectResponse struct {
	clientResponse
	Body ConnectBody `json:"body"`
}

func NewClientConnectResponse(body ConnectBody) Response {
	return &ClientConnectResponse{
		clientResponse: clientResponse{
			Method: "connect",
		},
		Body: body,
	}
}

type ClientRefreshResponse struct {
	clientResponse
	Body ConnectBody `json:"body"`
}

func NewClientRefreshResponse(body ConnectBody) Response {
	return &ClientRefreshResponse{
		clientResponse: clientResponse{
			Method: "refresh",
		},
		Body: body,
	}
}

type ClientSubscribeResponse struct {
	clientResponse
	Body SubscribeBody `json:"body"`
}

func NewClientSubscribeResponse(body SubscribeBody) Response {
	return &ClientSubscribeResponse{
		clientResponse: clientResponse{
			Method: "subscribe",
		},
		Body: body,
	}
}

type ClientUnsubscribeResponse struct {
	clientResponse
	Body UnsubscribeBody `json:"body"`
}

func NewClientUnsubscribeResponse(body UnsubscribeBody) Response {
	return &ClientUnsubscribeResponse{
		clientResponse: clientResponse{
			Method: "unsubscribe",
		},
		Body: body,
	}
}

type ClientPresenceResponse struct {
	clientResponse
	Body PresenceBody `json:"body"`
}

func NewClientPresenceResponse(body PresenceBody) Response {
	return &ClientPresenceResponse{
		clientResponse: clientResponse{
			Method: "presence",
		},
		Body: body,
	}
}

type ClientHistoryResponse struct {
	clientResponse
	Body HistoryBody `json:"body"`
}

func NewClientHistoryResponse(body HistoryBody) Response {
	return &ClientHistoryResponse{
		clientResponse: clientResponse{
			Method: "history",
		},
		Body: body,
	}
}

type ClientDisconnectResponse struct {
	clientResponse
	Body DisconnectBody `json:"body"`
}

func NewClientDisconnectResponse(body DisconnectBody) Response {
	return &ClientDisconnectResponse{
		clientResponse: clientResponse{
			Method: "disconnect",
		},
		Body: body,
	}
}

type ClientPublishResponse struct {
	clientResponse
	Body PublishBody `json:"body"`
}

func NewClientPublishResponse(body PublishBody) Response {
	return &ClientPublishResponse{
		clientResponse: clientResponse{
			Method: "publish",
		},
		Body: body,
	}
}

type ClientPingResponse struct {
	clientResponse
	Body PingBody `json:"body"`
}

func NewClientPingResponse(body PingBody) Response {
	return &ClientPingResponse{
		clientResponse: clientResponse{
			Method: "ping",
		},
		Body: body,
	}
}

// MultiClientResponse is a slice of responses in execution order - from first
// executed to last one
type MultiClientResponse []Response

// apiResponse represents an answer Centrifugo sends to API request commands
type apiResponse struct {
	UID    string  `json:"uid,omitempty"`
	Method string  `json:"method"`
	Error  *string `json:"error"`
	ResponseError
}

// SetErr set an error message on the api response and updates the 'err' field in
// the response. If an error has already been set it will be kept.
func (r *apiResponse) SetErr(err ResponseError) {
	if r.ResponseError.Err != nil {
		// error already set.
		return
	}
	r.ResponseError = err
	e := err.Err.Error()
	r.Error = &e
	return
}

func (r *apiResponse) SetUID(uid string) {
	r.UID = uid
}

type APIPublishResponse struct {
	apiResponse
	Body interface{} `json:"body"` // TODO: interface{} for API protocol backwards compatibility.
}

func NewAPIPublishResponse() Response {
	return &APIPublishResponse{
		apiResponse: apiResponse{
			Method: "publish",
		},
	}
}

type APIBroadcastResponse struct {
	apiResponse
	Body interface{} `json:"body"` // TODO: interface{} for API protocol backwards compatibility.
}

func NewAPIBroadcastResponse() Response {
	return &APIBroadcastResponse{
		apiResponse: apiResponse{
			Method: "broadcast",
		},
	}
}

type APIPresenceResponse struct {
	apiResponse
	Body PresenceBody `json:"body"`
}

func NewAPIPresenceResponse(body PresenceBody) Response {
	return &APIPresenceResponse{
		apiResponse: apiResponse{
			Method: "presence",
		},
		Body: body,
	}
}

type APIHistoryResponse struct {
	apiResponse
	Body HistoryBody `json:"body"`
}

func NewAPIHistoryResponse(body HistoryBody) Response {
	return &APIHistoryResponse{
		apiResponse: apiResponse{
			Method: "history",
		},
		Body: body,
	}
}

type APIChannelsResponse struct {
	apiResponse
	Body ChannelsBody `json:"body"`
}

func NewAPIChannelsResponse(body ChannelsBody) Response {
	return &APIChannelsResponse{
		apiResponse: apiResponse{
			Method: "channels",
		},
		Body: body,
	}
}

type APIStatsResponse struct {
	apiResponse
	Body StatsBody `json:"body"`
}

func NewAPIStatsResponse(body StatsBody) Response {
	return &APIStatsResponse{
		apiResponse: apiResponse{
			Method: "stats",
		},
		Body: body,
	}
}

type APIUnsubscribeResponse struct {
	apiResponse
	Body interface{} `json:"body"` // TODO: interface{} for API protocol backwards compatibility.
}

func NewAPIUnsubscribeResponse() Response {
	return &APIUnsubscribeResponse{
		apiResponse: apiResponse{
			Method: "unsubscribe",
		},
	}
}

type APIDisconnectResponse struct {
	apiResponse
	Body interface{} `json:"body"` // TODO: interface{} for API protocol backwards compatibility.
}

func NewAPIDisconnectResponse() Response {
	return &APIDisconnectResponse{
		apiResponse: apiResponse{
			Method: "disconnect",
		},
	}
}

type APINodeResponse struct {
	apiResponse
	Body NodeBody `json:"body"`
}

func NewAPINodeResponse(body NodeBody) Response {
	return &APINodeResponse{
		apiResponse: apiResponse{
			Method: "node",
		},
		Body: body,
	}
}

type AdminConnectResponse struct {
	apiResponse
	Body bool `json:"body"`
}

func NewAdminConnectResponse(body bool) Response {
	return &AdminConnectResponse{
		apiResponse: apiResponse{
			Method: "connect",
		},
		Body: body,
	}
}

type AdminInfoResponse struct {
	apiResponse
	Body AdminInfoBody `json:"body"`
}

func NewAdminInfoResponse(body AdminInfoBody) Response {
	return &AdminInfoResponse{
		apiResponse: apiResponse{
			Method: "info",
		},
		Body: body,
	}
}

type AdminPingResponse struct {
	apiResponse
	Body string `json:"body"`
}

func NewAdminPingResponse(body string) Response {
	return &AdminPingResponse{
		apiResponse: apiResponse{
			Method: "ping",
		},
		Body: body,
	}
}

type AdminMessageResponse struct {
	apiResponse
	Body raw.Raw `json:"body"`
}

func NewAdminMessageResponse(body raw.Raw) Response {
	return &AdminMessageResponse{
		apiResponse: apiResponse{
			Method: "message",
		},
		Body: body,
	}
}

// MultiAPIResponse is a slice of API responses returned as a result for
// slice of commands received by API in execution order - from first executed
// to last one.
type MultiAPIResponse []Response
