package libcentrifugo

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"

	"github.com/centrifugal/centrifugo/Godeps/_workspace/src/github.com/rcrowley/go-metrics"
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

	// TimeAPIMean shows mean response time in nanoseconds to API requests.
	TimeAPIMean int64 `json:"time_api_mean"`

	// TimeClientMean shows mean response time in nanoseconds to client requests.
	TimeClientMean int64 `json:"time_client_mean"`

	// TimeAPIMax shows maximum response time to API request.
	TimeAPIMax int64 `json:"time_api_max"`

	// TimeClientMax shows maximum response time to client request.
	TimeClientMax int64 `json:"time_client_max"`

	// MemSys shows system memory usage in bytes.
	MemSys int64 `json:"memory_sys"`

	// CPU shows cpu usage in percents.
	CPU int64 `json:"cpu_usage"`
}

type metricsRegistry struct {
	sync.RWMutex
	numMsgPublished   metrics.Counter
	numMsgQueued      metrics.Counter
	numMsgSent        metrics.Counter
	numAPIRequests    metrics.Counter
	numClientRequests metrics.Counter
	bytesClientIn     metrics.Counter
	bytesClientOut    metrics.Counter
	timeAPI           metrics.Timer
	timeClient        metrics.Timer
	metrics           *Metrics
}

func newMetricsRegistry() *metricsRegistry {
	return &metricsRegistry{
		numMsgPublished:   metrics.NewCounter(),
		numMsgQueued:      metrics.NewCounter(),
		numMsgSent:        metrics.NewCounter(),
		numAPIRequests:    metrics.NewCounter(),
		numClientRequests: metrics.NewCounter(),
		bytesClientIn:     metrics.NewCounter(),
		bytesClientOut:    metrics.NewCounter(),
		timeAPI:           metrics.NewCustomTimer(metrics.NewHistogram(metrics.NewExpDecaySample(1028, 2)), metrics.NewMeter()),
		timeClient:        metrics.NewCustomTimer(metrics.NewHistogram(metrics.NewExpDecaySample(1028, 2)), metrics.NewMeter()),
		metrics:           &Metrics{},
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
		ft := make([]string, 0)
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
