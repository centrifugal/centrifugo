package libcentrifugo

import (
	"bytes"
	"runtime"

	"github.com/centrifugal/centrifugo/libcentrifugo/encode"
	"github.com/centrifugal/centrifugo/libcentrifugo/raw"
	"github.com/oxtoacart/bpool"
)

// clientMessageResponse can not have an error.
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

type adminInfoBody struct {
	Engine string  `json:"engine"`
	Config *Config `json:"config"`
}

type response interface {
	SetErr(err responseError)
	SetUID(uid string)
}

type errorAdvice string

const (
	errorAdviceNone  errorAdvice = ""
	errorAdviceFix   errorAdvice = "fix"
	errorAdviceRetry errorAdvice = "retry"
)

type responseError struct {
	err    error
	Advice errorAdvice `json:"advice,omitempty"`
}

// clientResponse represents an answer Centrifugo sends to client request
// commands or protocol messages sent to client asynchronously.
type clientResponse struct {
	UID    string `json:"uid,omitempty"`
	Method string `json:"method"`
	Error  string `json:"error,omitempty"`
	responseError
}

// Err set a client error on the client response and updates the 'err'
// field in the response. If an error has already been set it will be kept.
func (r *clientResponse) SetErr(err responseError) {
	if r.responseError.err != nil {
		// error already set.
		return
	}
	r.responseError = err
	e := err.err.Error()
	r.Error = e
}

func (r *clientResponse) SetUID(uid string) {
	r.UID = uid
}

type clientConnectResponse struct {
	clientResponse
	Body connectBody `json:"body"`
}

func newClientConnectResponse(body connectBody) response {
	return &clientConnectResponse{
		clientResponse: clientResponse{
			Method: "connect",
		},
		Body: body,
	}
}

type clientRefreshResponse struct {
	clientResponse
	Body connectBody `json:"body"`
}

func newClientRefreshResponse(body connectBody) response {
	return &clientRefreshResponse{
		clientResponse: clientResponse{
			Method: "refresh",
		},
		Body: body,
	}
}

type clientSubscribeResponse struct {
	clientResponse
	Body subscribeBody `json:"body"`
}

func newClientSubscribeResponse(body subscribeBody) response {
	return &clientSubscribeResponse{
		clientResponse: clientResponse{
			Method: "subscribe",
		},
		Body: body,
	}
}

type clientUnsubscribeResponse struct {
	clientResponse
	Body unsubscribeBody `json:"body"`
}

func newClientUnsubscribeResponse(body unsubscribeBody) response {
	return &clientUnsubscribeResponse{
		clientResponse: clientResponse{
			Method: "unsubscribe",
		},
		Body: body,
	}
}

type clientPresenceResponse struct {
	clientResponse
	Body presenceBody `json:"body"`
}

func newClientPresenceResponse(body presenceBody) response {
	return &clientPresenceResponse{
		clientResponse: clientResponse{
			Method: "presence",
		},
		Body: body,
	}
}

type clientHistoryResponse struct {
	clientResponse
	Body historyBody `json:"body"`
}

func newClientHistoryResponse(body historyBody) response {
	return &clientHistoryResponse{
		clientResponse: clientResponse{
			Method: "history",
		},
		Body: body,
	}
}

type clientDisconnectResponse struct {
	clientResponse
	Body disconnectBody `json:"body"`
}

func newClientDisconnectResponse(body disconnectBody) response {
	return &clientDisconnectResponse{
		clientResponse: clientResponse{
			Method: "disconnect",
		},
		Body: body,
	}
}

type clientPublishResponse struct {
	clientResponse
	Body publishBody `json:"body"`
}

func newClientPublishResponse(body publishBody) response {
	return &clientPublishResponse{
		clientResponse: clientResponse{
			Method: "publish",
		},
		Body: body,
	}
}

type clientPingResponse struct {
	clientResponse
	Body pingBody `json:"body"`
}

func newClientPingResponse(body pingBody) response {
	return &clientPingResponse{
		clientResponse: clientResponse{
			Method: "ping",
		},
		Body: body,
	}
}

// multiClientResponse is a slice of responses in execution order - from first
// executed to last one
type multiClientResponse []response

// apiResponse represents an answer Centrifugo sends to API request commands
type apiResponse struct {
	UID    string  `json:"uid,omitempty"`
	Method string  `json:"method"`
	Error  *string `json:"error"`
	responseError
}

// SetErr set an error message on the api response and updates the 'err' field in
// the response. If an error has already been set it will be kept.
func (r *apiResponse) SetErr(err responseError) {
	if r.responseError.err != nil {
		// error already set.
		return
	}
	r.responseError = err
	e := err.err.Error()
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

func newAPIPublishResponse() response {
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

func newAPIBroadcastResponse() response {
	return &apiBroadcastResponse{
		apiResponse: apiResponse{
			Method: "broadcast",
		},
	}
}

type apiPresenceResponse struct {
	apiResponse
	Body presenceBody `json:"body"`
}

func newAPIPresenceResponse(body presenceBody) response {
	return &apiPresenceResponse{
		apiResponse: apiResponse{
			Method: "presence",
		},
		Body: body,
	}
}

type apiHistoryResponse struct {
	apiResponse
	Body historyBody `json:"body"`
}

func newAPIHistoryResponse(body historyBody) response {
	return &apiHistoryResponse{
		apiResponse: apiResponse{
			Method: "history",
		},
		Body: body,
	}
}

type apiChannelsResponse struct {
	apiResponse
	Body channelsBody `json:"body"`
}

func newAPIChannelsResponse(body channelsBody) response {
	return &apiChannelsResponse{
		apiResponse: apiResponse{
			Method: "channels",
		},
		Body: body,
	}
}

type apiStatsResponse struct {
	apiResponse
	Body statsBody `json:"body"`
}

func newAPIStatsResponse(body statsBody) response {
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

func newAPIUnsubscribeResponse() response {
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

func newAPIDisconnectResponse() response {
	return &apiDisconnectResponse{
		apiResponse: apiResponse{
			Method: "disconnect",
		},
	}
}

type apiNodeResponse struct {
	apiResponse
	Body nodeBody `json:"body"`
}

func newAPINodeResponse(body nodeBody) response {
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

func newAPIAdminConnectResponse(body bool) response {
	return &apiAdminConnectResponse{
		apiResponse: apiResponse{
			Method: "connect",
		},
		Body: body,
	}
}

type apiAdminInfoResponse struct {
	apiResponse
	Body adminInfoBody `json:"body"`
}

func newAPIAdminInfoResponse(body adminInfoBody) response {
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

func newAPIAdminPingResponse(body string) response {
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

func newAPIAdminMessageResponse(body *raw.Raw) response {
	return &apiAdminMessageResponse{
		apiResponse: apiResponse{
			Method: "message",
		},
		Body: body,
	}
}

// multiAPIResponse is a slice of API responses returned as a result for
// slice of commands received by API in execution order - from first executed
// to last one.
type multiAPIResponse []response
