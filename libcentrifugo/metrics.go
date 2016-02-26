package libcentrifugo

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/centrifugal/centrifugo/Godeps/_workspace/src/github.com/FZambia/go-logger"
)

// Metrics contains various Centrifugo statistic and metric information aggregated
// once in a configurable interval.
type Metrics struct {
	// NumMsgPublished is how many messages were published into channels.
	NumMsgPublished int64 `json:"num_msg_published"`

	// NumMsgQueued is how many messages were put into client queues.
	NumMsgQueued int64 `json:"num_msg_queued"`

	// NumMsgSent is how many messages were actually sent into client connections.
	NumMsgSent int64 `json:"num_msg_sent"`

	// NumAPIRequests shows amount of requests to server API.
	NumAPIRequests int64 `json:"num_api_requests"`

	// NumClientRequests shows amount of requests to client API.
	NumClientRequests int64 `json:"num_client_requests"`

	// BytesClientIn shows amount of data in bytes coming into client API.
	BytesClientIn int64 `json:"bytes_client_in"`

	// BytesClientOut shows amount of data in bytes coming out if client API.
	BytesClientOut int64 `json:"bytes_client_out"`

	// TimeAPIMean shows mean response time in nanoseconds to API requests. DEPRECATED!
	TimeAPIMean int64 `json:"time_api_mean"`

	// TimeClientMean shows mean response time in nanoseconds to client requests. DEPRECATED!
	TimeClientMean int64 `json:"time_client_mean"`

	// TimeAPIMax shows maximum response time to API request. DEPRECATED!
	TimeAPIMax int64 `json:"time_api_max"`

	// TimeClientMax shows maximum response time to client request. DEPRECATED!
	TimeClientMax int64 `json:"time_client_max"`

	// MemSys shows system memory usage in bytes.
	MemSys int64 `json:"memory_sys"`

	// CPU shows cpu usage in percents.
	CPU int64 `json:"cpu_usage"`
}

// metricsRegistry contains various Centrifugo statistic and metric information aggregated
// once in a configurable interval.
type metricsRegistry struct {
	// mu protects from multiple processes updating snapshot values at once
	// but raw counters may still increment atomically while held so it's not a strict
	// point-in-time snapshot of all values.
	mu sync.Mutex

	NumMsgPublished   metricCounter
	NumMsgQueued      metricCounter
	NumMsgSent        metricCounter
	NumAPIRequests    metricCounter
	NumClientRequests metricCounter
	BytesClientIn     metricCounter
	BytesClientOut    metricCounter
	TimeAPIMean       metricGauge // Deprecated
	TimeClientMean    metricGauge // Deprecated
	TimeAPIMax        metricGauge // Deprecated
	TimeClientMax     metricGauge // Deprecated
	MemSys            metricGauge
	CPU               metricGauge
}

// metricGauge is a wrapper around a single int that allows us to ensure that JSON
// Marshal access it atomically.
type metricGauge struct {
	value int64
}

func (g *metricGauge) Load() int64 {
	return atomic.LoadInt64(&g.value)
}

type metricCounter struct {
	value             int64
	lastIntervalValue int64
	lastIntervalDelta int64
}

func (c *metricCounter) LoadRaw() int64 {
	return atomic.LoadInt64(&c.value)
}

// LastIn allows to get last interval value for counter, note
// that we don't do this atomically because all operations on delta
// happens under mutex in UpdateSnapshot and GetSnapshotMetrics methods.
func (c *metricCounter) LastIn() int64 {
	return c.lastIntervalDelta
}

// Inc is equivalent to Add(name, 1)
func (c *metricCounter) Inc() int64 {
	return c.Add(1)
}

// Add adds the given number to the counter and returns the new value.
// Note that we assume all Register calls occur during init and all
// Add calls happen strictly after init such that no lock is needed to lookup
// the counter in read-only map.
func (c *metricCounter) Add(n int64) int64 {
	return atomic.AddInt64(&c.value, n)
}

// updateDelta updates the delta value for last interval based on current value and previous value.
// It is not threadsafe and should only be called by Metrics.UpdateSnapshot which is serialised by Mutex.
func (c *metricCounter) updateDelta() {
	now := atomic.LoadInt64(&c.value)
	c.lastIntervalDelta = now - c.lastIntervalValue
	c.lastIntervalValue = now
}

func (m *metricsRegistry) UpdateSnapshot() {
	// We update under a lock to ensure that no other process is also updating
	// snapshot nor dumping the values. Other processes CAN still atomically increment raw
	// values while we go though - we don't guarantee counter values are point-in-time consistent
	// with each other
	m.mu.Lock()
	defer m.mu.Unlock()

	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	atomic.StoreInt64(&m.MemSys.value, int64(mem.Sys))

	cpu, err := cpuUsage()
	if err != nil {
		logger.DEBUG.Println(err)
	}
	atomic.StoreInt64(&m.CPU.value, int64(cpu))

	// Would love to not have to list these explicitly but every alternative is slow
	// or hacky (code generation)
	m.NumMsgPublished.updateDelta()
	m.NumMsgQueued.updateDelta()
	m.NumMsgSent.updateDelta()
	m.NumAPIRequests.updateDelta()
	m.NumClientRequests.updateDelta()
	m.BytesClientIn.updateDelta()
	m.BytesClientOut.updateDelta()
}

// GetRawMetrics returns Metrics with raw counter values.
func (m *metricsRegistry) GetRawMetrics() *Metrics {
	m.mu.Lock()
	defer m.mu.Unlock()

	return &Metrics{
		NumMsgPublished:   m.NumMsgPublished.LoadRaw(),
		NumMsgQueued:      m.NumMsgQueued.LoadRaw(),
		NumMsgSent:        m.NumMsgSent.LoadRaw(),
		NumAPIRequests:    m.NumAPIRequests.LoadRaw(),
		NumClientRequests: m.NumClientRequests.LoadRaw(),
		BytesClientIn:     m.BytesClientIn.LoadRaw(),
		BytesClientOut:    m.BytesClientOut.LoadRaw(),
		MemSys:            m.MemSys.Load(),
		CPU:               m.CPU.Load(),
	}
}

// GetSnapshotMetrics returns Metrics with snapshoted over time interval
// counter values.
func (m *metricsRegistry) GetSnapshotMetrics() *Metrics {
	m.mu.Lock()
	defer m.mu.Unlock()

	return &Metrics{
		NumMsgPublished:   m.NumMsgPublished.LastIn(),
		NumMsgQueued:      m.NumMsgQueued.LastIn(),
		NumMsgSent:        m.NumMsgSent.LastIn(),
		NumAPIRequests:    m.NumAPIRequests.LastIn(),
		NumClientRequests: m.NumClientRequests.LastIn(),
		BytesClientIn:     m.BytesClientIn.LastIn(),
		BytesClientOut:    m.BytesClientOut.LastIn(),
		MemSys:            m.MemSys.Load(),
		CPU:               m.CPU.Load(),
	}
}

// cpuUsage is the simplest possible method to extract CPU usage info on most of platforms
// Centrifugo runs. I have not found a more sophisticated cross platform way to extract
// this info without using CGO.
func cpuUsage() (int64, error) {
	cmd := exec.Command("ps", "aux")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return 0, err
	}
	currentPID := os.Getpid()
	for {
		line, err := out.ReadString('\n')
		if err != nil {
			return 0, err
		}
		tokens := strings.Split(line, " ")
		var ft []string
		for _, t := range tokens {
			if t != "" && t != "\t" {
				ft = append(ft, t)
			}
		}
		pid, err := strconv.Atoi(ft[1])
		if err != nil {
			continue
		}
		if pid != currentPID {
			continue
		}
		cpu, err := strconv.ParseFloat(ft[2], 64)
		if err != nil {
			return 0, err
		}
		return int64(cpu), nil
	}
	return 0, errors.New("no cpu info found")
}
