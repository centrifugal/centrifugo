package libcentrifugo

type PresenceBody struct {
	Channel Channel               `json:"channel"`
	Data    map[ConnID]ClientInfo `json:"data"`
}

type HistoryBody struct {
	Channel Channel   `json:"channel"`
	Data    []Message `json:"data"`
}

type ChannelsBody struct {
	Data []Channel `json:"data"`
}

type JoinLeaveBody struct {
	Channel Channel    `json:"channel"`
	Data    ClientInfo `json:"data"`
}

type ConnectBody struct {
	Version string `json:"version"`
	Client  ConnID `json:"client"`
	Expired bool   `json:"expired"`
	TTL     *int64 `json:"ttl"`
}

type RefreshBody struct {
	TTL *int64 `json:"ttl"`
}

type SubscribeBody struct {
	Channel Channel `json:"channel"`
	Status  bool    `json:"status"`
}

type UnsubscribeBody struct {
	Channel Channel `json:"channel"`
	Status  bool    `json:"status"`
}

type PublishBody struct {
	Channel Channel `json:"channel"`
	Status  bool    `json:"status"`
}

type PingBody struct {
	Data string `json:"data"`
}

type StatsBody struct {
	Data Stats `json:"data"`
}

type adminMessageBody struct {
	Message Message `json:"message"`
}
