package main

import (
	"bytes"
	"crypto/tls"
	"errors"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/centrifugal/centrifugo/lib/channel"
	"github.com/centrifugal/centrifugo/lib/engine"
	"github.com/centrifugal/centrifugo/lib/engine/enginememory"
	"github.com/centrifugal/centrifugo/lib/engine/engineredis"
	"github.com/centrifugal/centrifugo/lib/grpc/apiservice"
	"github.com/centrifugal/centrifugo/lib/grpc/clientservice"
	"github.com/centrifugal/centrifugo/lib/logging"
	"github.com/centrifugal/centrifugo/lib/node"
	"github.com/centrifugal/centrifugo/lib/proto"
	"github.com/centrifugal/centrifugo/lib/proto/apiproto"
	"github.com/centrifugal/centrifugo/lib/server"
	"github.com/centrifugal/centrifugo/lib/statik"

	"github.com/FZambia/go-logger"
	"github.com/FZambia/viper-lite"
	"github.com/igm/sockjs-go/sockjs"
	"github.com/satori/go.uuid"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/acme/autocert"
	"google.golang.org/grpc"
)

// VERSION of Centrifugo server. Set on build stage.
var VERSION string

func main() {
	var configFile string

	var rootCmd = &cobra.Command{
		Use:   "",
		Short: "Centrifugo",
		Long:  "Centrifugo – real-time messaging server",
		Run: func(cmd *cobra.Command, args []string) {

			defaults := map[string]interface{}{
				"gomaxprocs":                     0,
				"engine":                         "memory",
				"debug":                          false,
				"name":                           "",
				"secret":                         "",
				"watch":                          false,
				"publish":                        false,
				"anonymous":                      false,
				"presence":                       false,
				"presence_stats":                 false,
				"history_size":                   0,
				"history_lifetime":               0,
				"history_recover":                false,
				"history_drop_inactive":          false,
				"namespaces":                     "",
				"node_ping_interval":             3,
				"node_metrics_interval":          60,
				"client_ping_interval":           25,
				"client_expire":                  false,
				"client_expired_close_delay":     25,
				"client_stale_close_delay":       25,
				"client_message_write_timeout":   0,
				"client_channel_limit":           128,
				"client_request_max_size":        65536,    // 64KB
				"client_queue_max_size":          10485760, // 10MB
				"presence_ping_interval":         25,
				"presence_expire_interval":       60,
				"channel_max_length":             255,
				"channel_private_prefix":         "$",
				"channel_namespace_boundary":     ":",
				"channel_user_boundary":          "#",
				"channel_user_separator":         ",",
				"channel_client_boundary":        "&",
				"user_connection_limit":          0,
				"http_prefix":                    "",
				"admin":                          false,
				"admin_web_path":                 "",
				"admin_password":                 "",
				"admin_secret":                   "",
				"admin_insecure":                 false,
				"sockjs_url":                     "//cdn.jsdelivr.net/sockjs/1.1/sockjs.min.js",
				"sockjs_heartbeat_delay":         25,
				"websocket_compression":          false,
				"websocket_compression_min_size": 0,
				"websocket_compression_level":    1,
				"websocket_read_buffer_size":     0,
				"websocket_write_buffer_size":    0,
				"tls_autocert":                   false,
				"tls_autocert_host_whitelist":    "",
				"tls_autocert_cache_dir":         "",
				"tls_autocert_email":             "",
				"tls_autocert_force_rsa":         false,
				"tls_autocert_server_name":       "",
				"redis_prefix":                   "centrifugo",
				"redis_connect_timeout":          1,
				"redis_read_timeout":             10, // Must be greater than ping channel publish interval.
				"redis_write_timeout":            1,
				"redis_pubsub_num_workers":       0,
				"grpc_api":                       false,
				"grpc_api_port":                  8001,
				"grpc_client":                    false,
				"grpc_client_port":               8002,
				"shutdown_time_limit":            30,
				"shutdown_termination_delay":     1,
			}

			for k, v := range defaults {
				viper.SetDefault(k, v)
			}

			bindEnvs := []string{
				"engine", "debug", "secret", "connection_lifetime", "watch",
				"publish", "anonymous", "join_leave", "presence", "presence_stats",
				"history_recover", "history_size", "history_lifetime", "history_drop_inactive",
				"client_insecure", "api_insecure", "admin", "admin_password", "admin_secret",
				"admin_insecure", "redis_host", "redis_port", "redis_url",
			}
			for _, env := range bindEnvs {
				viper.BindEnv(env)
			}

			bindPFlags := []string{
				"engine", "log_level", "log_file", "pid_file", "debug", "name", "admin",
				"client_insecure", "admin_insecure", "api_insecure", "port", "api_port", "admin_port",
				"address", "tls", "tls_cert", "tls_key",
				"redis_host", "redis_port", "redis_password", "redis_db", "redis_url",
				"redis_pool", "redis_master_name", "redis_sentinels", "grpc_api", "grpc_client",
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

			node.VERSION = VERSION
			nod := node.New(c)
			setLogHandler(nod)

			engineName := viper.GetString("engine")

			var e engine.Engine
			if engineName == "memory" {
				e, err = memoryEngine(nod, viper.GetViper())
			} else if engineName == "redis" {
				e, err = redisEngine(nod, viper.GetViper())
			} else {
				logger.FATAL.Fatalf("Unknown engine: %s", engineName)
			}

			if err != nil {
				logger.FATAL.Fatalf("Error creating engine: %v", err)
			}

			if err = nod.Run(e); err != nil {
				logger.FATAL.Fatalf("Error running node: %v", err)
			}

			httpServerConfig := serverConfig(viper.GetViper())
			httpServer, _ := server.New(nod, httpServerConfig)

			var grpcAPIServer *grpc.Server
			var grpcAPIAddr string
			if viper.GetBool("grpc_api") {
				grpcAPIAddr = fmt.Sprintf(":%d", viper.GetInt("grpc_api_port"))
				grpcAPIConn, err := net.Listen("tcp", grpcAPIAddr)
				if err != nil {
					logger.FATAL.Fatalf("Cannot listen to address %s", grpcAPIAddr)
				}
				grpcAPIServer = grpc.NewServer()
				apiproto.RegisterCentrifugoServer(grpcAPIServer, apiservice.New(nod, apiservice.Config{}))
				go func() {
					if err := grpcAPIServer.Serve(grpcAPIConn); err != nil {
						logger.FATAL.Fatalf("Serve GRPC: %v", err)
					}
				}()
			}

			var grpcClientServer *grpc.Server
			var grpcClientAddr string
			if viper.GetBool("grpc_client") {
				grpcClientAddr = fmt.Sprintf(":%d", viper.GetInt("grpc_client_port"))
				grpcClientConn, err := net.Listen("tcp", grpcClientAddr)
				if err != nil {
					logger.FATAL.Fatalf("Cannot listen to address %s", grpcClientAddr)
				}
				grpcClientServer = grpc.NewServer()
				proto.RegisterCentrifugoServer(grpcClientServer, clientservice.New(nod, clientservice.Config{}))
				go func() {
					if err := grpcClientServer.Serve(grpcClientConn); err != nil {
						logger.FATAL.Fatalf("Serve GRPC: %v", err)
					}
				}()
			}

			go func() {
				if err = runServer(nod, httpServer); err != nil {
					logger.FATAL.Fatalf("Error running server: %v", err)
				}
			}()

			logger.INFO.Printf("Config path: %s", absConfPath)
			logger.INFO.Printf("Version: %s", VERSION)
			logger.INFO.Printf("PID: %d", os.Getpid())
			logger.INFO.Printf("Engine: %s", e.Name())
			logger.INFO.Printf("GOMAXPROCS: %d", runtime.GOMAXPROCS(0))
			if c.ClientInsecure {
				logger.WARN.Println("INSECURE client mode enabled")
			}
			if viper.GetBool("api_insecure") {
				logger.WARN.Println("INSECURE API mode enabled")
			}
			if httpServerConfig.AdminInsecure {
				logger.WARN.Println("INSECURE admin mode enabled")
			}
			if viper.GetBool("debug") {
				logger.WARN.Println("DEBUG mode enabled")
			}
			if grpcAPIServer != nil {
				logger.INFO.Printf("Serving GRPC API service on %s", grpcAPIAddr)
			}
			if grpcClientServer != nil {
				logger.INFO.Printf("Serving GRPC client service on %s", grpcClientAddr)
			}

			handleSignals(nod, httpServer, grpcAPIServer, grpcClientServer)
		},
	}

	rootCmd.Flags().StringVarP(&configFile, "config", "c", "config.json", "path to config file")
	rootCmd.Flags().StringP("engine", "e", "memory", "engine to use: memory or redis")
	rootCmd.Flags().StringP("log_level", "", "info", "set the log level: debug, info, error, critical, fatal or none")
	rootCmd.Flags().StringP("log_file", "", "", "optional log file - if not specified logs go to STDOUT")
	rootCmd.Flags().StringP("pid_file", "", "", "optional path to create PID file")
	rootCmd.Flags().StringP("name", "n", "", "unique node name")

	rootCmd.Flags().BoolP("debug", "d", false, "enable debug mode")
	rootCmd.Flags().BoolP("admin", "", false, "enable admin socket")
	rootCmd.Flags().BoolP("client_insecure", "", false, "start in insecure client mode")
	rootCmd.Flags().BoolP("api_insecure", "", false, "use insecure API mode")
	rootCmd.Flags().BoolP("admin_insecure", "", false, "use insecure admin mode – no auth required for admin socket")

	rootCmd.Flags().BoolP("tls", "", false, "enable TLS, requires an X509 certificate and a key file")
	rootCmd.Flags().StringP("tls_cert", "", "", "path to an X509 certificate file")
	rootCmd.Flags().StringP("tls_key", "", "", "path to an X509 certificate key")

	rootCmd.Flags().StringP("address", "a", "", "interface address to listen on")
	rootCmd.Flags().StringP("port", "p", "8000", "port to bind HTTP server to")
	rootCmd.Flags().StringP("api_port", "", "", "custom port for API endpoints")
	rootCmd.Flags().StringP("admin_port", "", "", "custom port for admin endpoints")

	rootCmd.Flags().BoolP("grpc_api", "", false, "enable GRPC API server")
	rootCmd.Flags().BoolP("grpc_client", "", false, "enable GRPC client server")

	rootCmd.Flags().StringP("redis_host", "", "127.0.0.1", "Redis host (Redis engine)")
	rootCmd.Flags().StringP("redis_port", "", "6379", "Redis port (Redis engine)")
	rootCmd.Flags().StringP("redis_password", "", "", "Redis auth password (Redis engine)")
	rootCmd.Flags().StringP("redis_db", "", "0", "Redis database (Redis engine)")
	rootCmd.Flags().StringP("redis_url", "", "", "Redis connection URL in format redis://:password@hostname:port/db (Redis engine)")
	rootCmd.Flags().IntP("redis_pool", "", 256, "Redis pool size (Redis engine)")
	rootCmd.Flags().StringP("redis_master_name", "", "", "name of Redis master Sentinel monitors (Redis engine)")
	rootCmd.Flags().StringP("redis_sentinels", "", "", "comma-separated list of Sentinel addresses (Redis engine)")

	viper.SetEnvPrefix("centrifugo")

	var versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Centrifugo version number",
		Long:  `Print the version number of Centrifugo`,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("Centrifugo v%s (Go version: %s)\n", VERSION, runtime.Version())
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
		// do not log into stdout when log file provided.
		logger.SetStdoutThreshold(logger.LevelNone)
	}
}

func handleSignals(n *node.Node, httpServer *server.HTTPServer, grpcAPIServer *grpc.Server, grpcClientServer *grpc.Server) {
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGHUP, syscall.SIGINT, os.Interrupt, syscall.SIGTERM)
	for {
		sig := <-sigc
		logger.INFO.Println("Signal received:", sig)
		switch sig {
		case syscall.SIGHUP:
			// reload application configuration on SIGHUP.
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
			go time.AfterFunc(time.Duration(viper.GetInt("shutdown_time_limit"))*time.Second, func() {
				if pidFile != "" {
					os.Remove(pidFile)
				}
				os.Exit(1)
			})

			httpServer.Shutdown()

			var wg sync.WaitGroup

			if grpcAPIServer != nil {
				wg.Add(1)
				go func() {
					defer wg.Done()
					grpcAPIServer.GracefulStop()
				}()
			}

			if grpcClientServer != nil {
				wg.Add(1)
				go func() {
					defer wg.Done()
					grpcClientServer.GracefulStop()
				}()
			}

			n.Shutdown()

			wg.Wait()

			if pidFile != "" {
				os.Remove(pidFile)
			}
			time.Sleep(time.Duration(viper.GetInt("shutdown_termination_delay")) * time.Second)
			os.Exit(0)
		}
	}
}

func getTLSConfig() (*tls.Config, error) {
	tlsEnabled := viper.GetBool("tls")
	tlsCert := viper.GetString("tls_cert")
	tlsKey := viper.GetString("tls_key")
	tlsAutocertEnabled := viper.GetBool("tls_autocert")
	autocertHostWhitelist := viper.GetString("tls_autocert_host_whitelist")
	var tlsAutocertHostWhitelist []string
	if autocertHostWhitelist != "" {
		tlsAutocertHostWhitelist = strings.Split(autocertHostWhitelist, ",")
	} else {
		tlsAutocertHostWhitelist = nil
	}
	tlsAutocertCacheDir := viper.GetString("tls_autocert_cache_dir")
	tlsAutocertEmail := viper.GetString("tls_autocert_email")
	tlsAutocertForceRSA := viper.GetBool("tls_autocert_force_rsa")
	tlsAutocertServerName := viper.GetString("tls_autocert_server_name")

	if tlsAutocertEnabled {
		certManager := autocert.Manager{
			Prompt:   autocert.AcceptTOS,
			ForceRSA: tlsAutocertForceRSA,
			Email:    tlsAutocertEmail,
		}
		if tlsAutocertHostWhitelist != nil {
			certManager.HostPolicy = autocert.HostWhitelist(tlsAutocertHostWhitelist...)
		}
		if tlsAutocertCacheDir != "" {
			certManager.Cache = autocert.DirCache(tlsAutocertCacheDir)
		}
		return &tls.Config{
			GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
				// See https://github.com/centrifugal/centrifugo/issues/144#issuecomment-279393819
				if tlsAutocertServerName != "" && hello.ServerName == "" {
					hello.ServerName = tlsAutocertServerName
				}
				return certManager.GetCertificate(hello)
			},
		}, nil

	} else if tlsEnabled {
		// Autocert disabled - just try to use provided SSL cert and key files.
		tlsConfig := &tls.Config{}
		tlsConfig = tlsConfig.Clone()
		tlsConfig.Certificates = make([]tls.Certificate, 1)
		var err error
		tlsConfig.Certificates[0], err = tls.LoadX509KeyPair(tlsCert, tlsKey)
		if err != nil {
			return nil, err
		}
		return tlsConfig, nil
	}

	return nil, nil
}

func runServer(n *node.Node, s *server.HTTPServer) error {
	debug := viper.GetBool("debug")
	admin := viper.GetBool("admin")
	adminWebPath := viper.GetString("admin_web_path")
	httpAddress := viper.GetString("address")
	httpClientPort := viper.GetString("port")
	httpAdminPort := viper.GetString("admin_port")
	httpAPIPort := viper.GetString("api_port")
	httpPrefix := viper.GetString("http_prefix")
	sockjsURL := viper.GetString("sockjs_url")
	sockjsHeartbeatDelay := viper.GetInt("sockjs_heartbeat_delay")

	websocketReadBufferSize := viper.GetInt("websocket_read_buffer_size")
	websocketWriteBufferSize := viper.GetInt("websocket_write_buffer_size")

	sockjs.WebSocketReadBufSize = websocketReadBufferSize
	sockjs.WebSocketWriteBufSize = websocketWriteBufferSize

	sockjsOpts := sockjs.DefaultOptions

	// Override sockjs url. It's important to use the same SockJS
	// library version on client and server sides when using iframe
	// based SockJS transports, otherwise SockJS will raise error
	// about version mismatch.
	if sockjsURL != "" {
		logger.DEBUG.Println("SockJS url:", sockjsURL)
		sockjsOpts.SockJSURL = sockjsURL
	}

	sockjsOpts.HeartbeatDelay = time.Duration(sockjsHeartbeatDelay) * time.Second

	var webFS http.FileSystem
	if admin {
		webFS = statik.FS
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
	portFlags |= server.HandlerWebsocket | server.HandlerSockJS
	portToHandlerFlags[httpClientPort] = portFlags

	portFlags = portToHandlerFlags[httpAPIPort]
	portFlags |= server.HandlerAPI
	portToHandlerFlags[httpAPIPort] = portFlags

	portFlags = portToHandlerFlags[httpAdminPort]
	if admin {
		portFlags |= server.HandlerAdmin
	}
	if debug {
		portFlags |= server.HandlerDebug
	}
	portToHandlerFlags[httpAdminPort] = portFlags

	var wg sync.WaitGroup
	// Iterate over port to flags mapping and start HTTP servers
	// on separate ports serving handlers specified in flags.
	for handlerPort, handlerFlags := range portToHandlerFlags {
		if handlerFlags == 0 {
			continue
		}
		muxOpts := server.MuxOptions{
			Prefix:        httpPrefix,
			WebPath:       adminWebPath,
			WebFS:         webFS,
			HandlerFlags:  handlerFlags,
			SockjsOptions: sockjsOpts,
		}
		mux := server.ServeMux(s, muxOpts)

		addr := net.JoinHostPort(httpAddress, handlerPort)

		logger.INFO.Printf("Serving %s endpoints on %s\n", handlerFlags, addr)

		wg.Add(1)
		go func() {
			defer wg.Done()

			tlsConfig, err := getTLSConfig()
			if err != nil {
				logger.FATAL.Fatalf("Can not get TLS config: %v", err)
			}
			server := &http.Server{
				Addr:      addr,
				Handler:   mux,
				TLSConfig: tlsConfig,
			}
			if tlsConfig != nil {
				if err := server.ListenAndServeTLS("", ""); err != nil {
					logger.FATAL.Fatalf("ListenAndServe: %v", err)
				}
			} else {
				if err := server.ListenAndServe(); err != nil {
					logger.FATAL.Fatalf("ListenAndServe: %v", err)
				}
			}
		}()
	}

	wg.Wait()

	return nil
}

// pathExists returns whether the given file or directory exists or not
func pathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

var jsonConfigTemplate = `{
  "secret": "{{.Secret}}"
}
`

var tomlConfigTemplate = `secret = {{.Secret}}
`

var yamlConfigTemplate = `secret: {{.Secret}}
`

// generateConfig generates configuration file at provided path.
func generateConfig(f string) error {
	exists, err := pathExists(f)
	if err != nil {
		return err
	}
	if exists {
		return errors.New("output config file already exists: " + f)
	}
	ext := filepath.Ext(f)

	if len(ext) > 1 {
		ext = ext[1:]
	}

	supportedExts := []string{"json", "toml", "yaml", "yml"}

	if !stringInSlice(ext, supportedExts) {
		return errors.New("output config file must have one of supported extensions: " + strings.Join(supportedExts, ", "))
	}

	var t *template.Template

	switch ext {
	case "json":
		t, err = template.New("config").Parse(jsonConfigTemplate)
	case "toml":
		t, err = template.New("config").Parse(tomlConfigTemplate)
	case "yaml", "yml":
		t, err = template.New("config").Parse(yamlConfigTemplate)
	}
	if err != nil {
		return err
	}

	var output bytes.Buffer
	t.Execute(&output, struct {
		Secret string
	}{
		uuid.NewV4().String(),
	})

	err = ioutil.WriteFile(f, output.Bytes(), 0644)
	if err != nil {
		return err
	}

	err = validateConfig(f)
	if err != nil {
		_ = os.Remove(f)
		return err
	}

	return nil
}

// validateConfig validates config file located at provided path.
func validateConfig(f string) error {
	v := viper.New()
	v.SetConfigFile(f)
	err := v.ReadInConfig()
	if err != nil {
		switch err.(type) {
		case viper.ConfigParseError:
			return err
		default:
			return errors.New("Unable to locate config file, use \"centrifugo genconfig -c " + f + "\" command to generate one")
		}
	}
	c := newNodeConfig(v)
	return c.Validate()
}

func stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

// newNodeConfig creates new node.Config using getter interface.
func newNodeConfig(v *viper.Viper) *node.Config {
	cfg := &node.Config{}

	cfg.Name = applicationName(v)

	cfg.Secret = v.GetString("secret")
	cfg.Watch = v.GetBool("watch")
	cfg.Publish = v.GetBool("publish")
	cfg.Anonymous = v.GetBool("anonymous")
	cfg.Presence = v.GetBool("presence")
	cfg.PresenceStats = v.GetBool("presence_stats")
	cfg.JoinLeave = v.GetBool("join_leave")
	cfg.HistorySize = v.GetInt("history_size")
	cfg.HistoryLifetime = v.GetInt("history_lifetime")
	cfg.HistoryDropInactive = v.GetBool("history_drop_inactive")
	cfg.HistoryRecover = v.GetBool("history_recover")
	cfg.Namespaces = namespacesFromConfig(v)

	cfg.NodePingInterval = time.Duration(v.GetInt("node_ping_interval")) * time.Second
	cfg.NodeInfoCleanInterval = cfg.NodePingInterval * 3
	cfg.NodeInfoMaxDelay = cfg.NodePingInterval*2 + 1*time.Second
	cfg.NodeMetricsInterval = time.Duration(v.GetInt("node_metrics_interval")) * time.Second

	cfg.PresencePingInterval = time.Duration(v.GetInt("presence_ping_interval")) * time.Second
	cfg.PresenceExpireInterval = time.Duration(v.GetInt("presence_expire_interval")) * time.Second

	cfg.ChannelMaxLength = v.GetInt("channel_max_length")
	cfg.ChannelPrivatePrefix = v.GetString("channel_private_prefix")
	cfg.ChannelNamespaceBoundary = v.GetString("channel_namespace_boundary")
	cfg.ChannelUserBoundary = v.GetString("channel_user_boundary")
	cfg.ChannelUserSeparator = v.GetString("channel_user_separator")
	cfg.ChannelClientBoundary = v.GetString("channel_client_boundary")

	cfg.ClientPingInterval = time.Duration(v.GetInt("client_ping_interval")) * time.Second
	cfg.ClientMessageWriteTimeout = time.Duration(v.GetInt("client_message_write_timeout")) * time.Second
	cfg.ClientInsecure = v.GetBool("client_insecure")
	cfg.ClientExpire = v.GetBool("client_expire")
	cfg.ClientExpiredCloseDelay = time.Duration(v.GetInt("client_expired_close_delay")) * time.Second
	cfg.ClientStaleCloseDelay = time.Duration(v.GetInt("client_stale_close_delay")) * time.Second
	cfg.ClientRequestMaxSize = v.GetInt("client_request_max_size")
	cfg.ClientQueueMaxSize = v.GetInt("client_queue_max_size")
	cfg.ClientChannelLimit = v.GetInt("client_channel_limit")

	cfg.UserConnectionLimit = v.GetInt("user_connection_limit")

	return cfg
}

// applicationName returns a name for this node. If no name provided
// in configuration then it constructs node name based on hostname and port
func applicationName(v *viper.Viper) string {
	name := v.GetString("name")
	if name != "" {
		return name
	}
	port := v.GetString("port")
	var hostname string
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "?"
	}
	return hostname + "_" + port
}

// namespacesFromConfig allows to unmarshal channel namespaces.
func namespacesFromConfig(v *viper.Viper) []channel.Namespace {
	ns := []channel.Namespace{}
	if !v.IsSet("namespaces") {
		return ns
	}
	v.UnmarshalKey("namespaces", &ns)
	return ns
}

// serverConfig creates new server config using viper.
func serverConfig(v *viper.Viper) *server.Config {
	cfg := &server.Config{}
	cfg.APIKey = v.GetString("api_key")
	cfg.APIInsecure = v.GetBool("api_insecure")
	cfg.AdminPassword = v.GetString("admin_password")
	cfg.AdminSecret = v.GetString("admin_secret")
	cfg.AdminInsecure = v.GetBool("admin_insecure")
	cfg.WebsocketCompression = v.GetBool("websocket_compression")
	cfg.WebsocketCompressionLevel = v.GetInt("websocket_compression_level")
	cfg.WebsocketCompressionMinSize = v.GetInt("websocket_compression_min_size")
	cfg.WebsocketReadBufferSize = v.GetInt("websocket_read_buffer_size")
	cfg.WebsocketWriteBufferSize = v.GetInt("websocket_write_buffer_size")
	return cfg
}

func memoryEngine(n *node.Node, v *viper.Viper) (engine.Engine, error) {
	c, err := memoryEngineConfig(v)
	if err != nil {
		return nil, err
	}
	return enginememory.New(n, c)
}

func redisEngine(n *node.Node, v *viper.Viper) (engine.Engine, error) {
	c, err := redisEngineConfig(v)
	if err != nil {
		return nil, err
	}
	return engineredis.New(n, c)
}

func memoryEngineConfig(getter *viper.Viper) (*enginememory.Config, error) {
	return &enginememory.Config{}, nil
}

func redisEngineConfig(getter *viper.Viper) (*engineredis.Config, error) {
	numShards := 1

	hostsConf := getter.GetString("redis_host")
	portsConf := getter.GetString("redis_port")
	urlsConf := getter.GetString("redis_url")
	masterNamesConf := getter.GetString("redis_master_name")
	sentinelsConf := getter.GetString("redis_sentinels")

	password := getter.GetString("redis_password")
	db := getter.GetString("redis_db")

	hosts := []string{}
	if hostsConf != "" {
		hosts = strings.Split(hostsConf, ",")
		if len(hosts) > numShards {
			numShards = len(hosts)
		}
	}

	ports := []string{}
	if portsConf != "" {
		ports = strings.Split(portsConf, ",")
		if len(ports) > numShards {
			numShards = len(ports)
		}
	}

	urls := []string{}
	if urlsConf != "" {
		urls = strings.Split(urlsConf, ",")
		if len(urls) > numShards {
			numShards = len(urls)
		}
	}

	masterNames := []string{}
	if masterNamesConf != "" {
		masterNames = strings.Split(masterNamesConf, ",")
		if len(masterNames) > numShards {
			numShards = len(masterNames)
		}
	}

	if masterNamesConf != "" && sentinelsConf == "" {
		return nil, fmt.Errorf("Provide at least one Sentinel address")
	}

	if masterNamesConf != "" && len(masterNames) < numShards {
		return nil, fmt.Errorf("Redis master name must be set for every Redis shard when Sentinel used")
	}

	sentinelAddrs := []string{}
	if sentinelsConf != "" {
		for _, addr := range strings.Split(sentinelsConf, ",") {
			addr := strings.TrimSpace(addr)
			if addr == "" {
				continue
			}
			if _, _, err := net.SplitHostPort(addr); err != nil {
				return nil, fmt.Errorf("Malformed Sentinel address: %s", addr)
			}
			sentinelAddrs = append(sentinelAddrs, addr)
		}
	}

	if len(hosts) <= 1 {
		newHosts := make([]string, numShards)
		for i := 0; i < numShards; i++ {
			if len(hosts) == 0 {
				newHosts[i] = ""
			} else {
				newHosts[i] = hosts[0]
			}
		}
		hosts = newHosts
	} else if len(hosts) != numShards {
		return nil, fmt.Errorf("Malformed sharding configuration: wrong number of redis hosts")
	}

	if len(ports) <= 1 {
		newPorts := make([]string, numShards)
		for i := 0; i < numShards; i++ {
			if len(ports) == 0 {
				newPorts[i] = ""
			} else {
				newPorts[i] = ports[0]
			}
		}
		ports = newPorts
	} else if len(ports) != numShards {
		return nil, fmt.Errorf("Malformed sharding configuration: wrong number of redis ports")
	}

	if len(urls) > 0 && len(urls) != numShards {
		return nil, fmt.Errorf("Malformed sharding configuration: wrong number of redis urls")
	}

	if len(masterNames) == 0 {
		newMasterNames := make([]string, numShards)
		for i := 0; i < numShards; i++ {
			newMasterNames[i] = ""
		}
		masterNames = newMasterNames
	}

	passwords := make([]string, numShards)
	for i := 0; i < numShards; i++ {
		passwords[i] = password
	}

	dbs := make([]int, numShards)
	for i := 0; i < numShards; i++ {
		dbNum, err := strconv.Atoi(db)
		if err != nil {
			return nil, fmt.Errorf("malformed Redis db number: %s", db)
		}
		dbs[i] = dbNum
	}

	for i, confURL := range urls {
		if confURL == "" {
			continue
		}
		// If URL set then prefer it over other parameters.
		u, err := url.Parse(confURL)
		if err != nil {
			return nil, fmt.Errorf("%v", err)
		}
		if u.User != nil {
			var ok bool
			pass, ok := u.User.Password()
			if !ok {
				pass = ""
			}
			passwords[i] = pass
		}
		host, port, err := net.SplitHostPort(u.Host)
		if err != nil {
			return nil, fmt.Errorf("%v", err)
		}
		path := u.Path
		if path != "" {
			dbNum, err := strconv.Atoi(path[1:])
			if err != nil {
				return nil, fmt.Errorf("malformed Redis db number: %s", path[1:])
			}
			dbs[i] = dbNum
		}
		hosts[i] = host
		ports[i] = port
	}

	var shardConfigs []*engineredis.ShardConfig

	for i := 0; i < numShards; i++ {
		conf := &engineredis.ShardConfig{
			Host:             hosts[i],
			Port:             ports[i],
			Password:         passwords[i],
			DB:               dbs[i],
			MasterName:       masterNames[i],
			SentinelAddrs:    sentinelAddrs,
			PoolSize:         getter.GetInt("redis_pool"),
			Prefix:           getter.GetString("redis_prefix"),
			PubSubNumWorkers: getter.GetInt("redis_pubsub_num_workers"),
			ConnectTimeout:   time.Duration(getter.GetInt("redis_connect_timeout")) * time.Second,
			ReadTimeout:      time.Duration(getter.GetInt("redis_read_timeout")) * time.Second,
			WriteTimeout:     time.Duration(getter.GetInt("redis_write_timeout")) * time.Second,
		}
		shardConfigs = append(shardConfigs, conf)
	}

	return &engineredis.Config{
		Shards: shardConfigs,
	}, nil
}

func setLogHandler(n *node.Node) {
	level, ok := logging.StringToLevel[strings.ToLower(viper.GetString("log_level"))]
	if !ok {
		level = logging.INFO
	}
	handler := newLogHandler()
	n.SetLogger(logging.New(level, handler.handle))
}

type logHandler struct {
	entries chan logging.Entry
	handler func(entry logging.Entry)
}

func newLogHandler() *logHandler {
	h := &logHandler{
		entries: make(chan logging.Entry, 64),
	}
	go h.readEntries()
	return h
}

func (h *logHandler) readEntries() {
	for entry := range h.entries {
		var log *logger.LevelLogger
		switch entry.Level {
		case logging.DEBUG:
			log = logger.DEBUG
		case logging.INFO:
			log = logger.INFO
		case logging.ERROR:
			log = logger.ERROR
		case logging.CRITICAL:
			log = logger.CRITICAL
		}
		if entry.Fields != nil {
			log.Printf("%s: %v", entry.Message, entry.Fields)
		} else {
			log.Println(entry.Message)
		}
	}
}

func (h *logHandler) handle(entry logging.Entry) {
	select {
	case h.entries <- entry:
	default:
		return
	}
}
