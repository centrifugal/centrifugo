package apiproto

// RequestDecoder ...
type RequestDecoder interface {
	DecodeBatch([]byte) (*BatchRequest, error)
	DecodePublish([]byte) (*PublishRequest, error)
	DecodeBroadcast([]byte) (*BroadcastRequest, error)
	DecodeSubscribe([]byte) (*SubscribeRequest, error)
	DecodeUnsubscribe([]byte) (*UnsubscribeRequest, error)
	DecodeDisconnect([]byte) (*DisconnectRequest, error)
	DecodePresence([]byte) (*PresenceRequest, error)
	DecodePresenceStats([]byte) (*PresenceStatsRequest, error)
	DecodeHistory([]byte) (*HistoryRequest, error)
	DecodeHistoryRemove([]byte) (*HistoryRemoveRequest, error)
	DecodeInfo([]byte) (*InfoRequest, error)
	DecodeRPC([]byte) (*RPCRequest, error)
	DecodeRefresh([]byte) (*RefreshRequest, error)
	DecodeChannels([]byte) (*ChannelsRequest, error)
}
