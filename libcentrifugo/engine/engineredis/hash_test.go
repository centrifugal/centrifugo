package engineredis

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConsistentHash(t *testing.T) {
	chans := []string{"chan1", "chan2", "chan3", "chan4", "chan5", "chan6", "chan7", "chan8"}
	for _, ch := range chans {
		bucket := consistentHash(ch, 2)
		assert.True(t, bucket >= 0)
		assert.True(t, bucket < 2)
	}
}

func BenchmarkConsistentHash(b *testing.B) {
	for i := 0; i < b.N; i++ {
		consistentHash(strconv.Itoa(i), 4)
	}
}

func BenchmarkHash(b *testing.B) {
	for i := 0; i < b.N; i++ {
		hash(strconv.Itoa(i), 4)
	}
}
