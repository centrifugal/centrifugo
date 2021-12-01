package apiproto

import (
	"bytes"
	"testing"
)

var testEncodedData []byte

func BenchmarkJSONEncode(b *testing.B) {
	reply := &Reply{Id: 1, Result: []byte(`{"answer":42}`)}
	for i := 0; i < b.N; i++ {
		encoder := GetReplyEncoder()
		err := encoder.Encode(reply)
		if err != nil {
			b.Fatal(err)
		}
		testEncodedData = encoder.Finish()
		if !bytes.Equal([]byte(`{"id":1,"result":{"answer":42}}`), testEncodedData) {
			b.Fatal("unexpected encoded data", string(testEncodedData))
		}
		PutReplyEncoder(encoder)
	}
}
