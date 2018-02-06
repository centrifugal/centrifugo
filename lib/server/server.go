package server

import (
	"net/http"
	"sync"

	"github.com/centrifugal/centrifugo/lib/api"
	"github.com/centrifugal/centrifugo/lib/metrics"
	"github.com/centrifugal/centrifugo/lib/node"
)

func init() {
	metrics.DefaultRegistry.RegisterCounter("api.http.num_requests", metrics.NewCounter())
	metrics.DefaultRegistry.RegisterCounter("transport.websocket.num_requests", metrics.NewCounter())
	metrics.DefaultRegistry.RegisterCounter("transport.sockjs.num_requests", metrics.NewCounter())

	quantiles := []float64{50, 90, 99, 99.99}
	metrics.DefaultRegistry.RegisterHDRHistogram("http_api", metrics.NewHDRHistogram(15, 1, 60000000, 3, quantiles, "microseconds"))
}

// HTTPServer is a default builtin Centrifugo server.
type HTTPServer struct {
	sync.RWMutex
	node       *node.Node
	mux        *http.ServeMux
	config     *Config
	shutdown   bool
	shutdownCh chan struct{}
	api        *api.Handler
}

// New initializes HTTPServer.
func New(n *node.Node, config *Config) (*HTTPServer, error) {
	return &HTTPServer{
		node:       n,
		config:     config,
		shutdownCh: make(chan struct{}),
		api:        api.NewHandler(n),
	}, nil
}

func (s *HTTPServer) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(rw, r)
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
