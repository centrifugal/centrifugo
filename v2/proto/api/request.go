package server

import (
	"encoding/json"

	"github.com/centrifugal/centrifugo/libcentrifugo/raw"
)

// Command describes single request command.
type Command struct {
	ID     int     `json:"i"`
	Method int     `json:"m"`
	Params raw.Raw `json:"p"`
}

var (
	arrayJSONPrefix  byte = '['
	objectJSONPrefix byte = '{'
)

// CommandsFromJSON extracts slice of Command from request encoded as JSON.
func CommandsFromJSON(data []byte) ([]Command, error) {
	var cmds []Command
	firstByte := data[0]
	switch firstByte {
	case objectJSONPrefix:
		// single command request
		var cmd Command
		err := json.Unmarshal(data, &cmd)
		if err != nil {
			return nil, err
		}
		cmds = append(cmds, cmd)
	case arrayJSONPrefix:
		// array of commands received
		err := json.Unmarshal(data, &cmds)
		if err != nil {
			return nil, err
		}
	}
	return cmds, nil
}

// NodeInfo contains information and statistics about Centrifugo node.
type NodeInfo struct {
	UID     string           `json:"uid"`
	Name    string           `json:"name"`
	Started int64            `json:"started_at"`
	Metrics map[string]int64 `json:"metrics"`
}

// Stats contains state and metrics information from running Centrifugo nodes.
type Stats struct {
	Nodes           []NodeInfo `json:"nodes"`
	MetricsInterval int64      `json:"metrics_interval"`
}

// Publish is used to publish messages into channel.
type Publish struct {
	Channel string  `json:"channel"`
	Client  string  `json:"client"`
	Data    raw.Raw `json:"data"`
}

// Broadcast is used to publish messages into multiple channels.
type Broadcast struct {
	Channels []string `json:"channels"`
	Data     raw.Raw  `json:"data"`
	Client   string   `json:"client"`
}

// Unsubscribe is used to unsubscribe user from channel.
type Unsubscribe struct {
	Channel string `json:"channel"`
	User    string `json:"user"`
}

// Disconnect is used to disconnect user.
type Disconnect struct {
	User string `json:"user"`
}

// Presence is used to get presence (actual channel subscriptions)
// information for channel.
type Presence struct {
	Channel string `json:"channel"`
}

// History is used to get history information for channel.
type History struct {
	Channel string `json:"channel"`
}
