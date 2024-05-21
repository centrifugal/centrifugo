package proxyproto

import "encoding/json"

type ResponseDecoder interface {
	DecodeConnectResponse(data []byte) (*ConnectResponse, error)
	DecodeRefreshResponse(data []byte) (*RefreshResponse, error)
	DecodeRPCResponse(data []byte) (*RPCResponse, error)
	DecodeSubscribeResponse(data []byte) (*SubscribeResponse, error)
	DecodePublishResponse(data []byte) (*PublishResponse, error)
	DecodeSubRefreshResponse(data []byte) (*SubRefreshResponse, error)
	DecodeNotifyCacheEmptyResponse(data []byte) (*NotifyCacheEmptyResponse, error)
}

var _ ResponseDecoder = (*JSONDecoder)(nil)

type JSONDecoder struct{}

func (e *JSONDecoder) DecodeConnectResponse(data []byte) (*ConnectResponse, error) {
	var resp ConnectResponse
	err := json.Unmarshal(data, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

func (e *JSONDecoder) DecodeRefreshResponse(data []byte) (*RefreshResponse, error) {
	var resp RefreshResponse
	err := json.Unmarshal(data, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

func (e *JSONDecoder) DecodeRPCResponse(data []byte) (*RPCResponse, error) {
	var resp RPCResponse
	err := json.Unmarshal(data, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

func (e *JSONDecoder) DecodeSubscribeResponse(data []byte) (*SubscribeResponse, error) {
	var resp SubscribeResponse
	err := json.Unmarshal(data, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

func (e *JSONDecoder) DecodePublishResponse(data []byte) (*PublishResponse, error) {
	var resp PublishResponse
	err := json.Unmarshal(data, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

func (e *JSONDecoder) DecodeSubRefreshResponse(data []byte) (*SubRefreshResponse, error) {
	var resp SubRefreshResponse
	err := json.Unmarshal(data, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

func (e *JSONDecoder) DecodeNotifyCacheEmptyResponse(data []byte) (*NotifyCacheEmptyResponse, error) {
	var resp NotifyCacheEmptyResponse
	err := json.Unmarshal(data, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}
