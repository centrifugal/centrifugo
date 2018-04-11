package centrifugo

import (
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/FZambia/viper-lite"
	"github.com/centrifugal/centrifugo/libcentrifugo/config"
	"github.com/centrifugal/centrifugo/libcentrifugo/engine"
	"github.com/centrifugal/centrifugo/libcentrifugo/logger"
	"github.com/centrifugal/centrifugo/libcentrifugo/node"
	"github.com/centrifugal/centrifugo/libcentrifugo/plugin"
	"github.com/centrifugal/centrifugo/libcentrifugo/server"
	"github.com/igm/sockjs-go/sockjs"
	"github.com/rakyll/statik/fs"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/acme/autocert"
)

func writePidFile(pidFile string) error {
	if pidFile == "" {
		return nil
	}
	pid := []byte(strconv.Itoa(os.Getpid()) + "\n")
	return ioutil.WriteFile(pidFile, pid, 0644)
}

func setupLogging() {
	log.SetFlags(0)
	log.SetOutput(logger.INFO)
	logLevel, ok := logger.LevelMatches[strings.ToUpper(viper.GetString("log_level"))]
	if !ok {
		logLevel = logger.LevelInfo
	}
	logger.SetLogThreshold(logLevel)
	logger.SetStdoutThreshold(logLevel)

	if viper.IsSet("log_file") && viper.GetString("log_file") != "" {
		logger.SetLogFile(viper.GetString("log_file"))
		// do not log into stdout when log file provided
		logger.SetStdoutThreshold(logger.LevelNone)
	}
}

func handleSignals(n *node.Node, s *server.HTTPServer) {
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGHUP, syscall.SIGINT, os.Interrupt, syscall.SIGTERM)
	for {
		sig := <-sigc
		logger.INFO.Println("Signal received:", sig)
		switch sig {
		case syscall.SIGHUP:
			// reload application configuration on SIGHUP
			logger.INFO.Println("Reloading configuration")
			err := viper.ReadInConfig()
			if err != nil {
				switch err.(type) {
				case viper.ConfigParseError:
					logger.CRITICAL.Printf("Error parsing configuration: %s\n", err)
					continue
				default:
					logger.CRITICAL.Println("No config file found")
					continue
				}
			}
			setupLogging()
			cfg := newNodeConfig(viper.GetViper())
			if err := n.Reload(cfg); err != nil {
				logger.CRITICAL.Println(err)
				continue
			}
			logger.INFO.Println("Configuration successfully reloaded")
		case syscall.SIGINT, os.Interrupt, syscall.SIGTERM:
			logger.INFO.Println("Shutting down")
			pidFile := viper.GetString("pid_file")
			go time.AfterFunc(10*time.Second, func() {
				if pidFile != "" {
					os.Remove(pidFile)
				}
				os.Exit(1)
			})
			s.Shutdown()
			n.Shutdown()
			if pidFile != "" {
				os.Remove(pidFile)
			}
			os.Exit(0)
		}
	}
}

// ConfigureNode builds node related configuration.
func ConfigureNode(setter config.Setter) error {
	defaults := map[string]interface{}{
		"debug":                          false,
		"name":                           "",
		"admin":                          false,
		"admin_password":                 "",
		"admin_secret":                   "",
		"secret":                         "",
		"connection_lifetime":            0,
		"watch":                          false,
		"publish":                        false,
		"anonymous":                      false,
		"presence":                       false,
		"history_size":                   0,
		"history_lifetime":               0,
		"recover":                        false,
		"history_drop_inactive":          false,
		"namespaces":                     "",
		"max_channel_length":             255,
		"user_connection_limit":          0,
		"node_ping_interval":             3,
		"ping_interval":                  25,
		"node_metrics_interval":          60,
		"stale_connection_close_delay":   25,
		"expired_connection_close_delay": 25,
		"client_message_write_timeout":   0,
		"client_channel_limit":           128,
		"client_request_max_size":        65536,    // 64KB
		"client_queue_max_size":          10485760, // 10MB
		"client_queue_initial_capacity":  2,
		"presence_ping_interval":         25,
		"presence_expire_interval":       60,
		"private_channel_prefix":         "$",
		"namespace_channel_boundary":     ":",
		"user_channel_boundary":          "#",
		"user_channel_separator":         ",",
		"client_channel_boundary":        "&",
	}

	for k, v := range defaults {
		setter.SetDefault(k, v)
	}

	setter.StringFlag("name", "n", "", "unique node name")
	setter.BoolFlag("debug", "d", false, "enable debug mode")
	setter.BoolFlag("admin", "", false, "enable admin socket")
	setter.BoolFlag("insecure", "", false, "start in insecure client mode")
	setter.BoolFlag("insecure_api", "", false, "use insecure API mode")
	setter.BoolFlag("insecure_admin", "", false, "use insecure admin mode – no auth required for admin socket")

	bindEnvs := []string{
		"debug", "insecure", "insecure_api", "admin", "admin_password", "admin_secret",
		"insecure_admin", "secret", "connection_lifetime", "watch", "publish", "anonymous", "join_leave",
		"presence", "recover", "history_size", "history_lifetime", "history_drop_inactive",
	}

	for _, env := range bindEnvs {
		setter.BindEnv(env)
	}

	bindPFlags := []string{
		"debug", "name", "admin", "insecure", "insecure_admin", "insecure_api",
	}
	for _, flag := range bindPFlags {
		setter.BindFlag(flag, flag)
	}
	return nil
}

// ConfigureServer builds server related configuration.
func ConfigureServer(setter config.Setter) error {

	defaults := map[string]interface{}{
		"http_prefix":                    "",
		"web":                            false,
		"web_path":                       "",
		"admin_password":                 "",
		"admin_secret":                   "",
		"sockjs_url":                     "//cdn.jsdelivr.net/sockjs/1.1/sockjs.min.js",
		"sockjs_heartbeat_delay":         25,
		"websocket_compression":          false,
		"websocket_compression_min_size": 0,
		"websocket_compression_level":    1,
		"websocket_read_buffer_size":     0,
		"websocket_write_buffer_size":    0,
		"ssl_autocert":                   false,
		"ssl_autocert_host_whitelist":    "",
		"ssl_autocert_cache_dir":         "",
		"ssl_autocert_email":             "",
		"ssl_autocert_force_rsa":         false,
		"ssl_autocert_server_name":       "",
		"ssl_autocert_http":              false,
		"ssl_autocert_http_addr":         ":80",
	}

	for k, v := range defaults {
		setter.SetDefault(k, v)
	}

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

func runServer(n *node.Node, s *server.HTTPServer) error {

	debug := viper.GetBool("debug")
	adminEnabled := viper.GetBool("admin")
	webEnabled := viper.GetBool("web")
	webPath := viper.GetString("web_path")
	httpAddress := viper.GetString("address")
	httpClientPort := viper.GetString("port")
	httpAdminPort := viper.GetString("admin_port")
	httpAPIPort := viper.GetString("api_port")
	httpPrefix := viper.GetString("http_prefix")
	sockjsURL := viper.GetString("sockjs_url")
	sockjsHeartbeatDelay := viper.GetInt("sockjs_heartbeat_delay")
	sslEnabled := viper.GetBool("ssl")
	sslCert := viper.GetString("ssl_cert")
	sslKey := viper.GetString("ssl_key")
	sslAutocertEnabled := viper.GetBool("ssl_autocert")
	autocertHostWhitelist := viper.GetString("ssl_autocert_host_whitelist")
	var sslAutocertHostWhitelist []string
	if autocertHostWhitelist != "" {
		sslAutocertHostWhitelist = strings.Split(autocertHostWhitelist, ",")
	} else {
		sslAutocertHostWhitelist = nil
	}
	sslAutocertCacheDir := viper.GetString("ssl_autocert_cache_dir")
	sslAutocertEmail := viper.GetString("ssl_autocert_email")
	sslAutocertForceRSA := viper.GetBool("ssl_autocert_force_rsa")
	sslAutocertServerName := viper.GetString("ssl_autocert_server_name")
	sslAutocertHTTP := viper.GetBool("ssl_autocert_http")
	sslAutocertHTTPAddr := viper.GetString("ssl_autocert_http_addr")
	websocketReadBufferSize := viper.GetInt("websocket_read_buffer_size")
	websocketWriteBufferSize := viper.GetInt("websocket_write_buffer_size")

	sockjs.WebSocketReadBufSize = websocketReadBufferSize
	sockjs.WebSocketWriteBufSize = websocketWriteBufferSize

	sockjsOpts := sockjs.DefaultOptions

	// Override sockjs url. It's important to use the same SockJS library version
	// on client and server sides when using iframe based SockJS transports, otherwise
	// SockJS will raise error about version mismatch.
	if sockjsURL != "" {
		logger.INFO.Println("SockJS url:", sockjsURL)
		sockjsOpts.SockJSURL = sockjsURL
	}

	sockjsOpts.HeartbeatDelay = time.Duration(sockjsHeartbeatDelay) * time.Second

	var webFS http.FileSystem
	if webEnabled {
		webFS, _ = fs.New()
	}

	if webEnabled {
		adminEnabled = true
	}

	if httpAPIPort == "" {
		httpAPIPort = httpClientPort
	}
	if httpAdminPort == "" {
		httpAdminPort = httpClientPort
	}

	// portToHandlerFlags contains mapping between ports and handler flags
	// to serve on this port.
	portToHandlerFlags := map[string]server.HandlerFlag{}

	var portFlags server.HandlerFlag

	portFlags = portToHandlerFlags[httpClientPort]
	portFlags |= server.HandlerRawWS | server.HandlerSockJS
	portToHandlerFlags[httpClientPort] = portFlags

	portFlags = portToHandlerFlags[httpAPIPort]
	portFlags |= server.HandlerAPI
	portToHandlerFlags[httpAPIPort] = portFlags

	portFlags = portToHandlerFlags[httpAdminPort]
	if adminEnabled || webEnabled {
		portFlags |= server.HandlerAdmin
	}
	if webEnabled {
		portFlags |= server.HandlerWeb
	}
	if debug {
		portFlags |= server.HandlerDebug
	}
	portToHandlerFlags[httpAdminPort] = portFlags

	var wg sync.WaitGroup
	// Iterate over port to flags mapping and start HTTP servers
	// on separate ports serving handlers specified in flags.
	for handlerPort, handlerFlags := range portToHandlerFlags {
		muxOpts := server.MuxOptions{
			Prefix:        httpPrefix,
			WebPath:       webPath,
			WebFS:         webFS,
			HandlerFlags:  handlerFlags,
			SockjsOptions: sockjsOpts,
		}
		mux := server.ServeMux(s, muxOpts)

		addr := net.JoinHostPort(httpAddress, handlerPort)

		logger.INFO.Printf("Start serving %s endpoints on %s\n", handlerFlags, addr)

		wg.Add(1)
		go func() {
			defer wg.Done()

			if sslAutocertEnabled {
				certManager := autocert.Manager{
					Prompt:   autocert.AcceptTOS,
					ForceRSA: sslAutocertForceRSA,
					Email:    sslAutocertEmail,
				}
				if sslAutocertHostWhitelist != nil {
					certManager.HostPolicy = autocert.HostWhitelist(sslAutocertHostWhitelist...)
				}
				if sslAutocertCacheDir != "" {
					certManager.Cache = autocert.DirCache(sslAutocertCacheDir)
				}
				server := &http.Server{
					Addr:    addr,
					Handler: mux,
					TLSConfig: &tls.Config{
						GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
							// See https://github.com/centrifugal/centrifugo/issues/144#issuecomment-279393819
							if sslAutocertServerName != "" && hello.ServerName == "" {
								hello.ServerName = sslAutocertServerName
							}
							return certManager.GetCertificate(hello)
						},
					},
				}

				if sslAutocertHTTP {
					acmeHTTPserver := &http.Server{
						Handler: certManager.HTTPHandler(nil),
						Addr:    sslAutocertHTTPAddr,
					}
					go func() {
						logger.INFO.Printf("Serving ACME http_01 challenge on %s", sslAutocertHTTPAddr)
						if err := acmeHTTPserver.ListenAndServe(); err != nil {
							logger.FATAL.Fatalf("Can't create server to serve acme http challenges: %v", err)
						}
					}()
				}

				if err := server.ListenAndServeTLS("", ""); err != nil {
					logger.FATAL.Fatalf("ListenAndServe: %v", err)
				}
			} else if sslEnabled {
				// Autocert disabled - just try to use provided SSL cert and key files.
				server := &http.Server{Addr: addr, Handler: mux}
				if err := server.ListenAndServeTLS(sslCert, sslKey); err != nil {
					logger.FATAL.Fatalf("ListenAndServe: %v", err)
				}
			} else {
				if err := http.ListenAndServe(addr, mux); err != nil {
					logger.FATAL.Fatalf("ListenAndServe: %v", err)
				}
			}
		}()
	}
	wg.Wait()

	return nil
}

// Main starts Centrifugo server.
func Main(version string) {

	var configFile string

	var engn string
	var logLevel string
	var logFile string
	var pidFile string

	var rootCmd = &cobra.Command{
		Use:   "",
		Short: "Centrifugo",
		Long:  "Centrifugo. Real-time messaging (Websockets or SockJS) server in Go.",
		Run: func(cmd *cobra.Command, args []string) {

			defaults := map[string]interface{}{
				"gomaxprocs": 0,
				"engine":     "memory",
			}

			for k, v := range defaults {
				viper.SetDefault(k, v)
			}

			bindEnvs := []string{
				"engine",
			}
			for _, env := range bindEnvs {
				viper.BindEnv(env)
			}

			bindPFlags := []string{
				"engine", "log_level", "log_file", "pid_file",
			}
			for _, flag := range bindPFlags {
				viper.BindPFlag(flag, cmd.Flags().Lookup(flag))
			}

			viper.SetConfigFile(configFile)

			absConfPath, err := filepath.Abs(configFile)
			if err != nil {
				logger.FATAL.Fatalf("Error retreiving config file absolute path: %v", err)
			}

			err = viper.ReadInConfig()
			configFound := true
			if err != nil {
				switch err.(type) {
				case viper.ConfigParseError:
					logger.FATAL.Fatalf("Error parsing configuration: %s\n", err)
				default:
					configFound = false
				}
			}

			setupLogging()
			err = writePidFile(viper.GetString("pid_file"))
			if err != nil {
				logger.FATAL.Fatalf("Error writing PID: %v", err)
			}

			if !configFound {
				logger.WARN.Println("No config file found")
			}

			if os.Getenv("GOMAXPROCS") == "" {
				if viper.IsSet("gomaxprocs") && viper.GetInt("gomaxprocs") > 0 {
					runtime.GOMAXPROCS(viper.GetInt("gomaxprocs"))
				} else {
					runtime.GOMAXPROCS(runtime.NumCPU())
				}
			}

			c := newNodeConfig(viper.GetViper())
			err = c.Validate()
			if err != nil {
				logger.FATAL.Fatalf("Error validating config: %v", err)
			}

			node.VERSION = version
			nod := node.New(c)

			engineName := viper.GetString("engine")
			engineFactory, ok := plugin.EngineFactories[engineName]
			if !ok {
				logger.FATAL.Fatalf("Unknown engine: %s", engineName)
			}
			var e engine.Engine
			e, err = engineFactory(nod, viper.GetViper())
			if err != nil {
				logger.FATAL.Fatalf("Error creating engine: %v", err)
			}

			if err = nod.Run(e); err != nil {
				logger.FATAL.Fatalf("Error running node: %v", err)
			}

			sc := newServerConfig(viper.GetViper())
			srv, _ := server.New(nod, sc)

			go handleSignals(nod, srv)

			logger.INFO.Printf("Config path: %s", absConfPath)
			logger.INFO.Printf("Version: %s", version)
			logger.INFO.Printf("PID: %d", os.Getpid())
			logger.INFO.Printf("Engine: %s", e.Name())
			logger.INFO.Printf("GOMAXPROCS: %d", runtime.GOMAXPROCS(0))
			if c.Insecure {
				logger.WARN.Println("Running in INSECURE client mode")
			}
			if c.InsecureAPI {
				logger.WARN.Println("Running in INSECURE API mode")
			}
			if c.InsecureAdmin {
				logger.WARN.Println("Running in INSECURE admin mode")
			}
			if viper.GetBool("debug") {
				logger.WARN.Println("Running in DEBUG mode")
			}

			if err = runServer(nod, srv); err != nil {
				logger.FATAL.Fatalf("Error running server: %v", err)
			}

		},
	}

	rootCmd.Flags().StringVarP(&configFile, "config", "c", "config.json", "path to config file")
	rootCmd.Flags().StringVarP(&engn, "engine", "e", "memory", "engine to use: memory or redis")
	rootCmd.Flags().StringVarP(&logLevel, "log_level", "", "info", "set the log level: trace, debug, info, error, critical, fatal or none")
	rootCmd.Flags().StringVarP(&logFile, "log_file", "", "", "optional log file - if not specified logs go to STDOUT")
	rootCmd.Flags().StringVarP(&pidFile, "pid_file", "", "", "optional path to create PID file")

	viper.SetEnvPrefix("centrifugo")
	ConfigureNode(config.NewViperConfigSetter(viper.GetViper(), rootCmd.Flags()))
	ConfigureServer(config.NewViperConfigSetter(viper.GetViper(), rootCmd.Flags()))
	for _, configurator := range plugin.Configurators {
		configurator(config.NewViperConfigSetter(viper.GetViper(), rootCmd.Flags()))
	}

	var versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Centrifugo version number",
		Long:  `Print the version number of Centrifugo`,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("Centrifugo v%s (Go version: %s)\n", version, runtime.Version())
		},
	}

	var checkConfigFile string

	var checkConfigCmd = &cobra.Command{
		Use:   "checkconfig",
		Short: "Check configuration file",
		Long:  `Check Centrifugo configuration file`,
		Run: func(cmd *cobra.Command, args []string) {
			err := validateConfig(checkConfigFile)
			if err != nil {
				logger.FATAL.Fatalf("%v", err)
			}
		},
	}
	checkConfigCmd.Flags().StringVarP(&checkConfigFile, "config", "c", "config.json", "path to config file to check")

	var outputConfigFile string

	var generateConfigCmd = &cobra.Command{
		Use:   "genconfig",
		Short: "Generate simple configuration file to start with",
		Long:  `Generate simple configuration file to start with`,
		Run: func(cmd *cobra.Command, args []string) {
			err := generateConfig(outputConfigFile)
			if err != nil {
				logger.FATAL.Fatalf("%v", err)
			}
		},
	}
	generateConfigCmd.Flags().StringVarP(&outputConfigFile, "config", "c", "config.json", "path to output config file")

	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(checkConfigCmd)
	rootCmd.AddCommand(generateConfigCmd)
	rootCmd.Execute()
}
