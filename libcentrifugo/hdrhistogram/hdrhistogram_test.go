package hdrhistogram

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHDRHistogram(t *testing.T) {

}

func BenchmarkHDRHistogram(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		println(1)
	}
}
