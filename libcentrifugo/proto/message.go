package proto

import (
	"strconv"
	"time"

	"github.com/centrifugal/centrifugo/libcentrifugo/raw"
	"github.com/nats-io/nuid"
)

// NewClientInfo allows to initialize ClientInfo.
func NewClientInfo(user string, client string, defaultInfo raw.Raw, channelInfo raw.Raw) *ClientInfo {
	return &ClientInfo{
		User:        user,
		Client:      client,
		DefaultInfo: defaultInfo,
		ChannelInfo: channelInfo,
	}
}

// NewMessage initializes new Message.
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

// NewJoinMessage initializes new JoinMessage.
func NewJoinMessage(ch string, info ClientInfo) *JoinMessage {
	return &JoinMessage{
		Channel: ch,
		Data:    info,
	}
}

// NewLeaveMessage initializes new LeaveMessage.
func NewLeaveMessage(ch string, info ClientInfo) *LeaveMessage {
	return &LeaveMessage{
		Channel: ch,
		Data:    info,
	}
}

// NewControlMessage initializes new ControlMessage.
func NewControlMessage(uid string, method string, params []byte) *ControlMessage {
	raw := raw.Raw(params)
	return &ControlMessage{
		UID:    uid,
		Method: method,
		Params: raw,
	}
}

// NewAdminMessage initializes new AdminMessage.
func NewAdminMessage(method string, params []byte) *AdminMessage {
	raw := raw.Raw(params)
	return &AdminMessage{
		Method: method,
		Params: raw,
	}
}
