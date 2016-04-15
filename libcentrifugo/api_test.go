package libcentrifugo

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/centrifugal/centrifugo/Godeps/_workspace/src/github.com/stretchr/testify/assert"
)

func TestAPICmd(t *testing.T) {
	app := testApp()

	cmd := apiCommand{
		Method: "nonexistent",
		Params: []byte("{}"),
	}
	_, err := app.apiCmd(cmd)
	assert.Equal(t, err, ErrMethodNotFound)

	cmd = apiCommand{
		Method: "publish",
		Params: []byte("{}"),
	}
	resp, err := app.apiCmd(cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, ErrInvalidMessage, resp.err)

	cmd = apiCommand{
		Method: "publish",
		Params: []byte("test"),
	}
	_, err = app.apiCmd(cmd)
	assert.Equal(t, ErrInvalidMessage, err)

	cmd = apiCommand{
		Method: "broadcast",
		Params: []byte("{}"),
	}
	resp, err = app.apiCmd(cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, ErrInvalidMessage, resp.err)

	cmd = apiCommand{
		Method: "broadcast",
		Params: []byte("test"),
	}
	_, err = app.apiCmd(cmd)
	assert.Equal(t, ErrInvalidMessage, err)

	cmd = apiCommand{
		Method: "unsubscribe",
		Params: []byte("{}"),
	}
	resp, err = app.apiCmd(cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, ErrInvalidMessage, resp.err)

	cmd = apiCommand{
		Method: "unsubscribe",
		Params: []byte("test"),
	}
	_, err = app.apiCmd(cmd)
	assert.Equal(t, ErrInvalidMessage, err)

	cmd = apiCommand{
		Method: "disconnect",
		Params: []byte("{}"),
	}
	resp, err = app.apiCmd(cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, ErrInvalidMessage, resp.err)

	cmd = apiCommand{
		Method: "disconnect",
		Params: []byte("test"),
	}
	_, err = app.apiCmd(cmd)
	assert.Equal(t, ErrInvalidMessage, err)

	cmd = apiCommand{
		Method: "presence",
		Params: []byte("{}"),
	}
	resp, err = app.apiCmd(cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, ErrInvalidMessage, resp.err)

	cmd = apiCommand{
		Method: "presence",
		Params: []byte("test"),
	}
	_, err = app.apiCmd(cmd)
	assert.Equal(t, ErrInvalidMessage, err)

	cmd = apiCommand{
		Method: "history",
		Params: []byte("{}"),
	}
	resp, err = app.apiCmd(cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, ErrInvalidMessage, resp.err)

	cmd = apiCommand{
		Method: "history",
		Params: []byte("test"),
	}
	_, err = app.apiCmd(cmd)
	assert.Equal(t, ErrInvalidMessage, err)

	cmd = apiCommand{
		Method: "channels",
		Params: []byte("{}"),
	}
	resp, err = app.apiCmd(cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, resp.err)

	cmd = apiCommand{
		Method: "stats",
		Params: []byte("{}"),
	}
	resp, err = app.apiCmd(cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, resp.err)

	cmd = apiCommand{
		Method: "node",
		Params: []byte("{}"),
	}
	resp, err = app.apiCmd(cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, resp.err)
}

func TestAPIPublish(t *testing.T) {
	app := testApp()
	cmd := &publishAPICommand{
		Channel: "channel",
		Data:    []byte("null"),
	}
	resp, err := app.publishCmd(cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, resp.err)
	cmd = &publishAPICommand{
		Channel: "nonexistentnamespace:channel-2",
		Data:    []byte("null"),
	}
	resp, err = app.publishCmd(cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, ErrNamespaceNotFound, resp.err)
}

func TestAPIBroadcast(t *testing.T) {
	app := testApp()
	cmd := &broadcastAPICommand{
		Channels: []Channel{"channel-1", "channel-2"},
		Data:     []byte("null"),
	}
	resp, err := app.broadcastCmd(cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, resp.err)
	cmd = &broadcastAPICommand{
		Channels: []Channel{"channel-1", "nonexistentnamespace:channel-2"},
		Data:     []byte("null"),
	}
	resp, err = app.broadcastCmd(cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, ErrNamespaceNotFound, resp.err)
	cmd = &broadcastAPICommand{
		Channels: []Channel{},
		Data:     []byte("null"),
	}
	resp, err = app.broadcastCmd(cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, ErrInvalidMessage, resp.err)
}

func TestAPIUnsubscribe(t *testing.T) {
	app := testApp()
	cmd := &unsubscribeAPICommand{
		User:    "test user",
		Channel: "channel",
	}
	resp, err := app.unsubcribeCmd(cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, resp.err)

	// unsubscribe from all channels
	cmd = &unsubscribeAPICommand{
		User:    "test user",
		Channel: "",
	}
	resp, err = app.unsubcribeCmd(cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, resp.err)
}

func TestAPIDisconnect(t *testing.T) {
	app := testApp()
	cmd := &disconnectAPICommand{
		User: "test user",
	}
	resp, err := app.disconnectCmd(cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, resp.err)
}

func TestAPIPresence(t *testing.T) {
	app := testApp()
	cmd := &presenceAPICommand{
		Channel: "channel",
	}
	resp, err := app.presenceCmd(cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, resp.err)
}

func TestAPIHistory(t *testing.T) {
	app := testApp()
	cmd := &historyAPICommand{
		Channel: "channel",
	}
	resp, err := app.historyCmd(cmd)
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, resp.err)
}

func TestAPIChannels(t *testing.T) {
	app := testApp()
	resp, err := app.channelsCmd()
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, resp.err)
	app = testMemoryApp()
	createTestClients(app, 10, 1, nil)
	resp, err = app.channelsCmd()
	assert.Equal(t, nil, err)
	body := resp.Body.(*ChannelsBody)
	assert.Equal(t, 10, len(body.Data))
}

func TestAPIStats(t *testing.T) {
	app := testApp()
	resp, err := app.statsCmd()
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, resp.err)
}

func TestAPINode(t *testing.T) {
	app := testApp()
	resp, err := app.nodeCmd()
	assert.Equal(t, nil, err)
	assert.Equal(t, nil, resp.err)
}

func getNPublishJSON(channel string, n int) []byte {
	commands := make([]map[string]interface{}, n)
	command := map[string]interface{}{
		"method": "publish",
		"params": map[string]interface{}{
			"channel": channel,
			"data":    map[string]bool{"benchmarking": true},
		},
	}
	for i := 0; i < n; i++ {
		commands[i] = command
	}
	jsonData, _ := json.Marshal(commands)
	return jsonData
}

func getPublishJSON(channel string) []byte {
	commands := make([]map[string]interface{}, 1)
	command := map[string]interface{}{
		"method": "publish",
		"params": map[string]interface{}{
			"channel": channel,
			"data":    map[string]bool{"benchmarking": true},
		},
	}
	commands[0] = command
	jsonData, _ := json.Marshal(commands)
	return jsonData
}

func getNChannelsBroadcastJSON(n int) []byte {
	channels := make([]string, n)
	for i := 0; i < n; i++ {
		channels[i] = fmt.Sprintf("channel-%d", i)
	}
	commands := make([]map[string]interface{}, 1)
	command := map[string]interface{}{
		"method": "broadcast",
		"params": map[string]interface{}{
			"channels": channels,
			"data":     map[string]bool{"benchmarking": true},
		},
	}
	commands[0] = command
	jsonData, _ := json.Marshal(commands)
	return jsonData
}

func getManyNChannelsBroadcastJSON(nChannels int, nCommands int) []byte {
	channels := make([]string, nChannels)
	for i := 0; i < nChannels; i++ {
		channels[i] = fmt.Sprintf("channel-%d", i)
	}
	commands := make([]map[string]interface{}, nCommands)
	command := map[string]interface{}{
		"method": "broadcast",
		"params": map[string]interface{}{
			"channels": channels,
			"data":     map[string]bool{"benchmarking": true},
		},
	}
	for i := 0; i < nCommands; i++ {
		commands[i] = command
	}
	jsonData, _ := json.Marshal(commands)
	return jsonData
}

func testAPIrequest(app *Application, data []byte) ([]byte, error) {
	commands, err := cmdFromRequestMsg(data)
	if err != nil {
		return nil, err
	}

	var mr multiResponse

	for _, command := range commands {
		resp, err := app.apiCmd(command)
		if err != nil {
			return nil, err
		}
		mr = append(mr, resp)
	}
	return json.Marshal(mr)
}

// BenchmarkSerializationPublish allows to bench API request containing single
// publish command.
func BenchmarkSerializationPublish(b *testing.B) {
	app := testMemoryApp()
	jsonData := getPublishJSON("channel")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := testAPIrequest(app, jsonData)
		if err != nil {
			b.Error(err)
		}
	}
}

// BenchmarkSerializationPublishMany allows to bench API request containing many
// publish commands as array.
func BenchmarkSerializationPublishMany(b *testing.B) {
	app := testMemoryApp()
	jsonData := getNPublishJSON("channel", 1000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := testAPIrequest(app, jsonData)
		if err != nil {
			b.Error(err)
		}
	}
}

// BenchmarkSerializationBroadcast allows to bench broadcast API request containing single
// broadcast command into many channels.
func BenchmarkSerializationBroadcast(b *testing.B) {
	app := testMemoryApp()
	jsonData := getNChannelsBroadcastJSON(1000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := testAPIrequest(app, jsonData)
		if err != nil {
			b.Error(err)
		}
	}
}

// BenchmarkSerializationBroadcastMany allows to bench broadcast API request containing many
// broadcast commands into many channels.
func BenchmarkSerializationBroadcastMany(b *testing.B) {
	app := testMemoryApp()
	jsonData := getManyNChannelsBroadcastJSON(100, 100)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := testAPIrequest(app, jsonData)
		if err != nil {
			b.Error(err)
		}
	}
}
