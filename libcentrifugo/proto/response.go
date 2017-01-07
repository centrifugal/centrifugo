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

// Marshal marshals ClientMessageResponse into JSON using buffer pool and manual
// JSON construction.
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

// ClientJoinResponse sent to client when someone subscribed on channel.
type ClientJoinResponse struct {
	Method string      `json:"method"`
	Body   JoinMessage `json:"body"`
}

// NewClientJoinMessage initializes ClientJoinResponse.
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

// Marshal marshals ClientJoinResponse into JSON using buffer pool and manual
// JSON construction.
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

// ClientLeaveResponse sent when someone unsubscribes from channel.
type ClientLeaveResponse struct {
	Method string       `json:"method"`
	Body   LeaveMessage `json:"body"`
}

// NewClientLeaveMessage initializes ClientLeaveResponse.
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

// Marshal marshals ClientLeaveResponse into JSON using buffer pool and manual
// JSON construction.
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

// PingBody represents body of response in case of successful ping command.
type PingBody struct {
	Data string `json:"data,omitempty"`
}

// StatsBody represents body of response in case of successful stats command.
type StatsBody struct {
	Data ServerStats `json:"data"`
}

// NodeBody represents body of response in case of successful node command.
type NodeBody struct {
	Data NodeInfo `json:"data"`
}

// AdminInfoBody represents response to admin info command.
type AdminInfoBody struct {
	Data map[string]interface{} `json:"data"`
}

// Response is an interface describing response methods.
type Response interface {
	SetErr(err ResponseError)
	SetUID(uid string)
}

// ErrorAdvice is a string which is sent in case of error. It contains advice how client
// should behave when error occurred.
type ErrorAdvice string

const (
	// ErrorAdviceNone represents undefined advice.
	ErrorAdviceNone ErrorAdvice = ""
	// ErrorAdviceFix says that developer most probably made a mistake working with Centrifugo.
	ErrorAdviceFix ErrorAdvice = "fix"
	// ErrorAdviceRetry says that operation can't be done at moment but may be possible in future.
	ErrorAdviceRetry ErrorAdvice = "retry"
)

// ResponseError represents error in response.
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

// SetErr sets a client error on the client response and updates the 'err'
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

// SetUID sets unique uid which must be equal to request uid.
func (r *clientResponse) SetUID(uid string) {
	r.UID = uid
}

// ClientConnectResponse represents response to client connect command.
type ClientConnectResponse struct {
	clientResponse
	Body ConnectBody `json:"body"`
}

// NewClientConnectResponse initializes ClientConnectResponse.
func NewClientConnectResponse(body ConnectBody) *ClientConnectResponse {
	return &ClientConnectResponse{
		clientResponse: clientResponse{
			Method: "connect",
		},
		Body: body,
	}
}

// ClientRefreshResponse represents response to client refresh command.
type ClientRefreshResponse struct {
	clientResponse
	Body ConnectBody `json:"body"`
}

// NewClientRefreshResponse initializes ClientRefreshResponse.
func NewClientRefreshResponse(body ConnectBody) *ClientRefreshResponse {
	return &ClientRefreshResponse{
		clientResponse: clientResponse{
			Method: "refresh",
		},
		Body: body,
	}
}

// ClientSubscribeResponse represents response to client subscribe command.
type ClientSubscribeResponse struct {
	clientResponse
	Body SubscribeBody `json:"body"`
}

// NewClientSubscribeResponse initializes ClientSubscribeResponse.
func NewClientSubscribeResponse(body SubscribeBody) *ClientSubscribeResponse {
	return &ClientSubscribeResponse{
		clientResponse: clientResponse{
			Method: "subscribe",
		},
		Body: body,
	}
}

// ClientUnsubscribeResponse represents response to client unsubscribe command.
type ClientUnsubscribeResponse struct {
	clientResponse
	Body UnsubscribeBody `json:"body"`
}

// NewClientUnsubscribeResponse initializes ClientUnsubscribeResponse.
func NewClientUnsubscribeResponse(body UnsubscribeBody) *ClientUnsubscribeResponse {
	return &ClientUnsubscribeResponse{
		clientResponse: clientResponse{
			Method: "unsubscribe",
		},
		Body: body,
	}
}

// ClientPresenceResponse represents response to client presence command.
type ClientPresenceResponse struct {
	clientResponse
	Body PresenceBody `json:"body"`
}

// NewClientPresenceResponse initializes ClientPresenceResponse.
func NewClientPresenceResponse(body PresenceBody) *ClientPresenceResponse {
	return &ClientPresenceResponse{
		clientResponse: clientResponse{
			Method: "presence",
		},
		Body: body,
	}
}

// ClientHistoryResponse represents response to client history command.
type ClientHistoryResponse struct {
	clientResponse
	Body HistoryBody `json:"body"`
}

// NewClientHistoryResponse initializes ClientHistoryResponse.
func NewClientHistoryResponse(body HistoryBody) *ClientHistoryResponse {
	return &ClientHistoryResponse{
		clientResponse: clientResponse{
			Method: "history",
		},
		Body: body,
	}
}

// ClientPublishResponse represents response to ClientPublishResponse.
type ClientPublishResponse struct {
	clientResponse
	Body PublishBody `json:"body"`
}

// NewClientPublishResponse initializes ClientPublishResponse.
func NewClientPublishResponse(body PublishBody) *ClientPublishResponse {
	return &ClientPublishResponse{
		clientResponse: clientResponse{
			Method: "publish",
		},
		Body: body,
	}
}

// ClientPingResponse represents response to client ping command.
type ClientPingResponse struct {
	clientResponse
	Body *PingBody `json:"body,omitempty"`
}

// NewClientPingResponse initializes ClientPingResponse.
func NewClientPingResponse(body *PingBody) *ClientPingResponse {
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

// SetErr sets an error message on the api response and updates the 'err' field in
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

// SetUID allows to set uid to api response.
func (r *apiResponse) SetUID(uid string) {
	r.UID = uid
}

// APIPublishResponse represents response to API publish command.
type APIPublishResponse struct {
	apiResponse
	Body interface{} `json:"body"` // TODO: interface{} for API protocol backwards compatibility.
}

// NewAPIPublishResponse initializes APIPublishResponse.
func NewAPIPublishResponse() *APIPublishResponse {
	return &APIPublishResponse{
		apiResponse: apiResponse{
			Method: "publish",
		},
	}
}

// APIBroadcastResponse represents response to broadcast API command.
type APIBroadcastResponse struct {
	apiResponse
	Body interface{} `json:"body"` // TODO: interface{} for API protocol backwards compatibility.
}

// NewAPIBroadcastResponse initializes APIBroadcastResponse.
func NewAPIBroadcastResponse() *APIBroadcastResponse {
	return &APIBroadcastResponse{
		apiResponse: apiResponse{
			Method: "broadcast",
		},
	}
}

// APIPresenceResponse represents response to API presence command.
type APIPresenceResponse struct {
	apiResponse
	Body PresenceBody `json:"body"`
}

// NewAPIPresenceResponse initializes APIPresenceResponse.
func NewAPIPresenceResponse(body PresenceBody) *APIPresenceResponse {
	return &APIPresenceResponse{
		apiResponse: apiResponse{
			Method: "presence",
		},
		Body: body,
	}
}

// APIHistoryResponse represents response to API history command.
type APIHistoryResponse struct {
	apiResponse
	Body HistoryBody `json:"body"`
}

// NewAPIHistoryResponse initializes APIHistoryResponse.
func NewAPIHistoryResponse(body HistoryBody) *APIHistoryResponse {
	return &APIHistoryResponse{
		apiResponse: apiResponse{
			Method: "history",
		},
		Body: body,
	}
}

// APIChannelsResponse represents response to API channels command.
type APIChannelsResponse struct {
	apiResponse
	Body ChannelsBody `json:"body"`
}

// NewAPIChannelsResponse initializes APIChannelsResponse.
func NewAPIChannelsResponse(body ChannelsBody) *APIChannelsResponse {
	return &APIChannelsResponse{
		apiResponse: apiResponse{
			Method: "channels",
		},
		Body: body,
	}
}

// APIStatsResponse represents response to API stats command.
type APIStatsResponse struct {
	apiResponse
	Body StatsBody `json:"body"`
}

// NewAPIStatsResponse initializes APIStatsResponse.
func NewAPIStatsResponse(body StatsBody) *APIStatsResponse {
	return &APIStatsResponse{
		apiResponse: apiResponse{
			Method: "stats",
		},
		Body: body,
	}
}

// APIUnsubscribeResponse represents response to API unsubscribe command.
type APIUnsubscribeResponse struct {
	apiResponse
	Body interface{} `json:"body"` // TODO: interface{} for API protocol backwards compatibility.
}

// NewAPIUnsubscribeResponse initializes APIUnsubscribeResponse.
func NewAPIUnsubscribeResponse() *APIUnsubscribeResponse {
	return &APIUnsubscribeResponse{
		apiResponse: apiResponse{
			Method: "unsubscribe",
		},
	}
}

// APIDisconnectResponse represents response to API disconnect command.
type APIDisconnectResponse struct {
	apiResponse
	Body interface{} `json:"body"` // TODO: interface{} for API protocol backwards compatibility.
}

// NewAPIDisconnectResponse initializes APIDisconnectResponse.
func NewAPIDisconnectResponse() *APIDisconnectResponse {
	return &APIDisconnectResponse{
		apiResponse: apiResponse{
			Method: "disconnect",
		},
	}
}

// APINodeResponse represents response to API node command.
type APINodeResponse struct {
	apiResponse
	Body NodeBody `json:"body"`
}

// NewAPINodeResponse initializes APINodeResponse.
func NewAPINodeResponse(body NodeBody) *APINodeResponse {
	return &APINodeResponse{
		apiResponse: apiResponse{
			Method: "node",
		},
		Body: body,
	}
}

// AdminConnectResponse represents response to admin connect command.
type AdminConnectResponse struct {
	apiResponse
	Body bool `json:"body"`
}

// NewAdminConnectResponse initializes AdminConnectResponse.
func NewAdminConnectResponse(body bool) *AdminConnectResponse {
	return &AdminConnectResponse{
		apiResponse: apiResponse{
			Method: "connect",
		},
		Body: body,
	}
}

// AdminInfoResponse represents response to admin info command.
type AdminInfoResponse struct {
	apiResponse
	Body AdminInfoBody `json:"body"`
}

// NewAdminInfoResponse initializes AdminInfoResponse.
func NewAdminInfoResponse(body AdminInfoBody) *AdminInfoResponse {
	return &AdminInfoResponse{
		apiResponse: apiResponse{
			Method: "info",
		},
		Body: body,
	}
}

// AdminPingResponse represents response to admin ping command.
type AdminPingResponse struct {
	apiResponse
	Body string `json:"body"`
}

// NewAdminPingResponse initializes AdminPingResponse.
func NewAdminPingResponse(body string) *AdminPingResponse {
	return &AdminPingResponse{
		apiResponse: apiResponse{
			Method: "ping",
		},
		Body: body,
	}
}

// AdminMessageResponse is a new message for admin that watches.
type AdminMessageResponse struct {
	apiResponse
	Body raw.Raw `json:"body"`
}

// NewAdminMessageResponse initializes AdminMessageResponse.
func NewAdminMessageResponse(body raw.Raw) *AdminMessageResponse {
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
