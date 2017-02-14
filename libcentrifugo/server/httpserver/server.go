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

// HTTPServerConfigure is a Configurator func for default Centrifugo http server.
func HTTPServerConfigure(setter config.Setter) error {

	setter.SetDefault("http_prefix", "")
	setter.SetDefault("web", false)
	setter.SetDefault("web_path", "")
	setter.SetDefault("admin_password", "")
	setter.SetDefault("admin_secret", "")
	setter.SetDefault("sockjs_url", "//cdn.jsdelivr.net/sockjs/1.1/sockjs.min.js")
	setter.SetDefault("sockjs_heartbeat_delay", 25)
	setter.SetDefault("websocket_compression", false)
	setter.SetDefault("websocket_compression_min_size", 0)
	setter.SetDefault("websocket_compression_level", 1)
	setter.SetDefault("websocket_read_buffer_size", 4096)
	setter.SetDefault("websocket_write_buffer_size", 4096)

	setter.SetDefault("ssl_autocert", false)
	setter.SetDefault("ssl_autocert_host_whitelist", "")
	setter.SetDefault("ssl_autocert_cache_dir", "")
	setter.SetDefault("ssl_autocert_email", "")
	setter.SetDefault("ssl_autocert_force_rsa", false)
	setter.SetDefault("ssl_autocert_server_name", "")

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

// HTTPServer is a default builtin Centrifugo server.
type HTTPServer struct {
	sync.RWMutex
	node       *node.Node
	config     *Config
	shutdown   bool
	shutdownCh chan struct{}
}

// HTTPServerPlugin is a plugin that returns HTTPServer.
func HTTPServerPlugin(n *node.Node, getter config.Getter) (server.Server, error) {
	return New(n, newConfig(getter))
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
