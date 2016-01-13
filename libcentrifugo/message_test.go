package libcentrifugo

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/centrifugal/centrifugo/Godeps/_workspace/src/github.com/stretchr/testify/assert"
)

func TestMessage(t *testing.T) {
	msg := newMessage(Channel("test"), []byte("{}"), "", nil)
	assert.Equal(t, msg.Channel, Channel("test"))
	msgBytes, err := json.Marshal(msg)
	assert.Equal(t, nil, err)
	assert.Equal(t, true, strings.Contains(string(msgBytes), "\"channel\":\"test\""))
	assert.Equal(t, true, strings.Contains(string(msgBytes), "\"data\":{}"))
	assert.Equal(t, false, strings.Contains(string(msgBytes), "\"client\":\"\"")) // empty field must be omitted
	assert.Equal(t, true, strings.Contains(string(msgBytes), "\"timestamp\":"))
	assert.Equal(t, true, strings.Contains(string(msgBytes), "\"uid\":"))
}

func BenchmarkMsgMarshal(b *testing.B) {
	msg, _ := newMessage(Channel("test"), []byte("{}"), "", nil)
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
