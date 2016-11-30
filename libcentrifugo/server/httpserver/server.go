package httpserver

import (
	"sync"

	"github.com/centrifugal/centrifugo/libcentrifugo/config"
	"github.com/centrifugal/centrifugo/libcentrifugo/metrics"
	"github.com/centrifugal/centrifugo/libcentrifugo/node"
	"github.com/centrifugal/centrifugo/libcentrifugo/plugin"
	"github.com/centrifugal/centrifugo/libcentrifugo/server"
)

func init() {
	plugin.RegisterServer("http", HTTPServerPlugin)
	plugin.RegisterConfigurator("http", HTTPServerConfigure)

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

func HTTPServerConfigure(setter config.Setter) error {

	setter.SetDefault("http_prefix", "")
	setter.SetDefault("web", false)
	setter.SetDefault("web_path", "")
	setter.SetDefault("admin_password", "")
	setter.SetDefault("admin_secret", "")
	setter.SetDefault("sockjs_url", "//cdn.jsdelivr.net/sockjs/1.1/sockjs.min.js")
	setter.SetDefault("sockjs_heartbeat_delay", 0)
	setter.SetDefault("websocket_compression", false)
	setter.SetDefault("websocket_read_buffer_size", 4096)
	setter.SetDefault("websocket_write_buffer_size", 4096)

	setter.BoolFlag("web", "w", false, "serve admin web interface application (warning: automatically enables admin socket)")
	setter.StringFlag("web_path", "", "", "optional path to custom web interface application")
	setter.BoolFlag("ssl", "", false, "accept SSL connections. This requires an X509 certificate and a key file")
	setter.StringFlag("ssl_cert", "", "", "path to an X509 certificate file")
	setter.StringFlag("ssl_key", "", "", "path to an X509 certificate key")
	setter.StringFlag("address", "a", "", "address to listen on")
	setter.StringFlag("port", "p", "8000", "port to bind HTTP server to")
	setter.StringFlag("api_port", "", "", "port to bind api endpoints to (optional)")
	setter.StringFlag("admin_port", "", "", "port to bind admin endpoints to (optional)")

	bindFlags := []string{
		"port", "api_port", "admin_port", "address", "web", "web_path",
		"insecure_web", "ssl", "ssl_cert", "ssl_key",
	}

	for _, flag := range bindFlags {
		setter.BindFlag(flag, flag)
	}

	bindEnvs := []string{
		"web",
	}

	for _, env := range bindEnvs {
		setter.BindEnv(env)
	}

	return nil
}

type HTTPServer struct {
	sync.RWMutex
	node       *node.Node
	config     *Config
	shutdown   bool
	shutdownCh chan struct{}
}

func HTTPServerPlugin(n *node.Node, getter config.Getter) (server.Server, error) {
	return NewHTTPServer(n, newConfig(getter))
}

func NewHTTPServer(n *node.Node, config *Config) (server.Server, error) {
	return &HTTPServer{
		node:       n,
		config:     config,
		shutdownCh: make(chan struct{}),
	}, nil
}

func (s *HTTPServer) Run() error {
	return s.runHTTPServer()
}

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
