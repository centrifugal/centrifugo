package response

import (
	"github.com/centrifugal/centrifugo/libcentrifugo/config"
	"github.com/centrifugal/centrifugo/libcentrifugo/encode"
	"github.com/centrifugal/centrifugo/libcentrifugo/message"
	"github.com/centrifugal/centrifugo/libcentrifugo/metrics"
	"github.com/centrifugal/centrifugo/libcentrifugo/raw"
	"github.com/valyala/bytebufferpool"
)

// ClientMessageResponse can not have an error.
type ClientMessageResponse struct {
	Method string          `json:"method"`
	Body   message.Message `json:"body"`
}

// NewClientMessage returns initialized client message response.
func NewClientMessage() *ClientMessageResponse {
	return &ClientMessageResponse{
		Method: "message",
	}
}

func writeClientInfo(buf *bytebufferpool.ByteBuffer, info *message.ClientInfo) {
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

func writeMessage(buf *bytebufferpool.ByteBuffer, msg *message.Message) {
	buf.WriteString(`{"uid":"`)
	buf.WriteString(msg.UID)
	buf.WriteString(`","timestamp":"`)
	buf.WriteString(msg.Timestamp)
	buf.WriteString(`",`)

	if msg.Client != "" {
		buf.WriteString(`"client":`)
		encode.EncodeJSONString(buf, msg.Client, true)
		buf.WriteString(`,`)
	}

	if msg.Info != nil {
		buf.WriteString(`"info":`)
		writeClientInfo(buf, msg.Info)
		buf.WriteString(`,`)
	}

	buf.WriteString(`"channel":`)
	encode.EncodeJSONString(buf, msg.Channel, true)
	buf.WriteString(`,"data":`)
	buf.Write(*msg.Data)
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
	Method string              `json:"method"`
	Body   message.JoinMessage `json:"body"`
}

func NewClientJoinMessage() *ClientJoinResponse {
	return &ClientJoinResponse{
		Method: "join",
	}
}

func writeJoin(buf *bytebufferpool.ByteBuffer, msg *message.JoinMessage) {
	buf.WriteString(`{`)
	buf.WriteString(`"channel":`)
	encode.EncodeJSONString(buf, msg.Channel, true)
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
	Method string               `json:"method"`
	Body   message.LeaveMessage `json:"body"`
}

func NewClientLeaveMessage() *ClientLeaveResponse {
	return &ClientLeaveResponse{
		Method: "leave",
	}
}

func writeLeave(buf *bytebufferpool.ByteBuffer, msg *message.LeaveMessage) {
	buf.WriteString(`{`)
	buf.WriteString(`"channel":`)
	encode.EncodeJSONString(buf, msg.Channel, true)
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
	Channel message.Channel                       `json:"channel"`
	Data    map[message.ConnID]message.ClientInfo `json:"data"`
}

// HistoryBody represents body of response in case of successful history command.
type HistoryBody struct {
	Channel message.Channel   `json:"channel"`
	Data    []message.Message `json:"data"`
}

// ChannelsBody represents body of response in case of successful channels command.
type ChannelsBody struct {
	Data []message.Channel `json:"data"`
}

// ConnectBody represents body of response in case of successful connect command.
type ConnectBody struct {
	Version string         `json:"version"`
	Client  message.ConnID `json:"client"`
	Expires bool           `json:"expires"`
	Expired bool           `json:"expired"`
	TTL     int64          `json:"ttl"`
}

// SubscribeBody represents body of response in case of successful subscribe command.
type SubscribeBody struct {
	Channel   message.Channel   `json:"channel"`
	Status    bool              `json:"status"`
	Last      message.MessageID `json:"last"`
	Messages  []message.Message `json:"messages"`
	Recovered bool              `json:"recovered"`
}

// UnsubscribeBody represents body of response in case of successful unsubscribe command.
type UnsubscribeBody struct {
	Channel message.Channel `json:"channel"`
	Status  bool            `json:"status"`
}

// PublishBody represents body of response in case of successful publish command.
type PublishBody struct {
	Channel message.Channel `json:"channel"`
	Status  bool            `json:"status"`
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
	Data metrics.ServerStats `json:"data"`
}

// NodeBody represents body of response in case of successful node command.
type NodeBody struct {
	Data metrics.NodeInfo `json:"data"`
}

type adminMessageBody struct {
	Message message.Message `json:"message"`
}

type AdminInfoBody struct {
	Engine string         `json:"engine"`
	Config *config.Config `json:"config"`
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
	Err    error
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

type clientConnectResponse struct {
	clientResponse
	Body ConnectBody `json:"body"`
}

func NewClientConnectResponse(body ConnectBody) Response {
	return &clientConnectResponse{
		clientResponse: clientResponse{
			Method: "connect",
		},
		Body: body,
	}
}

type clientRefreshResponse struct {
	clientResponse
	Body ConnectBody `json:"body"`
}

func NewClientRefreshResponse(body ConnectBody) Response {
	return &clientRefreshResponse{
		clientResponse: clientResponse{
			Method: "refresh",
		},
		Body: body,
	}
}

type clientSubscribeResponse struct {
	clientResponse
	Body SubscribeBody `json:"body"`
}

func NewClientSubscribeResponse(body SubscribeBody) Response {
	return &clientSubscribeResponse{
		clientResponse: clientResponse{
			Method: "subscribe",
		},
		Body: body,
	}
}

type clientUnsubscribeResponse struct {
	clientResponse
	Body UnsubscribeBody `json:"body"`
}

func NewClientUnsubscribeResponse(body UnsubscribeBody) Response {
	return &clientUnsubscribeResponse{
		clientResponse: clientResponse{
			Method: "unsubscribe",
		},
		Body: body,
	}
}

type clientPresenceResponse struct {
	clientResponse
	Body PresenceBody `json:"body"`
}

func NewClientPresenceResponse(body PresenceBody) Response {
	return &clientPresenceResponse{
		clientResponse: clientResponse{
			Method: "presence",
		},
		Body: body,
	}
}

type clientHistoryResponse struct {
	clientResponse
	Body HistoryBody `json:"body"`
}

func NewClientHistoryResponse(body HistoryBody) Response {
	return &clientHistoryResponse{
		clientResponse: clientResponse{
			Method: "history",
		},
		Body: body,
	}
}

type clientDisconnectResponse struct {
	clientResponse
	Body DisconnectBody `json:"body"`
}

func NewClientDisconnectResponse(body DisconnectBody) Response {
	return &clientDisconnectResponse{
		clientResponse: clientResponse{
			Method: "disconnect",
		},
		Body: body,
	}
}

type clientPublishResponse struct {
	clientResponse
	Body PublishBody `json:"body"`
}

func NewClientPublishResponse(body PublishBody) Response {
	return &clientPublishResponse{
		clientResponse: clientResponse{
			Method: "publish",
		},
		Body: body,
	}
}

type clientPingResponse struct {
	clientResponse
	Body PingBody `json:"body"`
}

func NewClientPingResponse(body PingBody) Response {
	return &clientPingResponse{
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

type apiPublishResponse struct {
	apiResponse
	Body interface{} `json:"body"` // TODO: interface{} for API protocol backwards compatibility.
}

func NewAPIPublishResponse() Response {
	return &apiPublishResponse{
		apiResponse: apiResponse{
			Method: "publish",
		},
	}
}

type apiBroadcastResponse struct {
	apiResponse
	Body interface{} `json:"body"` // TODO: interface{} for API protocol backwards compatibility.
}

func NewAPIBroadcastResponse() Response {
	return &apiBroadcastResponse{
		apiResponse: apiResponse{
			Method: "broadcast",
		},
	}
}

type apiPresenceResponse struct {
	apiResponse
	Body PresenceBody `json:"body"`
}

func NewAPIPresenceResponse(body PresenceBody) Response {
	return &apiPresenceResponse{
		apiResponse: apiResponse{
			Method: "presence",
		},
		Body: body,
	}
}

type apiHistoryResponse struct {
	apiResponse
	Body HistoryBody `json:"body"`
}

func NewAPIHistoryResponse(body HistoryBody) Response {
	return &apiHistoryResponse{
		apiResponse: apiResponse{
			Method: "history",
		},
		Body: body,
	}
}

type apiChannelsResponse struct {
	apiResponse
	Body ChannelsBody `json:"body"`
}

func NewAPIChannelsResponse(body ChannelsBody) Response {
	return &apiChannelsResponse{
		apiResponse: apiResponse{
			Method: "channels",
		},
		Body: body,
	}
}

type apiStatsResponse struct {
	apiResponse
	Body StatsBody `json:"body"`
}

func NewAPIStatsResponse(body StatsBody) Response {
	return &apiStatsResponse{
		apiResponse: apiResponse{
			Method: "stats",
		},
		Body: body,
	}
}

type apiUnsubscribeResponse struct {
	apiResponse
	Body interface{} `json:"body"` // TODO: interface{} for API protocol backwards compatibility.
}

func NewAPIUnsubscribeResponse() Response {
	return &apiUnsubscribeResponse{
		apiResponse: apiResponse{
			Method: "unsubscribe",
		},
	}
}

type apiDisconnectResponse struct {
	apiResponse
	Body interface{} `json:"body"` // TODO: interface{} for API protocol backwards compatibility.
}

func NewAPIDisconnectResponse() Response {
	return &apiDisconnectResponse{
		apiResponse: apiResponse{
			Method: "disconnect",
		},
	}
}

type apiNodeResponse struct {
	apiResponse
	Body NodeBody `json:"body"`
}

func NewAPINodeResponse(body NodeBody) Response {
	return &apiNodeResponse{
		apiResponse: apiResponse{
			Method: "node",
		},
		Body: body,
	}
}

type apiAdminConnectResponse struct {
	apiResponse
	Body bool `json:"body"`
}

func NewAPIAdminConnectResponse(body bool) Response {
	return &apiAdminConnectResponse{
		apiResponse: apiResponse{
			Method: "connect",
		},
		Body: body,
	}
}

type apiAdminInfoResponse struct {
	apiResponse
	Body AdminInfoBody `json:"body"`
}

func NewAPIAdminInfoResponse(body AdminInfoBody) Response {
	return &apiAdminInfoResponse{
		apiResponse: apiResponse{
			Method: "info",
		},
		Body: body,
	}
}

type apiAdminPingResponse struct {
	apiResponse
	Body string `json:"body"`
}

func NewAPIAdminPingResponse(body string) Response {
	return &apiAdminPingResponse{
		apiResponse: apiResponse{
			Method: "ping",
		},
		Body: body,
	}
}

type apiAdminMessageResponse struct {
	apiResponse
	Body *raw.Raw `json:"body"`
}

func NewAPIAdminMessageResponse(body *raw.Raw) Response {
	return &apiAdminMessageResponse{
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
