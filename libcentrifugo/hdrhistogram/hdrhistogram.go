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
	quantity    string
}

// NewHDRHistogram creates new HDRHistogram.
func NewHDRHistogram(name string, nHistograms int, minValue, maxValue int64, sigfigs int, quantiles []float64, quantity string) *HDRHistogram {
	h := &HDRHistogram{
		hist:        hdr.NewWindowed(nHistograms, minValue, maxValue, sigfigs),
		name:        name,
		quantiles:   quantiles,
		nHistograms: nHistograms,
		quantity:    quantity,
	}
	return h
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

// LoadValues allows to export map of histogram values - both current and merged.
func (h *HDRHistogram) LoadValues() map[string]int64 {
	values := make(map[string]int64)
	name := h.name
	nHistograms := strconv.Itoa(h.nHistograms)
	currentSnapshot := h.Snapshot()
	mergedSnapshot := h.MergedSnapshot()
	quantity := h.quantity
	if h.quantity == "" {
		quantity = ""
	} else {
		quantity = h.quantity + "_"
	}
	values[name+"_1_count"] = int64(currentSnapshot.TotalCount())
	values[name+"_1_"+quantity+"max"] = int64(currentSnapshot.Max())
	values[name+"_1_"+quantity+"min"] = int64(currentSnapshot.Min())
	values[name+"_1_"+quantity+"mean"] = int64(currentSnapshot.Mean())
	for _, q := range h.quantiles {
		values[name+"_1_"+quantity+strconv.FormatFloat(q, 'f', -1, 64)+"%ile"] = int64(currentSnapshot.ValueAtQuantile(q))
	}
	values[name+"_"+nHistograms+"_count"] = int64(mergedSnapshot.TotalCount())
	values[name+"_"+nHistograms+"_"+quantity+"max"] = int64(mergedSnapshot.Max())
	values[name+"_"+nHistograms+"_"+quantity+"min"] = int64(mergedSnapshot.Min())
	values[name+"_"+nHistograms+"_"+quantity+"mean"] = int64(mergedSnapshot.Mean())
	for _, q := range h.quantiles {
		values[name+"_"+nHistograms+"_"+quantity+strconv.FormatFloat(q, 'f', -1, 64)+"%ile"] = int64(mergedSnapshot.ValueAtQuantile(q))
	}
	return values
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
	r.histograms[h.name] = h
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

func stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

// LoadValues allows to get union of metric values over registered
// histograms - both for current and merged over all buckets.
// If names not provided then return values for all registered histograms.
func (r *HDRHistogramRegistry) LoadValues(names ...string) map[string]int64 {
	values := make(map[string]int64)
	for _, hist := range r.histograms {
		name := hist.name
		if len(names) > 0 && !stringInSlice(name, names) {
			continue
		}
		histValues := hist.LoadValues()
		for k, v := range histValues {
			values[k] = v
		}
	}
	return values
}
