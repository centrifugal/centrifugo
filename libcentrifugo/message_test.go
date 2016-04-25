package libcentrifugo

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMessage(t *testing.T) {
	msg := newMessage(Channel("test"), []byte("{}"), "", nil)
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

func BenchmarkClientResponseMarshal(b *testing.B) {
	msg := newMessage(Channel("test"), []byte("{}"), "", nil)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp := newClientMessage()
		resp.Body = msg
		_, err := json.Marshal(resp)
		if err != nil {
			panic(err)
		}
	}
}

func BenchmarkMsgMarshalJSON(b *testing.B) {
	msg := newMessage(Channel("test"), []byte("{}"), "", nil)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := json.Marshal(msg)
		if err != nil {
			panic(err)
		}
	}
}

func BenchmarkMsgMarshalGogoprotobuf(b *testing.B) {
	msg := newMessage(Channel("test"), []byte("{}"), "", nil)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := msg.Marshal()
		if err != nil {
			panic(err)
		}
	}
}
