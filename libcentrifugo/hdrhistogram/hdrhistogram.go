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

func NewHDRHistogram(name string, numBuckets int, minValue, maxValue int64, sigfigs int, quantiles []float64) *HDRHistogram {
	h := &HDRHistogram{
		hist:       hdr.NewWindowed(numBuckets, minValue, maxValue, sigfigs),
		name:       name,
		quantiles:  quantiles,
		numBuckets: numBuckets,
	}
	return h
}

func (h *HDRHistogram) Name() string {
	return h.name
}

func (h *HDRHistogram) NumBuckets() int {
	return h.numBuckets
}

func (h *HDRHistogram) RecordValue(value int64) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.hist.Current.RecordValue(value)
}

func (h *HDRHistogram) RecordMicroseconds(value time.Duration) error {
	return h.RecordValue(int64(value) / 1000)
}

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

func NewHDRHistogramRegistry() *HDRHistogramRegistry {
	return &HDRHistogramRegistry{
		histograms: make(map[string]*HDRHistogram),
	}
}

func (r *HDRHistogramRegistry) Register(h *HDRHistogram) {
	r.histograms[h.Name()] = h
}

func (r *HDRHistogramRegistry) RecordValue(name string, value int64) error {
	return r.histograms[name].hist.Current.RecordValue(value)
}

func (r *HDRHistogramRegistry) RecordMicroseconds(name string, value time.Duration) error {
	return r.RecordValue(name, int64(value)/1000)
}

func (r *HDRHistogramRegistry) Rotate() {
	for _, hist := range r.histograms {
		hist.hist.Rotate()
	}
}

func (r *HDRHistogramRegistry) LoadLatencies() map[string]int64 {
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
