package convert

import "testing"

var tests = []string{
	"abc",
	"hello world",
	"tests",
}

func TestBytesToString(t *testing.T) {
	for _, want := range tests {
		if got := BytesToString([]byte(want)); got != want {
			t.Errorf("BytesToString() = %s, want %s", got, want)
		}
	}
}

func TestStringToBytes(t *testing.T) {
	for _, want := range tests {
		if got := StringToBytes(want); string(got) != want {
			t.Errorf("StringToBytes() = %s, want %s", got, want)
		}
	}
}
