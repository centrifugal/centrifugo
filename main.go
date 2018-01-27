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
	"github.com/centrifugal/centrifugo/lib/grpcserver"
	"github.com/centrifugal/centrifugo/lib/logger"
	"github.com/centrifugal/centrifugo/lib/node"
	"github.com/centrifugal/centrifugo/lib/proto/api"
	"github.com/centrifugal/centrifugo/lib/server"
	"github.com/centrifugal/centrifugo/lib/statik"

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
		Long:  "Centrifugo – real-time messaging (Websockets or SockJS) server in Go",
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
				"max_channel_length":             255,
				"user_connection_limit":          0,
				"node_ping_interval":             3,
				"ping_interval":                  25,
				"node_metrics_interval":          60,
				"client_expire":                  false,
				"client_expired_close_delay":     25,
				"client_stale_close_delay":       25,
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
				"http_prefix":                    "",
				"admin":                          false,
				"admin_password":                 "",
				"admin_secret":                   "",
				"web":                            false,
				"web_path":                       "",
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
				"redis_prefix":                   "centrifugo",
				"redis_connect_timeout":          1,
				"redis_read_timeout":             10, // Must be greater than ping channel publish interval.
				"redis_write_timeout":            1,
				"redis_pubsub_num_workers":       0,
			}

			for k, v := range defaults {
				viper.SetDefault(k, v)
			}

			bindEnvs := []string{
				"engine", "debug", "insecure", "insecure_api", "admin", "admin_password",
				"admin_secret", "insecure_admin", "secret", "connection_lifetime", "watch",
				"publish", "anonymous", "join_leave", "presence", "presence_stats",
				"history_recover", "history_size", "history_lifetime", "history_drop_inactive",
				"web", "redis_host", "redis_port", "redis_url",
			}
			for _, env := range bindEnvs {
				viper.BindEnv(env)
			}

			bindPFlags := []string{
				"engine", "log_level", "log_file", "pid_file", "debug", "name", "admin",
				"insecure", "insecure_admin", "insecure_api", "port", "api_port", "admin_port",
				"address", "web", "web_path", "insecure_web", "ssl", "ssl_cert", "ssl_key",
				"redis_host", "redis_port", "redis_password", "redis_db", "redis_url",
				"redis_pool", "redis_master_name", "redis_sentinels",
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

			sc := serverConfig(viper.GetViper())
			srv, _ := server.New(nod, sc)

			grpcAddr := fmt.Sprintf(":%d", 8001)
			conn, err := net.Listen("tcp", grpcAddr)
			if err != nil {
				logger.FATAL.Fatalf("Cannot listen to address %s", grpcAddr)
			}
			grpcServer := grpc.NewServer()
			api.RegisterCentrifugoServer(grpcServer, grpcserver.New(nod, grpcserver.Config{}))
			go func() {
				if err := grpcServer.Serve(conn); err != nil {
					logger.FATAL.Fatalf("Serve GRPC: %v", err)
				}
			}()

			go handleSignals(nod, srv, grpcServer)

			logger.INFO.Printf("Config path: %s", absConfPath)
			logger.INFO.Printf("Version: %s", VERSION)
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
			logger.INFO.Printf("Start serving GRPC API on %s", grpcAddr)

			if err = runServer(nod, srv); err != nil {
				logger.FATAL.Fatalf("Error running server: %v", err)
			}

		},
	}

	rootCmd.Flags().StringVarP(&configFile, "config", "c", "config.json", "path to config file")
	rootCmd.Flags().StringP("engine", "e", "memory", "engine to use: memory or redis")
	rootCmd.Flags().StringP("log_level", "", "info", "set the log level: debug, info, error, critical, fatal or none")
	rootCmd.Flags().StringP("log_file", "", "", "optional log file - if not specified logs go to STDOUT")
	rootCmd.Flags().StringP("pid_file", "", "", "optional path to create PID file")

	rootCmd.Flags().BoolP("debug", "d", false, "enable debug mode")
	rootCmd.Flags().StringP("name", "n", "", "unique node name")
	rootCmd.Flags().BoolP("admin", "", false, "enable admin socket")
	rootCmd.Flags().BoolP("insecure", "", false, "start in insecure client mode")
	rootCmd.Flags().BoolP("insecure_api", "", false, "use insecure API mode")
	rootCmd.Flags().BoolP("insecure_admin", "", false, "use insecure admin mode – no auth required for admin socket")

	rootCmd.Flags().BoolP("web", "w", false, "serve admin web interface application (warning: automatically enables admin socket)")
	rootCmd.Flags().StringP("web_path", "", "", "optional path to custom web interface application")

	rootCmd.Flags().BoolP("ssl", "", false, "accept SSL connections. This requires an X509 certificate and a key file")
	rootCmd.Flags().StringP("ssl_cert", "", "", "path to an X509 certificate file")
	rootCmd.Flags().StringP("ssl_key", "", "", "path to an X509 certificate key")

	rootCmd.Flags().StringP("address", "a", "", "address to listen on")
	rootCmd.Flags().StringP("port", "p", "8000", "port to bind HTTP server to")
	rootCmd.Flags().StringP("api_port", "", "", "port to bind api endpoints to (optional)")
	rootCmd.Flags().StringP("admin_port", "", "", "port to bind admin endpoints to (optional)")

	rootCmd.Flags().StringP("redis_host", "", "127.0.0.1", "redis host (Redis engine)")
	rootCmd.Flags().StringP("redis_port", "", "6379", "redis port (Redis engine)")
	rootCmd.Flags().StringP("redis_password", "", "", "redis auth password (Redis engine)")
	rootCmd.Flags().StringP("redis_db", "", "0", "redis database (Redis engine)")
	rootCmd.Flags().StringP("redis_url", "", "", "redis connection URL in format redis://:password@hostname:port/db (Redis engine)")
	rootCmd.Flags().IntP("redis_pool", "", 256, "Redis pool size (Redis engine)")
	rootCmd.Flags().StringP("redis_master_name", "", "", "Name of Redis master Sentinel monitors (Redis engine)")
	rootCmd.Flags().StringP("redis_sentinels", "", "", "Comma separated list of Sentinels (Redis engine)")

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
		// do not log into stdout when log file provided
		logger.SetStdoutThreshold(logger.LevelNone)
	}
}

func handleSignals(n *node.Node, s *server.HTTPServer, grpcServer *grpc.Server) {
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
			go func() {
				grpcServer.GracefulStop()
			}()
			if pidFile != "" {
				os.Remove(pidFile)
			}
			os.Exit(0)
		}
	}
}

func runServer(n *node.Node, s *server.HTTPServer) error {
	debug := viper.GetBool("debug")
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
	if webEnabled {
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
	if webEnabled {
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
	cfg.Admin = v.GetBool("admin")
	cfg.AdminPassword = v.GetString("admin_password")
	cfg.AdminSecret = v.GetString("admin_secret")
	cfg.MaxChannelLength = v.GetInt("max_channel_length")
	cfg.PingInterval = time.Duration(v.GetInt("ping_interval")) * time.Second
	cfg.NodePingInterval = time.Duration(v.GetInt("node_ping_interval")) * time.Second
	cfg.NodeInfoCleanInterval = cfg.NodePingInterval * 3
	cfg.NodeInfoMaxDelay = cfg.NodePingInterval*2 + 1*time.Second
	cfg.NodeMetricsInterval = time.Duration(v.GetInt("node_metrics_interval")) * time.Second
	cfg.PresencePingInterval = time.Duration(v.GetInt("presence_ping_interval")) * time.Second
	cfg.PresenceExpireInterval = time.Duration(v.GetInt("presence_expire_interval")) * time.Second
	cfg.ClientMessageWriteTimeout = time.Duration(v.GetInt("client_message_write_timeout")) * time.Second
	cfg.PrivateChannelPrefix = v.GetString("private_channel_prefix")
	cfg.NamespaceChannelBoundary = v.GetString("namespace_channel_boundary")
	cfg.UserChannelBoundary = v.GetString("user_channel_boundary")
	cfg.UserChannelSeparator = v.GetString("user_channel_separator")
	cfg.ClientChannelBoundary = v.GetString("client_channel_boundary")
	cfg.ClientExpire = v.GetBool("client_expire")
	cfg.ClientExpiredCloseDelay = time.Duration(v.GetInt("client_expired_close_delay")) * time.Second
	cfg.ClientStaleCloseDelay = time.Duration(v.GetInt("client_stale_close_delay")) * time.Second
	cfg.ClientRequestMaxSize = v.GetInt("client_request_max_size")
	cfg.ClientQueueMaxSize = v.GetInt("client_queue_max_size")
	cfg.ClientQueueInitialCapacity = v.GetInt("client_queue_initial_capacity")
	cfg.ClientChannelLimit = v.GetInt("client_channel_limit")
	cfg.UserConnectionLimit = v.GetInt("user_connection_limit")
	cfg.Insecure = v.GetBool("insecure")
	cfg.InsecureAPI = v.GetBool("insecure_api")
	cfg.InsecureAdmin = v.GetBool("insecure_admin")
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
func serverConfig(getter *viper.Viper) *server.Config {
	cfg := &server.Config{}
	cfg.WebsocketCompression = getter.GetBool("websocket_compression")
	cfg.WebsocketCompressionLevel = getter.GetInt("websocket_compression_level")
	cfg.WebsocketCompressionMinSize = getter.GetInt("websocket_compression_min_size")
	cfg.WebsocketReadBufferSize = getter.GetInt("websocket_read_buffer_size")
	cfg.WebsocketWriteBufferSize = getter.GetInt("websocket_write_buffer_size")
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
