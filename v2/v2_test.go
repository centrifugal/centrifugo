package v2

import (
	"encoding/json"
	"testing"
)

type Command struct {
	ID     int             `json:"i"`
	Method int             `json:"m"`
	Params json.RawMessage `json:"p"`
}

var json1 = []byte(`[
	{"i": 1, "m": "connect", "p": {}},
	{"i": 2, "m": "subscribe", "p": {"channel": "test1"}},
	{"i": 3, "m": "subscribe", "p": {"channel": "test2"}},
	{"i": 4, "m": "subscribe", "p": {"channel": "test3"}},
	{"i": 5, "m": "ping"},
]`)

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

func BenchmarkArray(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			var cmds []Command
			err := json.Unmarshal(json1, &cmds)
			if err != nil {
				panic(err)
			}
		}
	})
}
