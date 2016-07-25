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
	mu          sync.Mutex
	hist        *hdr.WindowedHistogram
	name        string
	nHistograms int
	quantiles   []float64
}

// NewHDRHistogram creates new HDRHistogram.
func NewHDRHistogram(name string, nHistograms int, minValue, maxValue int64, sigfigs int, quantiles []float64) *HDRHistogram {
	h := &HDRHistogram{
		hist:        hdr.NewWindowed(nHistograms, minValue, maxValue, sigfigs),
		name:        name,
		quantiles:   quantiles,
		nHistograms: nHistograms,
	}
	return h
}

// Name returns HDRHistogram name. Can be useful to distinguishing between histograms
// and for exported metric names.
func (h *HDRHistogram) Name() string {
	return h.name
}

// NumHistograms returns amount of buckets(windows) this histogram initialized with.
func (h *HDRHistogram) NumHistograms() int {
	return h.nHistograms
}

// RecordValue is wrapper over github.com/codahale/hdrhistogram.WindowedHistogram.RecordValue
// with extra mutex protection to allow concurrent writes.
func (h *HDRHistogram) RecordValue(value int64) error {
	h.mu.Lock()
	err := h.hist.Current.RecordValue(value)
	h.mu.Unlock()
	return err
}

// RecordMicroseconds allows to record time.Duration value as microseconds.
func (h *HDRHistogram) RecordMicroseconds(value time.Duration) error {
	return h.RecordValue(int64(value) / 1000)
}

// Snapshot allows to get snapshot of current histogram in a thread-safe way.
func (h *HDRHistogram) Snapshot() *hdr.Histogram {
	h.mu.Lock()
	defer h.mu.Unlock()
	return hdr.Import(h.hist.Current.Export())
}

// MergedSnapshot allows to get snapshot of merged histogram in a thread-safe way.
func (h *HDRHistogram) MergedSnapshot() *hdr.Histogram {
	h.mu.Lock()
	defer h.mu.Unlock()
	return hdr.Import(h.hist.Merge().Export())
}

// Rotate histogram.
func (h *HDRHistogram) Rotate() {
	h.mu.Lock()
	h.hist.Rotate()
	h.mu.Unlock()
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
		hist.Rotate()
	}
}

// LoadValues allows to get union of metric values over all registered
// histograms - both for current and merged over all buckets.
func (r *HDRHistogramRegistry) LoadValues() map[string]int64 {
	values := make(map[string]int64)
	for _, hist := range r.histograms {
		name := hist.Name()
		nHistograms := strconv.Itoa(hist.NumHistograms())
		currentSnapshot := hist.Snapshot()
		mergedSnapshot := hist.MergedSnapshot()
		values[name+"_1_count"] = int64(currentSnapshot.TotalCount())
		values[name+"_1_max"] = int64(currentSnapshot.Max())
		values[name+"_1_min"] = int64(currentSnapshot.Min())
		values[name+"_1_mean"] = int64(currentSnapshot.Mean())
		for _, q := range hist.quantiles {
			values[name+"_1_"+strconv.FormatFloat(q, 'f', -1, 64)+"%ile"] = int64(currentSnapshot.ValueAtQuantile(q))
		}
		values[name+"_"+nHistograms+"_count"] = int64(mergedSnapshot.TotalCount())
		values[name+"_"+nHistograms+"_max"] = int64(mergedSnapshot.Max())
		values[name+"_"+nHistograms+"_min"] = int64(mergedSnapshot.Min())
		values[name+"_"+nHistograms+"_mean"] = int64(mergedSnapshot.Mean())
		for _, q := range hist.quantiles {
			values[name+"_"+nHistograms+"_"+strconv.FormatFloat(q, 'f', -1, 64)+"%ile"] = int64(mergedSnapshot.ValueAtQuantile(q))
		}
	}
	return values
}
