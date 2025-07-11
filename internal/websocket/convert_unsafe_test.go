package websocket

import "testing"

var tests = []string{
	"abc",
	"hello world",
	"tests",
}

func TestStringToBytes(t *testing.T) {
	for _, want := range tests {
		if got := stringToBytes(want); string(got) != want {
			t.Errorf("StringToBytes() = %s, want %s", got, want)
		}
	}
}
