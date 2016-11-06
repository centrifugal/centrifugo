package proto

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMessage(t *testing.T) {
	msg := NewMessage("test", []byte("{}"), "", nil)
	assert.Equal(t, msg.Channel, "test")
	msgBytes, err := json.Marshal(msg)
	assert.Equal(t, nil, err)
	assert.Equal(t, true, strings.Contains(string(msgBytes), "\"channel\":\"test\""))
	assert.Equal(t, true, strings.Contains(string(msgBytes), "\"data\":{}"))
	assert.Equal(t, false, strings.Contains(string(msgBytes), "\"client\":\"\"")) // empty field must be omitted
	assert.Equal(t, true, strings.Contains(string(msgBytes), "\"timestamp\":"))
	assert.Equal(t, true, strings.Contains(string(msgBytes), "\"uid\":"))
	var unmarshalledMsg Message
	err = json.Unmarshal(msgBytes, &unmarshalledMsg)
	assert.Equal(t, nil, err)
	assert.Equal(t, "test", unmarshalledMsg.Channel)
}

func TestMarshalUnmarshal(t *testing.T) {
	data := `{"key": "привет"}`
	msg := NewMessage("test", []byte(data), "", nil)
	encoded, _ := msg.Marshal()
	var newMsg Message
	newMsg.Unmarshal(encoded)
	assert.Equal(t, string(data), string(newMsg.Data))
}

func TestEngineEncodeDecode(t *testing.T) {
	message := NewMessage(string("encode_decode_test"), []byte("{}"), "", nil)
	byteMessage, err := message.Marshal()
	assert.Equal(t, nil, err)
	assert.True(t, bytes.Contains(byteMessage, []byte("encode_decode_test")))
	var decodedMessage Message
	err = decodedMessage.Unmarshal(byteMessage)
	assert.Equal(t, nil, err)
	assert.Equal(t, "encode_decode_test", decodedMessage.Channel)

	joinMessage := NewJoinMessage(string("encode_decode_test"), ClientInfo{})
	byteMessage, err = joinMessage.Marshal()
	assert.Equal(t, nil, err)
	assert.True(t, bytes.Contains(byteMessage, []byte("encode_decode_test")))
	var decodedJoinMessage JoinMessage
	err = decodedJoinMessage.Unmarshal(byteMessage)
	assert.Equal(t, nil, err)
	assert.Equal(t, "encode_decode_test", decodedJoinMessage.Channel)

	leaveMessage := NewLeaveMessage(string("encode_decode_test"), ClientInfo{})
	byteMessage, err = leaveMessage.Marshal()
	assert.Equal(t, nil, err)
	assert.True(t, bytes.Contains(byteMessage, []byte("encode_decode_test")))
	var decodedLeaveMessage LeaveMessage
	err = decodedLeaveMessage.Unmarshal(byteMessage)
	assert.Equal(t, nil, err)
	assert.Equal(t, "encode_decode_test", decodedLeaveMessage.Channel)

	controlMessage := NewControlMessage("test_encode_decode_uid", "ping", []byte("{}"))
	byteMessage, err = controlMessage.Marshal()
	assert.Equal(t, nil, err)
	assert.True(t, bytes.Contains(byteMessage, []byte("test_encode_decode_uid")))
	var decodedControlMessage ControlMessage
	err = decodedControlMessage.Unmarshal(byteMessage)
	assert.Equal(t, nil, err)
	assert.Equal(t, "test_encode_decode_uid", decodedControlMessage.UID)

	adminMessage := NewAdminMessage("test_encode_decode", []byte("{}"))
	byteMessage, err = adminMessage.Marshal()
	assert.Equal(t, nil, err)
	assert.True(t, bytes.Contains(byteMessage, []byte("test_encode_decode")))
	var decodedAdminMessage AdminMessage
	err = decodedAdminMessage.Unmarshal(byteMessage)
	assert.Equal(t, nil, err)
	assert.Equal(t, "test_encode_decode", decodedAdminMessage.Method)
}

func BenchmarkClientResponseMarshalJSON(b *testing.B) {
	responses := make([]*ClientMessageResponse, 10000)
	for i := 0; i < 10000; i++ {
		resp := NewClientMessage(NewMessage("test"+strconv.Itoa(i), []byte("{}"), "", nil))
		responses[i] = resp
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := json.Marshal(responses[i%10000])
		if err != nil {
			panic(err)
		}
	}
}

func BenchmarkClientResponseMarshalManual(b *testing.B) {
	responses := make([]*ClientMessageResponse, 10000)
	for i := 0; i < 10000; i++ {
		resp := NewClientMessage(NewMessage("test"+strconv.Itoa(i), []byte("{}"), "", nil))
		responses[i] = resp
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := responses[i%10000].Marshal()
		if err != nil {
			panic(err)
		}
	}
}

func BenchmarkMsgMarshalJSON(b *testing.B) {
	msg := NewMessage("test", []byte("{}"), "", nil)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := json.Marshal(msg)
		if err != nil {
			panic(err)
		}
	}
}

func BenchmarkMsgMarshalGogoprotobuf(b *testing.B) {
	msg := NewMessage("test", []byte("{}"), "", nil)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := msg.Marshal()
		if err != nil {
			panic(err)
		}
	}
}
