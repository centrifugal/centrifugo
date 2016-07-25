// package hdrhistogram is a small library that wraps github.com/codahale/hdrhistogram.WindowedHistogram
// to make WindowedHistogram synchronized and provides registry structure to simplify using
// multiple histograms.
package hdrhistogram

import (
	"strconv"
	"sync"
	"time"

	hdr "github.com/codahale/hdrhistogram"
)

// HDRHistogram is a synchronized wrapper over github.com/codahale/hdrhistogram.WindowedHistogram
type HDRHistogram struct {
	mu         sync.Mutex
	hist       *hdr.WindowedHistogram
	name       string
	numBuckets int
	quantiles  []float64
}

// NewHDRHistogram creates new HDRHistogram.
func NewHDRHistogram(name string, numBuckets int, minValue, maxValue int64, sigfigs int, quantiles []float64) *HDRHistogram {
	h := &HDRHistogram{
		hist:       hdr.NewWindowed(numBuckets, minValue, maxValue, sigfigs),
		name:       name,
		quantiles:  quantiles,
		numBuckets: numBuckets,
	}
	return h
}

// Name returns HDRHistogram name. Can be useful to distinguishing between histograms
// and for exported metric names.
func (h *HDRHistogram) Name() string {
	return h.name
}

// NumBuckets returns amount of buckets(windows) this histogram initialized with.
func (h *HDRHistogram) NumBuckets() int {
	return h.numBuckets
}

// RecordValue is wrapper over github.com/codahale/hdrhistogram.WindowedHistogram.RecordValue
// with extra mutex protection to allow concurrent writes.
func (h *HDRHistogram) RecordValue(value int64) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.hist.Current.RecordValue(value)
}

// RecordMicroseconds allows to record time.Duration value as microseconds.
func (h *HDRHistogram) RecordMicroseconds(value time.Duration) error {
	return h.RecordValue(int64(value) / 1000)
}

// Rotate histogram.
func (h *HDRHistogram) Rotate() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.hist.Rotate()
}

// HDRHistogramRegistry is a wrapper to deal with several HDRHistogram instances.
// After it has been initialized you only can write values into histograms and extract
// data - do not register histograms dinamically after initial setup.
type HDRHistogramRegistry struct {
	histograms map[string]*HDRHistogram
}

// NewHDRHistogramRegistry creates new HDRHistogramRegistry.
func NewHDRHistogramRegistry() *HDRHistogramRegistry {
	return &HDRHistogramRegistry{
		histograms: make(map[string]*HDRHistogram),
	}
}

// Register allows to register HDRHistogram in registry so you can use its name
// later to record values over registry instance using RecordValue and RecordMicroseconds
// methods.
func (r *HDRHistogramRegistry) Register(h *HDRHistogram) {
	r.histograms[h.Name()] = h
}

// RecordValue into histogram with provided name. Panics if name not registered.
func (r *HDRHistogramRegistry) RecordValue(name string, value int64) error {
	return r.histograms[name].RecordValue(value)
}

// RecordMicroseconds into histogram with provided name. Panics if name not registered.
func (r *HDRHistogramRegistry) RecordMicroseconds(name string, value time.Duration) error {
	return r.RecordValue(name, int64(value)/1000)
}

// Rotate all registered histograms.
func (r *HDRHistogramRegistry) Rotate() {
	for _, hist := range r.histograms {
		hist.hist.Rotate()
	}
}

// LoadValues allows to get union of metric values over all registered
// histograms - both for current and merged over all buckets.
func (r *HDRHistogramRegistry) LoadValues() map[string]int64 {
	latencies := make(map[string]int64)
	for _, hist := range r.histograms {
		name := hist.Name()
		numBuckets := strconv.Itoa(hist.NumBuckets())
		latencies[name+"_1_count"] = int64(hist.hist.Current.TotalCount())
		latencies[name+"_1_max"] = int64(hist.hist.Current.Max())
		latencies[name+"_1_min"] = int64(hist.hist.Current.Min())
		latencies[name+"_1_mean"] = int64(hist.hist.Current.Mean())
		for _, q := range hist.quantiles {
			latencies[name+"_1_"+strconv.FormatFloat(q, 'f', -1, 64)+"%ile"] = int64(hist.hist.Current.ValueAtQuantile(q))
		}
		merged := hist.hist.Merge()
		latencies[name+"_"+numBuckets+"_count"] = int64(merged.TotalCount())
		latencies[name+"_"+numBuckets+"_max"] = int64(merged.Max())
		latencies[name+"_"+numBuckets+"_min"] = int64(merged.Min())
		latencies[name+"_"+numBuckets+"_mean"] = int64(merged.Mean())
		for _, q := range hist.quantiles {
			latencies[name+"_"+numBuckets+"_"+strconv.FormatFloat(q, 'f', -1, 64)+"%ile"] = int64(merged.ValueAtQuantile(q))
		}
	}
	return latencies
}
