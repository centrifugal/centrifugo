package experiments

import (
	"bufio"
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"sync"
	"testing"

	"github.com/centrifugal/centrifugo/lib/proto"
	"github.com/centrifugal/centrifugo/lib/proto/client"
)

var jsonData = []byte(`{"i": 1, "m": "connect", "p": {"token": "asdfghertyuxcvbnsdfghvbnghjxcvbnfwrhfwruhfb"}}
	{"i": 2, "m": "subscribe", "p": {"channel": "test1"}}
	{"i": 3, "m": "subscribe", "p": {"channel": "test2"}}
	{"i": 4, "m": "subscribe", "p": {"channel": "test3"}}
	{"i": 5, "m": "ping"}`)

var res = []byte(`[
	{"i": 1, "e": {"code": 10001, "message": "permission denied"}},
	{"i": 2, "r": {"messages": [{}, {}]}},
	{"i": 3, "r": {"messages": [{}, {}]}}},
	{"i": 4, "r": {"messages": [{}, {}]}}},
	{"i": 5}
]`)

var async = []byte(`[
	{"r": {"t": "m", "d": {"channel": "test", "data": null}}},
	{"r": {"t": "j", "d": {"channel": "test", "data": null}}}
	{"r": {"t": "l", "d": {"channel": "test", "data": null}}}
]`)

func BenchmarkMarshalJSON(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			msg1 := &proto.Message{
				UID:     "1",
				Channel: "test1",
				Data:    proto.Raw([]byte("{}")),
			}

			msg1json, err := json.Marshal(msg1)
			if err != nil {
				panic(err)
			}

			asyncMessage1json, _ := json.Marshal(client.AsyncMessage{
				Type: 0,
				Data: msg1json,
			})

			reply1json, _ := json.Marshal(client.Reply{
				Result: asyncMessage1json,
			})

			if len(reply1json) == 0 {
				panic("Zero")
			}
		}
	})
}

func BenchmarkMarshalProtobuf(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			msg1 := &proto.Message{
				UID:     "1",
				Channel: "test1",
				Data:    proto.Raw([]byte("{}")),
			}

			msg1binary, _ := msg1.Marshal()

			asyncMessage1binary, _ := (&client.AsyncMessage{
				Type: 0,
				Data: msg1binary,
			}).Marshal()

			reply1binary, _ := (&client.Reply{
				Result: asyncMessage1binary,
			}).Marshal()

			if len(reply1binary) == 0 {
				panic("Zero")
			}
		}
	})
}

var brpool sync.Pool

func getBR(reader io.Reader) *bufio.Reader {
	r := brpool.Get()
	if r == nil {
		return bufio.NewReader(reader)
	}
	br := r.(*bufio.Reader)
	br.Reset(reader)
	return br
}

func putBR(r *bufio.Reader) {
	brpool.Put(r)
}

var rpool sync.Pool

func getR(data []byte) *bytes.Reader {
	r := rpool.Get()
	if r == nil {
		return bytes.NewReader(data)
	}
	reader := r.(*bytes.Reader)
	reader.Reset(data)
	return reader
}

func putR(r *bytes.Reader) {
	rpool.Put(r)
}

func BenchmarkJSONDecodeReader(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			r := getR(jsonData)
			br := getBR(r)
			for {
				l, err := br.ReadBytes('\n')
				var m Command
				if err := json.Unmarshal(l, &m); err != nil {
					panic(err)
				}
				if err == io.EOF {
					break
				}
			}
			putBR(br)
			putR(r)
		}
	})
}

func BenchmarkJSONDecodeSplit(b *testing.B) {
	sep := []byte("\n")
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			parts := bytes.Split(jsonData, sep)
			for _, part := range parts {
				var m Command
				if err := json.Unmarshal(part, &m); err != nil {
					panic(err)
				}
			}
		}
	})
}

func BenchmarkJSONDecodeStream(b *testing.B) {
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			r := getR(jsonData)
			dec := json.NewDecoder(r)
			for {
				var m Command
				if err := dec.Decode(&m); err == io.EOF {
					break
				} else if err != nil {
					panic(err)
				}
			}
			putR(r)
		}
	})
}

func GenerateClientToken(secret, user, timestamp, info string) string {
	token := hmac.New(sha256.New, []byte(secret))
	token.Write([]byte(user))
	token.Write([]byte(timestamp))
	token.Write([]byte(info))
	return hex.EncodeToString(token.Sum(nil))
}

func TestToken(t *testing.T) {
	info := &proto.ClientInfo{
		User:   "42",
		Client: "123",
	}
	infoData, _ := info.Marshal()
	infoString := base64.StdEncoding.EncodeToString([]byte(infoData))
	//infoString := string(infoData)
	println(infoString)
	token := GenerateClientToken("secret", "42", "123", infoString)
	println(token)

	d, _ := base64.StdEncoding.DecodeString(infoString)
	var c proto.ClientInfo
	c.Unmarshal(d)
	println(c.Client)

	infoData, _ = json.Marshal(info)
	infoString = base64.StdEncoding.EncodeToString([]byte(infoData))
	println(infoString)
	token = GenerateClientToken("secret", "42", "123", infoString)
	println(token)

	d, _ = base64.StdEncoding.DecodeString(infoString)
	println(string(d))
	json.Unmarshal(d, &c)
	println(c.Client)
}
