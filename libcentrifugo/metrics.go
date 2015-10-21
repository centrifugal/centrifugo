package libcentrifugo

import (
	"bytes"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"

	"github.com/centrifugal/centrifugo/Godeps/_workspace/src/github.com/rcrowley/go-metrics"
	"github.com/centrifugal/centrifugo/libcentrifugo/logger"
)

type Metrics struct {
	NumMsgPublished   int64 `json:"num_msg_published"`
	NumMsgQueued      int64 `json:"num_msg_queued"`
	NumMsgSent        int64 `json:"num_msg_sent"`
	NumAPIRequests    int64 `json:"num_api_requests"`
	NumClientRequests int64 `json:"num_client_requests"`
	BytesClientIn     int64 `json:"bytes_client_in"`
	BytesClientOut    int64 `json:"bytes_client_out"`
	TimeAPIMean       int64 `json:"time_api_mean"`
	TimeClientMean    int64 `json:"time_client_mean"`
	TimeAPIMax        int64 `json:"time_api_max"`
	TimeClientMax     int64 `json:"time_client_max"`
	MemSys            int64 `json:"memory_sys"`
	CPU               int64 `json:"cpu_usage"`
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

func NewMetricsRegistry() *metricsRegistry {
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

func cpuUsage() int64 {
	cmd := exec.Command("ps", "aux")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		logger.DEBUG.Println(err)
		return 0
	}
	currentPID := os.Getpid()
	for {
		line, err := out.ReadString('\n')
		if err != nil {
			logger.DEBUG.Println(err)
			return 0
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
			logger.DEBUG.Println(err)
			return 0
		}
		return int64(cpu)
	}
	logger.DEBUG.Println("no cpu info found")
	return 0
}
