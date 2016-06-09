package libcentrifugo

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

type testEngine struct{}

func newTestEngine() *testEngine {
	return &testEngine{}
}

func (e *testEngine) name() string {
	return "test engine"
}

func (e *testEngine) run() error {
	return nil
}

func (e *testEngine) publishMessage(ch Channel, message *Message, opts *ChannelOptions) <-chan error {
	eChan := make(chan error, 1)
	eChan <- nil
	return eChan
}

func (e *testEngine) publishJoin(ch Channel, message *JoinMessage) <-chan error {
	eChan := make(chan error, 1)
	eChan <- nil
	return eChan
}

func (e *testEngine) publishLeave(ch Channel, message *LeaveMessage) <-chan error {
	eChan := make(chan error, 1)
	eChan <- nil
	return eChan
}

func (e *testEngine) publishAdmin(message *AdminMessage) <-chan error {
	eChan := make(chan error, 1)
	eChan <- nil
	return eChan
}

func (e *testEngine) publishControl(message *ControlMessage) <-chan error {
	eChan := make(chan error, 1)
	eChan <- nil
	return eChan
}

func (e *testEngine) subscribe(ch Channel) error {
	return nil
}

func (e *testEngine) unsubscribe(ch Channel) error {
	return nil
}

func (e *testEngine) addPresence(ch Channel, uid ConnID, info ClientInfo) error {
	return nil
}

func (e *testEngine) removePresence(ch Channel, uid ConnID) error {
	return nil
}

func (e *testEngine) presence(ch Channel) (map[ConnID]ClientInfo, error) {
	return map[ConnID]ClientInfo{}, nil
}

func (e *testEngine) history(ch Channel, opts historyOpts) ([]Message, error) {
	return []Message{}, nil
}

func (e *testEngine) channels() ([]Channel, error) {
	return []Channel{}, nil
}

func TestEngineEncodeDecode(t *testing.T) {
	message := newMessage(Channel("encode_decode_test"), []byte("{}"), "", nil)
	byteMessage, err := encodeEngineClientMessage(message)
	assert.Equal(t, nil, err)
	assert.True(t, bytes.Contains(byteMessage, []byte("encode_decode_test")))

	decodedMessage, err := decodeEngineClientMessage(byteMessage)
	assert.Equal(t, nil, err)
	assert.Equal(t, "encode_decode_test", decodedMessage.Channel)

	joinMessage := newJoinMessage(Channel("encode_decode_test"), ClientInfo{})
	byteMessage, err = encodeEngineJoinMessage(joinMessage)
	assert.Equal(t, nil, err)
	assert.True(t, bytes.Contains(byteMessage, []byte("encode_decode_test")))

	decodedJoinMessage, err := decodeEngineJoinMessage(byteMessage)
	assert.Equal(t, nil, err)
	assert.Equal(t, "encode_decode_test", decodedJoinMessage.Channel)

	leaveMessage := newLeaveMessage(Channel("encode_decode_test"), ClientInfo{})
	byteMessage, err = encodeEngineLeaveMessage(leaveMessage)
	assert.Equal(t, nil, err)
	assert.True(t, bytes.Contains(byteMessage, []byte("encode_decode_test")))

	decodedLeaveMessage, err := decodeEngineLeaveMessage(byteMessage)
	assert.Equal(t, nil, err)
	assert.Equal(t, "encode_decode_test", decodedLeaveMessage.Channel)

	controlMessage := newControlMessage("test_encode_decode_uid", "ping", []byte("{}"))
	byteMessage, err = encodeEngineControlMessage(controlMessage)
	assert.Equal(t, nil, err)
	assert.True(t, bytes.Contains(byteMessage, []byte("test_encode_decode_uid")))

	decodedControlMessage, err := decodeEngineControlMessage(byteMessage)
	assert.Equal(t, nil, err)
	assert.Equal(t, "test_encode_decode_uid", decodedControlMessage.UID)

	adminMessage := newAdminMessage("test_encode_decode", []byte("{}"))
	byteMessage, err = encodeEngineAdminMessage(adminMessage)
	assert.Equal(t, nil, err)
	assert.True(t, bytes.Contains(byteMessage, []byte("test_encode_decode")))

	decodedAdminMessage, err := decodeEngineAdminMessage(byteMessage)
	assert.Equal(t, nil, err)
	assert.Equal(t, "test_encode_decode", decodedAdminMessage.Method)
}
