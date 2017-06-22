package server

import (
	"sync"

	"github.com/centrifugal/centrifugo/libcentrifugo/metrics"
	"github.com/centrifugal/centrifugo/libcentrifugo/node"
	"github.com/centrifugal/centrifugo/libcentrifugo/plugin"
)

func init() {
	plugin.Metrics.RegisterCounter("http_api_num_requests", metrics.NewCounter())
	plugin.Metrics.RegisterCounter("http_raw_ws_num_requests", metrics.NewCounter())
	plugin.Metrics.RegisterCounter("http_sockjs_num_requests", metrics.NewCounter())

	quantiles := []float64{50, 90, 99, 99.99}
	var minValue int64 = 1        // record latencies in microseconds, min resolution 1mks.
	var maxValue int64 = 60000000 // record latencies in microseconds, max resolution 60s.
	numBuckets := 15              // histograms will be rotated every time we updating snapshot.
	sigfigs := 3
	plugin.Metrics.RegisterHDRHistogram("http_api", metrics.NewHDRHistogram(numBuckets, minValue, maxValue, sigfigs, quantiles, "microseconds"))
}

// HTTPServer is a default builtin Centrifugo server.
type HTTPServer struct {
	sync.RWMutex
	node       *node.Node
	config     *Config
	shutdown   bool
	shutdownCh chan struct{}
}

// New initializes HTTPServer.
func New(n *node.Node, config *Config) (*HTTPServer, error) {
	return &HTTPServer{
		node:       n,
		config:     config,
		shutdownCh: make(chan struct{}),
	}, nil
}

// Run runs HTTPServer.
func (s *HTTPServer) Run() error {
	return s.runHTTPServer()
}

// Shutdown shuts down server.
func (s *HTTPServer) Shutdown() error {
	s.Lock()
	defer s.Unlock()
	if s.shutdown {
		return nil
	}
	s.shutdown = true
	close(s.shutdownCh)
	return nil
}
