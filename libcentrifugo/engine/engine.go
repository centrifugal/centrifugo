package engine

import (
	"github.com/centrifugal/centrifugo/libcentrifugo/config"
	"github.com/centrifugal/centrifugo/libcentrifugo/message"
)

// Engine is an interface with all methods that can be used by client or
// application to publish message, handle subscriptions, save or retrieve
// presence and history data.
type Engine interface {
	// Name returns a name of concrete engine implementation.
	Name() string

	// Run called once on Centrifugo start just after engine set to application.
	Run() error

	// PublishMessage allows to send message into channel. This message should be delivered
	// to all clients subscribed on this channel at moment on any Centrifugo node.
	// The returned value is channel in which we will send error as soon as engine finishes
	// publish operation. Also the task of this method is to maintain history for channels
	// if enabled.
	PublishMessage(message.Channel, *message.Message, *config.ChannelOptions) <-chan error
	// PublishJoin allows to send join message into channel.
	PublishJoin(message.Channel, *message.JoinMessage) <-chan error
	// PublishLeave allows to send leave message into channel.
	PublishLeave(message.Channel, *message.LeaveMessage) <-chan error
	// PublishControl allows to send control message to all connected nodes.
	PublishControl(*message.ControlMessage) <-chan error
	// PublishAdmin allows to send admin message to all connected admins.
	PublishAdmin(*message.AdminMessage) <-chan error

	// Subscribe on channel.
	Subscribe(message.Channel) error
	// Unsubscribe from channel.
	Unsubscribe(message.Channel) error
	// Channels returns slice of currently active channels (with one or more subscribers)
	// on all Centrifugo nodes.
	Channels() ([]message.Channel, error)

	// AddPresence sets or updates presence info in channel for connection with uid.
	AddPresence(message.Channel, message.ConnID, message.ClientInfo) error
	// RemovePresence removes presence information for connection with uid.
	RemovePresence(message.Channel, message.ConnID) error
	// Presence returns actual presence information for channel.
	Presence(message.Channel) (map[message.ConnID]message.ClientInfo, error)

	// History returns a slice of history messages for channel.
	// Integer limit sets the max amount of messages that must be returned. 0 means no limit - i.e.
	// return all history messages (actually limited by configured history_size).
	History(ch message.Channel, limit int) ([]message.Message, error)
}

func decodeEngineClientMessage(data []byte) (*message.Message, error) {
	var msg message.Message
	err := msg.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &msg, nil
}

func decodeEngineJoinMessage(data []byte) (*message.JoinMessage, error) {
	var msg message.JoinMessage
	err := msg.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &msg, nil
}

func decodeEngineLeaveMessage(data []byte) (*message.LeaveMessage, error) {
	var msg message.LeaveMessage
	err := msg.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &msg, nil
}

func decodeEngineControlMessage(data []byte) (*message.ControlMessage, error) {
	var msg message.ControlMessage
	err := msg.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &msg, nil
}

func decodeEngineAdminMessage(data []byte) (*message.AdminMessage, error) {
	var msg message.AdminMessage
	err := msg.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &msg, nil
}

func encodeEngineClientMessage(msg *message.Message) ([]byte, error) {
	return msg.Marshal()
}

func encodeEngineJoinMessage(msg *message.JoinMessage) ([]byte, error) {
	return msg.Marshal()
}

func encodeEngineLeaveMessage(msg *message.LeaveMessage) ([]byte, error) {
	return msg.Marshal()
}

func encodeEngineControlMessage(msg *message.ControlMessage) ([]byte, error) {
	return msg.Marshal()
}

func encodeEngineAdminMessage(msg *message.AdminMessage) ([]byte, error) {
	return msg.Marshal()
}
