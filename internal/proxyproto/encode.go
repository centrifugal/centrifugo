package proxyproto

import "encoding/json"

type RequestEncoder interface {
	EncodeConnectRequest(req *ConnectRequest) ([]byte, error)
	EncodeRefreshRequest(req *RefreshRequest) ([]byte, error)
	EncodeRPCRequest(req *RPCRequest) ([]byte, error)
	EncodeSubscribeRequest(req *SubscribeRequest) ([]byte, error)
	EncodePublishRequest(req *PublishRequest) ([]byte, error)
	EncodeSubRefreshRequest(req *SubRefreshRequest) ([]byte, error)
	EncodeNotifyCacheEmptyRequest(req *NotifyCacheEmptyRequest) ([]byte, error)
	EncodeMapPublishRequest(req *MapPublishRequest) ([]byte, error)
	EncodeMapRemoveRequest(req *MapRemoveRequest) ([]byte, error)
	EncodeSharedPollRefreshRequest(req *SharedPollRefreshRequest) ([]byte, error)
}

var _ RequestEncoder = (*JSONEncoder)(nil)

type JSONEncoder struct{}

func (e *JSONEncoder) EncodeConnectRequest(req *ConnectRequest) ([]byte, error) {
	return json.Marshal(req)
}

func (e *JSONEncoder) EncodeRefreshRequest(req *RefreshRequest) ([]byte, error) {
	return json.Marshal(req)
}

func (e *JSONEncoder) EncodeRPCRequest(req *RPCRequest) ([]byte, error) {
	return json.Marshal(req)
}

func (e *JSONEncoder) EncodeSubscribeRequest(req *SubscribeRequest) ([]byte, error) {
	return json.Marshal(req)
}

func (e *JSONEncoder) EncodePublishRequest(req *PublishRequest) ([]byte, error) {
	return json.Marshal(req)
}

func (e *JSONEncoder) EncodeSubRefreshRequest(req *SubRefreshRequest) ([]byte, error) {
	return json.Marshal(req)
}

func (e *JSONEncoder) EncodeNotifyCacheEmptyRequest(req *NotifyCacheEmptyRequest) ([]byte, error) {
	return json.Marshal(req)
}

func (e *JSONEncoder) EncodeMapPublishRequest(req *MapPublishRequest) ([]byte, error) {
	return json.Marshal(req)
}

func (e *JSONEncoder) EncodeMapRemoveRequest(req *MapRemoveRequest) ([]byte, error) {
	return json.Marshal(req)
}

func (e *JSONEncoder) EncodeSharedPollRefreshRequest(req *SharedPollRefreshRequest) ([]byte, error) {
	return json.Marshal(req)
}
