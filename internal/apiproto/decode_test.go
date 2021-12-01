package apiproto

import "testing"

func BenchmarkJSONDecode(b *testing.B) {
	input := []byte(`{"id": 1, "method": "publish", "params": {"channel": "test", "data": {}}}`)
	for i := 0; i < b.N; i++ {
		decoder := GetCommandDecoder(input)
		cmd, err := decoder.Decode()
		if err != nil {
			b.Fatal(err)
		}
		if cmd.Method != Command_PUBLISH {
			b.Fatal("wrong method")
		}
		PutCommandDecoder(decoder)
	}
}
