package libcentrifugo

import (
	"encoding/json"
	"strconv"
	"time"

	"github.com/centrifugal/centrifugo/Godeps/_workspace/src/github.com/nats-io/nuid"
)

// Message represents client message.
type Message struct {
	UID       MessageID        `json:"uid"`
	Timestamp string           `json:"timestamp"`
	Info      *ClientInfo      `json:"info,omitempty"`
	Channel   Channel          `json:"channel"`
	Data      *json.RawMessage `json:"data"`
	Client    ConnID           `json:"client,omitempty"`
}

func newMessage(ch Channel, data []byte, client ConnID, info *ClientInfo) Message {
	raw := json.RawMessage(data)
	return Message{
		UID:       MessageID(nuid.Next()),
		Timestamp: strconv.FormatInt(time.Now().Unix(), 10),
		Info:      info,
		Channel:   ch,
		Data:      &raw,
		Client:    client,
	}
}
