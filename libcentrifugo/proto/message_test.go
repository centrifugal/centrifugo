package proto

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMessage(t *testing.T) {
	msg := NewMessage(Channel("test"), []byte("{}"), "", nil)
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

func BenchmarkClientResponseMarshalJSON(b *testing.B) {
	responses := make([]*ClientMessageResponse, 10000)
	for i := 0; i < 10000; i++ {
		resp := NewClientMessage(NewMessage(Channel("test"+strconv.Itoa(i)), []byte("{}"), "", nil))
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
		resp := NewClientMessage(NewMessage(Channel("test"+strconv.Itoa(i)), []byte("{}"), "", nil))
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
	msg := NewMessage(Channel("test"), []byte("{}"), "", nil)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := json.Marshal(msg)
		if err != nil {
			panic(err)
		}
	}
}

func BenchmarkMsgMarshalGogoprotobuf(b *testing.B) {
	msg := NewMessage(Channel("test"), []byte("{}"), "", nil)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := msg.Marshal()
		if err != nil {
			panic(err)
		}
	}
}
