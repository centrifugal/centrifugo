package metrics

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestHDRHistogram(t *testing.T) {
	hist := NewHDRHistogram(3, 1, 1000, 3, []float64{50.0}, "")
	err := hist.RecordValue(100)
	assert.Equal(t, nil, err)
	err = hist.RecordValue(1000000)
	assert.NotEqual(t, nil, err, "out of range should return error")
	err = hist.RecordMicroseconds(time.Duration(1000))
	assert.Equal(t, nil, err)
	err = hist.RecordMicroseconds(time.Duration(10000000))
	assert.NotEqual(t, nil, err, "out of range should return error")
}

func TestRegistryRegister(t *testing.T) {
	registry := NewHDRHistogramRegistry()
	hist := NewHDRHistogram(3, 1, 1000, 3, []float64{50.0}, "")
	registry.Register("test", hist)
	assert.Equal(t, 1, len(registry.histograms))
}

func TestRegistryValueRecording(t *testing.T) {
	registry := NewHDRHistogramRegistry()
	hist := NewHDRHistogram(3, 1, 1000, 3, []float64{50.0}, "")
	registry.Register("test", hist)
	err := registry.RecordValue("test", 100)
	assert.Equal(t, nil, err)
	err = registry.RecordValue("test", 1000000)
	assert.NotEqual(t, nil, err, "out of range should return error")
	err = registry.RecordMicroseconds("test", time.Duration(1000))
	assert.Equal(t, nil, err)
	err = registry.RecordMicroseconds("test", time.Duration(10000000))
	assert.NotEqual(t, nil, err, "out of range should return error")
}

func TestRegistryRotate(t *testing.T) {
	registry := NewHDRHistogramRegistry()
	hist1 := NewHDRHistogram(3, 1, 1000, 3, []float64{50.0}, "")
	hist2 := NewHDRHistogram(3, 1, 1000, 3, []float64{50.0}, "")
	registry.Register("test1", hist1)
	registry.Register("test2", hist2)
	registry.RecordValue("test1", 100)
	registry.RecordValue("test2", 200)
	assert.Equal(t, registry.histograms["test1"].hist.Current.Max(), int64(100))
	assert.Equal(t, registry.histograms["test2"].hist.Current.Max(), int64(200))
	registry.Rotate()
	assert.Equal(t, registry.histograms["test1"].hist.Current.Max(), int64(0))
	assert.Equal(t, registry.histograms["test2"].hist.Current.Max(), int64(0))
}

func TestRegistryLoadValues(t *testing.T) {
	registry := NewHDRHistogramRegistry()
	hist1 := NewHDRHistogram(3, 1, 1000, 3, []float64{50.0}, "")
	hist2 := NewHDRHistogram(3, 1, 1000, 3, []float64{50.0}, "")
	registry.Register("test1", hist1)
	registry.Register("test2", hist2)
	registry.RecordValue("test1", 100)
	registry.RecordValue("test2", 200)
	registry.RecordValue("test1", 200)
	registry.RecordValue("test2", 100)
	values := registry.LoadValues()
	assert.Equal(t, int64(150), values["test1_3_mean"])
	assert.Equal(t, int64(150), values["test1_1_mean"])
	assert.Equal(t, int64(150), values["test2_3_mean"])
	assert.Equal(t, int64(150), values["test2_1_mean"])
	assert.Equal(t, int64(200), values["test1_3_max"])
	assert.Equal(t, int64(200), values["test1_1_max"])
	assert.Equal(t, int64(200), values["test2_3_max"])
	assert.Equal(t, int64(200), values["test2_1_max"])
	registry.Rotate()
	registry.RecordValue("test1", 200)
	registry.RecordValue("test2", 300)
	registry.RecordValue("test1", 300)
	registry.RecordValue("test2", 200)
	values = registry.LoadValues()
	assert.Equal(t, int64(200), values["test1_3_mean"])
	assert.Equal(t, int64(250), values["test1_1_mean"])
	assert.Equal(t, int64(200), values["test2_3_mean"])
	assert.Equal(t, int64(250), values["test2_1_mean"])
	assert.Equal(t, int64(300), values["test1_3_max"])
	assert.Equal(t, int64(300), values["test1_1_max"])
	assert.Equal(t, int64(300), values["test2_3_max"])
	assert.Equal(t, int64(300), values["test2_1_max"])
}

func TestRegistryLoadValuesCustomOnly(t *testing.T) {
	registry := NewHDRHistogramRegistry()
	hist1 := NewHDRHistogram(3, 1, 1000, 3, []float64{50.0}, "")
	hist2 := NewHDRHistogram(3, 1, 1000, 3, []float64{50.0}, "")
	registry.Register("test1", hist1)
	registry.Register("test2", hist2)
	values := registry.LoadValues("test2")
	_, ok := values["test2_3_mean"]
	assert.True(t, ok)
	_, ok = values["test1_3_mean"]
	assert.False(t, ok)
}

func TestRegistryLoadValueCustomQuantity(t *testing.T) {
	registry := NewHDRHistogramRegistry()
	hist := NewHDRHistogram(3, 1, 1000, 3, []float64{50.0}, "microseconds")
	registry.Register("test", hist)
	values := registry.LoadValues("test")
	_, ok := values["test_1_microseconds_mean"]
	assert.True(t, ok)
}

func BenchmarkRegistryRecording(b *testing.B) {
	registry := NewHDRHistogramRegistry()
	hist := NewHDRHistogram(3, 1, 1000, 3, []float64{50.0}, "")
	registry.Register("test", hist)
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
	hist := NewHDRHistogram(3, 1, 1000, 3, []float64{50.0}, "")
	registry.Register("test", hist)
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
