package proto

import (
	"strconv"
	"time"

	"github.com/centrifugal/centrifugo/libcentrifugo/raw"
	"github.com/nats-io/nuid"
)

func NewClientInfo(user string, client string, defaultInfo raw.Raw, channelInfo raw.Raw) *ClientInfo {
	return &ClientInfo{
		User:        user,
		Client:      client,
		DefaultInfo: defaultInfo,
		ChannelInfo: channelInfo,
	}
}

func NewMessage(ch string, data []byte, client string, info *ClientInfo) *Message {
	raw := raw.Raw(data)
	return &Message{
		UID:       nuid.Next(),
		Timestamp: strconv.FormatInt(time.Now().Unix(), 10),
		Info:      info,
		Channel:   ch,
		Data:      raw,
		Client:    client,
	}
}

func NewJoinMessage(ch string, info ClientInfo) *JoinMessage {
	return &JoinMessage{
		Channel: ch,
		Data:    info,
	}
}

func NewLeaveMessage(ch string, info ClientInfo) *LeaveMessage {
	return &LeaveMessage{
		Channel: ch,
		Data:    info,
	}
}

func NewControlMessage(uid string, method string, params []byte) *ControlMessage {
	raw := raw.Raw(params)
	return &ControlMessage{
		UID:    uid,
		Method: method,
		Params: raw,
	}
}

func NewAdminMessage(method string, params []byte) *AdminMessage {
	raw := raw.Raw(params)
	return &AdminMessage{
		Method: method,
		Params: raw,
	}
}
