package libcentrifugo

type PresenceBody struct {
	Channel Channel               `json:"channel"`
	Data    map[ConnID]ClientInfo `json:"data"`
}

type HistoryBody struct {
	Channel Channel   `json:"channel"`
	Data    []Message `json:"data"`
}

type adminMessageBody struct {
	Project ProjectKey `json:"project"`
	Message Message    `json:"message"`
}

type joinLeaveBody struct {
	Channel Channel    `json:"channel"`
	Data    ClientInfo `json:"data"`
}

type connectBody struct {
	Client  *ConnID `json:"client"`
	Expired bool    `json:"expired"`
	TTL     *int64  `json:"ttl"`
}

type refreshBody struct {
	TTL *int64 `json:"ttl"`
}

type subscribeBody struct {
	Channel Channel `json:"channel"`
}

type unsubscribeBody struct {
	Channel Channel `json:"channel"`
}

type publishBody struct {
	Channel Channel `json:"channel"`
	Status  bool    `json:"status"`
}
