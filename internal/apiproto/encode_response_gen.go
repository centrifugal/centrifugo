package apiproto

import "encoding/json"

// JSONResponseEncoder ...
type JSONResponseEncoder struct{}

func NewJSONResponseEncoder() *JSONResponseEncoder {
	return &JSONResponseEncoder{}
}

func (e *JSONResponseEncoder) EncodePublish(response *PublishResponse) ([]byte, error) {
	//nolint:staticcheck
	return json.Marshal(response)
}

func (e *JSONResponseEncoder) EncodeBroadcast(response *BroadcastResponse) ([]byte, error) {
	//nolint:staticcheck
	return json.Marshal(response)
}

func (e *JSONResponseEncoder) EncodeSubscribe(response *SubscribeResponse) ([]byte, error) {
	//nolint:staticcheck
	return json.Marshal(response)
}

func (e *JSONResponseEncoder) EncodeUnsubscribe(response *UnsubscribeResponse) ([]byte, error) {
	//nolint:staticcheck
	return json.Marshal(response)
}

func (e *JSONResponseEncoder) EncodeDisconnect(response *DisconnectResponse) ([]byte, error) {
	//nolint:staticcheck
	return json.Marshal(response)
}

func (e *JSONResponseEncoder) EncodePresence(response *PresenceResponse) ([]byte, error) {
	//nolint:staticcheck
	return json.Marshal(response)
}

func (e *JSONResponseEncoder) EncodePresenceStats(response *PresenceStatsResponse) ([]byte, error) {
	//nolint:staticcheck
	return json.Marshal(response)
}

func (e *JSONResponseEncoder) EncodeHistory(response *HistoryResponse) ([]byte, error) {
	//nolint:staticcheck
	return json.Marshal(response)
}

func (e *JSONResponseEncoder) EncodeHistoryRemove(response *HistoryRemoveResponse) ([]byte, error) {
	//nolint:staticcheck
	return json.Marshal(response)
}

func (e *JSONResponseEncoder) EncodeInfo(response *InfoResponse) ([]byte, error) {
	//nolint:staticcheck
	return json.Marshal(response)
}

func (e *JSONResponseEncoder) EncodeRPC(response *RPCResponse) ([]byte, error) {
	//nolint:staticcheck
	return json.Marshal(response)
}

func (e *JSONResponseEncoder) EncodeRefresh(response *RefreshResponse) ([]byte, error) {
	//nolint:staticcheck
	return json.Marshal(response)
}

func (e *JSONResponseEncoder) EncodeChannels(response *ChannelsResponse) ([]byte, error) {
	//nolint:staticcheck
	return json.Marshal(response)
}

func (e *JSONResponseEncoder) EncodeBatch(response *BatchResponse) ([]byte, error) {
	//nolint:staticcheck
	return json.Marshal(response)
}
