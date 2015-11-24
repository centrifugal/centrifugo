package libcentrifugo

import (
	"encoding/json"
	"strconv"
	"time"

	"github.com/centrifugal/centrifugo/Godeps/_workspace/src/github.com/nu7hatch/gouuid"
)

// Message represents client message.
type Message struct {
	UID       string           `json:"uid"`
	Timestamp string           `json:"timestamp"`
	Info      *ClientInfo      `json:"info"`
	Channel   Channel          `json:"channel"`
	Data      *json.RawMessage `json:"data"`
	Client    ConnID           `json:"client"`
}

func newMessage(ch Channel, data []byte, client ConnID, info *ClientInfo) (Message, error) {
	uid, err := uuid.NewV4()
	if err != nil {
		return Message{}, err
	}

	raw := json.RawMessage(data)

	message := Message{
		UID:       uid.String(),
		Timestamp: strconv.FormatInt(time.Now().Unix(), 10),
		Info:      info,
		Channel:   ch,
		Data:      &raw,
		Client:    client,
	}
	return message, nil
}
