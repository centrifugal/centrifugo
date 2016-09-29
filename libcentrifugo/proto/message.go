package proto

import (
	"strconv"
	"time"

	"github.com/centrifugal/centrifugo/libcentrifugo/raw"
	"github.com/nats-io/nuid"
)

type (
	// UserID is web application user ID as string.
	UserID string
	// ConnID is a unique connection ID.
	ConnID string
	// MessageID is a unique message ID
	MessageID string
)

func NewClientInfo(user UserID, client ConnID, defaultInfo *raw.Raw, channelInfo *raw.Raw) *ClientInfo {
	return &ClientInfo{
		User:        string(user),
		Client:      string(client),
		DefaultInfo: defaultInfo,
		ChannelInfo: channelInfo,
	}
}

func NewMessage(ch Channel, data []byte, client ConnID, info *ClientInfo) *Message {
	raw := raw.Raw(data)
	return &Message{
		UID:       nuid.Next(),
		Timestamp: strconv.FormatInt(time.Now().Unix(), 10),
		Info:      info,
		Channel:   string(ch),
		Data:      &raw,
		Client:    string(client),
	}
}

func NewJoinMessage(ch Channel, info ClientInfo) *JoinMessage {
	return &JoinMessage{
		Channel: string(ch),
		Data:    info,
	}
}

func NewLeaveMessage(ch Channel, info ClientInfo) *LeaveMessage {
	return &LeaveMessage{
		Channel: string(ch),
		Data:    info,
	}
}

func NewControlMessage(uid string, method string, params []byte) *ControlMessage {
	raw := raw.Raw(params)
	return &ControlMessage{
		UID:    uid,
		Method: method,
		Params: &raw,
	}
}

func NewAdminMessage(method string, params []byte) *AdminMessage {
	raw := raw.Raw(params)
	return &AdminMessage{
		Method: method,
		Params: &raw,
	}
}
