package apiproto

// ResultEncoder ...
type ResultEncoder interface {
	EncodePublish(*PublishResult) ([]byte, error)
	EncodeBroadcast(*BroadcastResult) ([]byte, error)
	EncodeSubscribe(*SubscribeResult) ([]byte, error)
	EncodeUnsubscribe(*UnsubscribeResult) ([]byte, error)
	EncodeDisconnect(*DisconnectResult) ([]byte, error)
	EncodePresence(*PresenceResult) ([]byte, error)
	EncodePresenceStats(*PresenceStatsResult) ([]byte, error)
	EncodeHistory(*HistoryResult) ([]byte, error)
	EncodeHistoryRemove(*HistoryRemoveResult) ([]byte, error)
	EncodeInfo(*InfoResult) ([]byte, error)
	EncodeRPC(*RPCResult) ([]byte, error)
	EncodeRefresh(*RefreshResult) ([]byte, error)
	EncodeChannels(*ChannelsResult) ([]byte, error)
}
