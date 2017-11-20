package client

import (
	"encoding/json"
)

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

// // Command describes single request command.
// type CommandJ struct {
// 	ID     int     `json:"i"`
// 	Method int     `json:"m"`
// 	Params raw.Raw `json:"p"`
// }

// // Connect is a command to authorize connection - it contains user ID
// // in web application, additional connection information as JSON string, timestamp
// // with unix seconds on moment when connect parameters generated and HMAC token to
// // prove correctness of all those parameters.
// type ConnectJ struct {
// 	User      string `json:"user"`
// 	Timestamp string `json:"timestamp"`
// 	Info      string `json:"info"`
// 	Token     string `json:"token"`
// }

// // Refresh is used to prolong connection lifetime when connection check
// // mechanism is enabled. It can only be sent by client after successful connect.
// type RefreshJ struct {
// 	User      string `json:"user"`
// 	Timestamp string `json:"timestamp"`
// 	Info      string `json:"info"`
// 	Token     string `json:"token"`
// }

// // Subscribe is used to subscribe on channel.
// // It can only be sent by client after successful connect.
// // It also can have Client, Info and Sign properties when channel is private.
// type SubscribeJ struct {
// 	Channel string `json:"channel"`
// 	Client  string `json:"client"`
// 	Info    string `json:"info"`
// 	Sign    string `json:"sign"`
// 	Recover bool   `json:"recover"`
// 	Last    string `json:"last"`
// 	Passive bool   `json:"passive"`
// }

// // Unsubscribe is used to unsubscribe from channel.
// type UnsubscribeJ struct {
// 	Channel string `json:"channel"`
// }

// // Publish is used to publish messages into channel.
// type PublishJ struct {
// 	Channel string  `json:"channel"`
// 	Data    raw.Raw `json:"data"`
// }

// // Presence is used to get presence (actual channel subscriptions).
// // information for channel
// type PresenceJ struct {
// 	Channel string `json:"channel"`
// }

// // History is used to get history information for channel.
// type HistoryJ struct {
// 	Channel string `json:"channel"`
// }

// // Ping is used to ping server.
// type PingJ struct {
// 	Data string `json:"data"`
// }
