package engineredis

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHash(t *testing.T) {
	chans := []string{"chan1", "chan2", "chan3", "chan4", "chan5", "chan6", "chan7", "chan8"}
	for _, ch := range chans {
		bucket := Hash(ch, 2)
		assert.True(t, bucket >= 0)
		assert.True(t, bucket < 2)
	}
}
