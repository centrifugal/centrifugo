package libcentrifugo

//go:generate stringer -type=MetricID

import (
	"bytes"
	"encoding/json"
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
	NumMsgPublished metricCounter `json:"num_msg_published"`

	// NumMsgQueued is how many messages were put into client queues.
	NumMsgQueued metricCounter `json:"num_msg_queued"`

	// NumMsgSent is how many messages were actually sent into client connections.
	NumMsgSent metricCounter `json:"num_msg_sent"`

	// NumAPIRequests shows amount of requests to server API.
	NumAPIRequests metricCounter `json:"num_api_requests"`

	// NumClientRequests shows amount of requests to client API.
	NumClientRequests metricCounter `json:"num_client_requests"`

	// BytesClientIn shows amount of data in bytes coming into client API.
	BytesClientIn metricCounter `json:"bytes_client_in"`

	// BytesClientOut shows amount of data in bytes coming out if client API.
	BytesClientOut metricCounter `json:"bytes_client_out"`

	// TimeAPIMean shows mean response time in nanoseconds to API requests. DEPRECATED will return 0
	TimeAPIMean int64 `json:"time_api_mean"`

	// TimeClientMean shows mean response time in nanoseconds to client requests. DEPRECATED will return 0
	TimeClientMean int64 `json:"time_client_mean"`

	// TimeAPIMax shows maximum response time to API request. DEPRECATED will return 0
	TimeAPIMax int64 `json:"time_api_max"`

	// TimeClientMax shows maximum response time to client request. DEPRECATED will return 0
	TimeClientMax int64 `json:"time_client_max"`

	// MemSys shows system memory usage in bytes.
	MemSys int64 `json:"memory_sys"`

	// CPU shows cpu usage in percents.
	CPU int64 `json:"cpu_usage"`

	// mu protects from multiple processes updating snapshot values at once
	// but raw counters may still increment atomically while held so it's not a strict
	// point-in-time snapshot of all values.
	mu sync.Mutex
}

type metricCounter struct {
	value             int64
	lastIntervalValue int64
	lastIntervalDelta int64
	// marshalRaw indicates that JSON Marshalling should dump the raw counter rather than
	// the last interval delta. Note that if this is true then this struct is a read-only
	// snapshot and doesn't need to use atomic.LoadInt64 to read the raw values.
	marshalRaw bool
}

// cloneRaw creates a read-only copy of a counter by loading it's raw value
// and copying it's last interval values.
// It is only safe to do while holding counter's parent Metrics mutex to ensure that last
// interval values are not being updated.
// The cloned counter is marked to be marshaled to JSON with the raw value rather than delta.
// Any attempt to mutate the cloned counter will panic.
func (c *metricCounter) cloneRaw() *metricCounter {
	return &metricCounter{
		value:             c.LoadRaw(),
		lastIntervalValue: c.lastIntervalValue,
		lastIntervalDelta: c.lastIntervalDelta,
		marshalRaw:        true,
	}
}

// MarshalJSON converts a counter struct into a single JSON int representing
// the last interval delta since that is what we report in general.
// See marshalRaw definition for more detail
func (c metricCounter) MarshalJSON() ([]byte, error) {
	if c.marshalRaw {
		// No need for atomic load - marshalRaw should only be set
		// on a read-only copy anyway
		return json.Marshal(c.value)
	}
	return json.Marshal(c.lastIntervalDelta)
}

func (c metricCounter) UnmarshalJSON(bs []byte) error {
	// We shouldn't need to Unmarshal raw counters ever
	return json.Unmarshal(bs, &c.lastIntervalDelta)
}

func (c *metricCounter) LoadRaw() int64 {
	return atomic.LoadInt64(&c.value)
}

func (c *metricCounter) LastIn() int64 {
	return atomic.LoadInt64(&c.value)
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
	if c.marshalRaw {
		panic("Attempt to modify a counter that has been cloned as a read-only raw value copy")
	}
	return atomic.AddInt64(&c.value, n)
}

// updateDelta updates the delta value for last interval based on current value and previous value.
// It is not threadsafe and should only be called by Metrics.UpdateSnapshot which is serialised by Mutex.
func (c *metricCounter) updateDelta() {
	now := atomic.LoadInt64(&c.value)
	c.lastIntervalDelta = now - c.lastIntervalValue
	c.lastIntervalValue = now
}

func (m *Metrics) UpdateSnapshot() {
	// We update under a lock to ensure that no other process is also updating
	// snapshot nor dumping the values. Other processes CAN still atomically increment raw
	// values while we go though - we don't guarantee counter values are point-in-time consistent
	// with each other
	m.mu.Lock()
	defer m.mu.Unlock()

	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	m.MemSys = int64(mem.Sys)

	cpu, err := cpuUsage()
	if err != nil {
		logger.DEBUG.Println(err)
	}
	m.CPU = cpu

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

// Get RawCounts returns a copy of the current raw counter values.
// The returned value is another instance of Metrics but you should treat it
// as read-only. The only valid operations are to access the raw count values,
// or more likely to marshal it to JSON
func (m *Metrics) GetRawCounts() *Metrics {
	m.mu.Lock()
	defer m.mu.Unlock()

	m2 := Metrics{
		NumMsgPublished:   *m.NumMsgPublished.cloneRaw(),
		NumMsgQueued:      *m.NumMsgQueued.cloneRaw(),
		NumMsgSent:        *m.NumMsgSent.cloneRaw(),
		NumAPIRequests:    *m.NumAPIRequests.cloneRaw(),
		NumClientRequests: *m.NumClientRequests.cloneRaw(),
		BytesClientIn:     *m.BytesClientIn.cloneRaw(),
		BytesClientOut:    *m.BytesClientOut.cloneRaw(),
		MemSys:            m.MemSys,
		CPU:               m.CPU,
	}

	return &m2
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
