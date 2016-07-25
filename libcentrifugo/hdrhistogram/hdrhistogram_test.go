package hdrhistogram

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRegistryValueRecording(t *testing.T) {
	registry := NewHDRHistogramRegistry()
	hist := NewHDRHistogram("test", 3, 1, 1000, 3, []float64{50.0})
	registry.Register(hist)
	err := registry.RecordValue("test", 100)
	assert.Equal(t, nil, err)
	err = registry.RecordValue("test", 1000000)
	assert.NotEqual(t, nil, err, "out of range should return error")
	err = registry.RecordMicroseconds("test", time.Duration(1000))
	assert.Equal(t, nil, err)
	err = registry.RecordMicroseconds("test", time.Duration(10000000))
	assert.NotEqual(t, nil, err, "out of range should return error")
}

func BenchmarkRegistryRecording(b *testing.B) {
	registry := NewHDRHistogramRegistry()
	hist := NewHDRHistogram("test", 3, 1, 1000, 3, []float64{50.0})
	registry.Register(hist)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := registry.RecordValue("test", int64(i%1000))
		if err != nil {
			panic(err)
		}
	}
}

func BenchmarkRegistryRecordingParallel(b *testing.B) {
	registry := NewHDRHistogramRegistry()
	hist := NewHDRHistogram("test", 3, 1, 1000, 3, []float64{50.0})
	registry.Register(hist)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			err := registry.RecordValue("test", int64(i%1000))
			if err != nil {
				panic(err)
			}
			i++
		}
	})
}
