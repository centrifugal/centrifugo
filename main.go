package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/http/pprof"
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

	"github.com/centrifugal/centrifugo/internal/admin"
	"github.com/centrifugal/centrifugo/internal/middleware"
	"github.com/centrifugal/centrifugo/internal/webui"

	"github.com/FZambia/go-logger"
	"github.com/FZambia/viper-lite"
	"github.com/centrifugal/centrifuge"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/satori/go.uuid"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/acme/autocert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
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

			for k, v := range configDefaults {
				viper.SetDefault(k, v)
			}

			bindEnvs := []string{
				"engine", "debug", "secret", "connection_lifetime",
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
				"client_insecure", "admin_insecure", "api_insecure", "port",
				"address", "tls", "tls_cert", "tls_key", "internal_port", "prometheus",
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

			c := nodeConfig()
			err = c.Validate()
			if err != nil {
				logger.FATAL.Fatalf("Error validating config: %v", err)
			}

			node := centrifuge.New(*c)
			setLogHandler(node)

			engineName := viper.GetString("engine")

			var e centrifuge.Engine
			if engineName == "memory" {
				e, err = memoryEngine(node)
			} else if engineName == "redis" {
				e, err = redisEngine(node)
			} else {
				logger.FATAL.Fatalf("Unknown engine: %s", engineName)
			}

			if err != nil {
				logger.FATAL.Fatalf("Error creating engine: %v", err)
			}

			if err = node.Run(e); err != nil {
				logger.FATAL.Fatalf("Error running node: %v", err)
			}

			logger.INFO.Printf("Starting Centrifugo %s", VERSION)
			logger.INFO.Printf("Config path: %s", absConfPath)
			logger.INFO.Printf("PID: %d", os.Getpid())
			logger.INFO.Printf("Engine: %s", strings.ToTitle(engineName))
			logger.INFO.Printf("GOMAXPROCS: %d", runtime.GOMAXPROCS(0))
			if viper.GetBool("client_insecure") {
				logger.WARN.Println("INSECURE client mode enabled")
			}
			if viper.GetBool("api_insecure") {
				logger.WARN.Println("INSECURE API mode enabled")
			}
			if viper.GetBool("admin_insecure") {
				logger.WARN.Println("INSECURE admin mode enabled")
			}
			if viper.GetBool("debug") {
				logger.WARN.Println("DEBUG mode enabled, see /debug/pprof")
			}

			var grpcAPIServer *grpc.Server
			var grpcAPIAddr string
			if viper.GetBool("grpc_api") {
				grpcAPIAddr = fmt.Sprintf(":%d", viper.GetInt("grpc_api_port"))
				grpcAPIConn, err := net.Listen("tcp", grpcAPIAddr)
				if err != nil {
					logger.FATAL.Fatalf("Cannot listen to address %s", grpcAPIAddr)
				}
				grpcOpts := []grpc.ServerOption{}
				tlsConfig, err := getTLSConfig()
				if err != nil {
					logger.FATAL.Fatalf("Error getting TLS config: %v", err)
				}
				if tlsConfig != nil {
					grpcOpts = append(grpcOpts, grpc.Creds(credentials.NewTLS(tlsConfig)))
				}
				grpcAPIServer = grpc.NewServer(grpcOpts...)
				centrifuge.RegisterGRPCServerAPI(node, grpcAPIServer, centrifuge.GRPCAPIServiceConfig{})
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
				grpcOpts := []grpc.ServerOption{
					grpc.MaxRecvMsgSize(viper.GetInt("client_request_max_size")),
				}
				tlsConfig, err := getTLSConfig()
				if err != nil {
					logger.FATAL.Fatalf("Error getting TLS config: %v", err)
				}
				if tlsConfig != nil {
					grpcOpts = append(grpcOpts, grpc.Creds(credentials.NewTLS(tlsConfig)))
				}
				grpcClientServer = grpc.NewServer(grpcOpts...)
				centrifuge.RegisterGRPCServerClient(node, grpcClientServer, centrifuge.GRPCClientServiceConfig{})
				go func() {
					if err := grpcClientServer.Serve(grpcClientConn); err != nil {
						logger.FATAL.Fatalf("Serve GRPC: %v", err)
					}
				}()
			}

			if grpcAPIServer != nil {
				logger.INFO.Printf("Serving GRPC API service on %s", grpcAPIAddr)
			}
			if grpcClientServer != nil {
				logger.INFO.Printf("Serving GRPC client service on %s", grpcClientAddr)
			}

			servers, err := runHTTPServers(node)
			if err != nil {
				logger.FATAL.Fatalf("Error running HTTP server: %v", err)
			}

			handleSignals(node, servers, grpcAPIServer, grpcClientServer)
		},
	}

	rootCmd.Flags().StringVarP(&configFile, "config", "c", "config.json", "path to config file")
	rootCmd.Flags().StringP("engine", "e", "memory", "engine to use: memory or redis")
	rootCmd.Flags().StringP("log_level", "", "info", "set the log level: debug, info, error, fatal or none")
	rootCmd.Flags().StringP("log_file", "", "", "optional log file - if not specified logs go to STDOUT")
	rootCmd.Flags().StringP("pid_file", "", "", "optional path to create PID file")
	rootCmd.Flags().StringP("name", "n", "", "unique node name")

	rootCmd.Flags().BoolP("debug", "", false, "enable debug endpoints")
	rootCmd.Flags().BoolP("admin", "", false, "enable admin web interface")
	rootCmd.Flags().BoolP("prometheus", "", false, "enable Prometheus metrics endpoint")

	rootCmd.Flags().BoolP("client_insecure", "", false, "start in insecure client mode")
	rootCmd.Flags().BoolP("api_insecure", "", false, "use insecure API mode")
	rootCmd.Flags().BoolP("admin_insecure", "", false, "use insecure admin mode – no auth required for admin socket")

	rootCmd.Flags().BoolP("tls", "", false, "enable TLS, requires an X509 certificate and a key file")
	rootCmd.Flags().StringP("tls_cert", "", "", "path to an X509 certificate file")
	rootCmd.Flags().StringP("tls_key", "", "", "path to an X509 certificate key")

	rootCmd.Flags().StringP("address", "a", "", "interface address to listen on")
	rootCmd.Flags().StringP("port", "p", "8000", "port to bind HTTP server to")
	rootCmd.Flags().StringP("internal_port", "", "", "custom port for internal endpoints")

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

var configDefaults = map[string]interface{}{
	"gomaxprocs":                      0,
	"engine":                          "memory",
	"name":                            "",
	"secret":                          "",
	"publish":                         false,
	"anonymous":                       false,
	"presence":                        false,
	"presence_stats":                  false,
	"history_size":                    0,
	"history_lifetime":                0,
	"history_recover":                 false,
	"history_drop_inactive":           false,
	"namespaces":                      "",
	"node_ping_interval":              3,
	"node_metrics_interval":           60,
	"client_ping_interval":            25,
	"client_expire":                   false,
	"client_expired_close_delay":      25,
	"client_stale_close_delay":        25,
	"client_message_write_timeout":    0,
	"client_channel_limit":            128,
	"client_request_max_size":         65536,    // 64KB
	"client_queue_max_size":           10485760, // 10MB
	"client_presence_ping_interval":   25,
	"client_presence_expire_interval": 60,
	"client_user_connection_limit":    0,
	"channel_max_length":              255,
	"channel_private_prefix":          "$",
	"channel_namespace_boundary":      ":",
	"channel_user_boundary":           "#",
	"channel_user_separator":          ",",
	"channel_client_boundary":         "&",
	"debug":                           false,
	"prometheus":                      false,
	"admin":                           false,
	"admin_password":                  "",
	"admin_secret":                    "",
	"admin_insecure":                  false,
	"admin_web_path":                  "",
	"sockjs_url":                      "//cdn.jsdelivr.net/sockjs/1.1/sockjs.min.js",
	"sockjs_heartbeat_delay":          25,
	"websocket_compression":           false,
	"websocket_compression_min_size":  0,
	"websocket_compression_level":     1,
	"websocket_read_buffer_size":      0,
	"websocket_write_buffer_size":     0,
	"tls_autocert":                    false,
	"tls_autocert_host_whitelist":     "",
	"tls_autocert_cache_dir":          "",
	"tls_autocert_email":              "",
	"tls_autocert_force_rsa":          false,
	"tls_autocert_server_name":        "",
	"redis_prefix":                    "centrifugo",
	"redis_connect_timeout":           1,
	"redis_read_timeout":              10, // Must be greater than ping channel publish interval.
	"redis_write_timeout":             1,
	"redis_pubsub_num_workers":        0,
	"grpc_api":                        false,
	"grpc_api_port":                   8001,
	"grpc_api_key":                    "",
	"grpc_api_insecure":               false,
	"grpc_client":                     false,
	"grpc_client_port":                8002,
	"shutdown_timeout":                30,
	"shutdown_termination_delay":      1,
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

func handleSignals(n *centrifuge.Node, httpServers []*http.Server, grpcAPIServer *grpc.Server, grpcClientServer *grpc.Server) {
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
					logger.CRITICAL.Printf("Error parsing configuration: %s", err)
					continue
				default:
					logger.CRITICAL.Println("No config file found")
					continue
				}
			}

			setupLogging()

			cfg := nodeConfig()
			if err := n.Reload(*cfg); err != nil {
				logger.CRITICAL.Printf("Error reloading: %v", err)
				continue
			}
			logger.INFO.Println("Configuration successfully reloaded")
		case syscall.SIGINT, os.Interrupt, syscall.SIGTERM:
			logger.INFO.Println("Shutting down, wait...")
			pidFile := viper.GetString("pid_file")
			go time.AfterFunc(time.Duration(viper.GetInt("shutdown_timeout"))*time.Second, func() {
				if pidFile != "" {
					os.Remove(pidFile)
				}
				os.Exit(1)
			})

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

			for _, srv := range httpServers {
				wg.Add(1)
				go func(srv *http.Server) {
					defer wg.Done()
					srv.Shutdown(context.Background())
				}(srv)
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

func runHTTPServers(n *centrifuge.Node) ([]*http.Server, error) {
	debug := viper.GetBool("debug")

	admin := viper.GetBool("admin")
	prometheus := viper.GetBool("prometheus")

	httpAddress := viper.GetString("address")
	httpPort := viper.GetString("port")
	httpInternalPort := viper.GetString("internal_port")

	if httpInternalPort == "" {
		httpInternalPort = httpPort
	}

	// portToHandlerFlags contains mapping between ports and handler flags
	// to serve on this port.
	portToHandlerFlags := map[string]HandlerFlag{}

	var portFlags HandlerFlag

	portFlags = portToHandlerFlags[httpPort]
	portFlags |= HandlerWebsocket | HandlerSockJS
	portToHandlerFlags[httpPort] = portFlags

	portFlags = portToHandlerFlags[httpInternalPort]
	portFlags |= HandlerAPI

	if admin {
		portFlags |= HandlerAdmin
	}
	if prometheus {
		portFlags |= HandlerPrometheus
	}
	if debug {
		portFlags |= HandlerDebug
	}
	portToHandlerFlags[httpInternalPort] = portFlags

	servers := []*http.Server{}

	// Iterate over port to flags mapping and start HTTP servers
	// on separate ports serving handlers specified in flags.
	for handlerPort, handlerFlags := range portToHandlerFlags {
		if handlerFlags == 0 {
			continue
		}
		mux := Mux(n, handlerFlags)

		addr := net.JoinHostPort(httpAddress, handlerPort)

		logger.INFO.Printf("Serving %s endpoints on %s\n", handlerFlags, addr)

		tlsConfig, err := getTLSConfig()
		if err != nil {
			logger.FATAL.Fatalf("Can not get TLS config: %v", err)
		}
		server := &http.Server{
			Addr:      addr,
			Handler:   mux,
			TLSConfig: tlsConfig,
		}

		servers = append(servers, server)

		go func() {
			if tlsConfig != nil {
				if err := server.ListenAndServeTLS("", ""); err != nil {
					if err != http.ErrServerClosed {
						logger.FATAL.Fatalf("ListenAndServe: %v", err)
					}
				}
			} else {
				if err := server.ListenAndServe(); err != nil {
					if err != http.ErrServerClosed {
						logger.FATAL.Fatalf("ListenAndServe: %v", err)
					}
				}
			}
		}()
	}

	return servers, nil
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
	c := nodeConfig()
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

func nodeConfig() *centrifuge.Config {
	v := viper.GetViper()

	cfg := &centrifuge.Config{}

	cfg.Version = VERSION
	cfg.Name = applicationName()
	cfg.Secret = v.GetString("secret")

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

	cfg.ChannelMaxLength = v.GetInt("channel_max_length")
	cfg.ChannelPrivatePrefix = v.GetString("channel_private_prefix")
	cfg.ChannelNamespaceBoundary = v.GetString("channel_namespace_boundary")
	cfg.ChannelUserBoundary = v.GetString("channel_user_boundary")
	cfg.ChannelUserSeparator = v.GetString("channel_user_separator")
	cfg.ChannelClientBoundary = v.GetString("channel_client_boundary")

	cfg.ClientPresencePingInterval = time.Duration(v.GetInt("client_presence_ping_interval")) * time.Second
	cfg.ClientPresenceExpireInterval = time.Duration(v.GetInt("client_presence_expire_interval")) * time.Second
	cfg.ClientPingInterval = time.Duration(v.GetInt("client_ping_interval")) * time.Second
	cfg.ClientMessageWriteTimeout = time.Duration(v.GetInt("client_message_write_timeout")) * time.Second
	cfg.ClientInsecure = v.GetBool("client_insecure")
	cfg.ClientExpire = v.GetBool("client_expire")
	cfg.ClientExpiredCloseDelay = time.Duration(v.GetInt("client_expired_close_delay")) * time.Second
	cfg.ClientStaleCloseDelay = time.Duration(v.GetInt("client_stale_close_delay")) * time.Second
	cfg.ClientRequestMaxSize = v.GetInt("client_request_max_size")
	cfg.ClientQueueMaxSize = v.GetInt("client_queue_max_size")
	cfg.ClientChannelLimit = v.GetInt("client_channel_limit")
	cfg.ClientUserConnectionLimit = v.GetInt("client_user_connection_limit")

	return cfg
}

// applicationName returns a name for this centrifuge. If no name provided
// in configuration then it constructs node name based on hostname and port
func applicationName() string {
	v := viper.GetViper()

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
func namespacesFromConfig(v *viper.Viper) []centrifuge.ChannelNamespace {
	ns := []centrifuge.ChannelNamespace{}
	if !v.IsSet("namespaces") {
		return ns
	}
	v.UnmarshalKey("namespaces", &ns)
	return ns
}

func websocketHandlerConfig() centrifuge.WebsocketConfig {
	v := viper.GetViper()
	cfg := centrifuge.WebsocketConfig{}
	cfg.Compression = v.GetBool("websocket_compression")
	cfg.CompressionLevel = v.GetInt("websocket_compression_level")
	cfg.CompressionMinSize = v.GetInt("websocket_compression_min_size")
	cfg.ReadBufferSize = v.GetInt("websocket_read_buffer_size")
	cfg.WriteBufferSize = v.GetInt("websocket_write_buffer_size")
	return cfg
}

func sockjsHandlerConfig() centrifuge.SockjsConfig {
	v := viper.GetViper()
	cfg := centrifuge.SockjsConfig{}
	cfg.URL = v.GetString("sockjs_url")
	cfg.HeartbeatDelay = time.Duration(v.GetInt("sockjs_heartbeat_delay")) * time.Second
	cfg.WebsocketReadBufferSize = v.GetInt("websocket_read_buffer_size")
	cfg.WebsocketWriteBufferSize = v.GetInt("websocket_write_buffer_size")
	return cfg
}

func adminHandlerConfig() admin.Config {
	v := viper.GetViper()
	cfg := admin.Config{}
	cfg.WebFS = webui.FS
	cfg.WebPath = v.GetString("admin_web_path")
	cfg.Password = v.GetString("admin_password")
	cfg.Secret = v.GetString("admin_secret")
	cfg.Insecure = v.GetBool("admin_insecure")
	return cfg
}

func memoryEngine(n *centrifuge.Node) (centrifuge.Engine, error) {
	c, err := memoryEngineConfig()
	if err != nil {
		return nil, err
	}
	return centrifuge.NewMemoryEngine(n, *c)
}

func redisEngine(n *centrifuge.Node) (centrifuge.Engine, error) {
	c, err := redisEngineConfig()
	if err != nil {
		return nil, err
	}
	return centrifuge.NewRedisEngine(n, *c)
}

func memoryEngineConfig() (*centrifuge.MemoryEngineConfig, error) {
	return &centrifuge.MemoryEngineConfig{}, nil
}

func redisEngineConfig() (*centrifuge.RedisEngineConfig, error) {
	v := viper.GetViper()

	numShards := 1

	hostsConf := v.GetString("redis_host")
	portsConf := v.GetString("redis_port")
	urlsConf := v.GetString("redis_url")
	masterNamesConf := v.GetString("redis_master_name")
	sentinelsConf := v.GetString("redis_sentinels")

	password := v.GetString("redis_password")
	db := v.GetString("redis_db")

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

	var shardConfigs []*centrifuge.RedisShardConfig

	for i := 0; i < numShards; i++ {
		conf := &centrifuge.RedisShardConfig{
			Host:             hosts[i],
			Port:             ports[i],
			Password:         passwords[i],
			DB:               dbs[i],
			MasterName:       masterNames[i],
			SentinelAddrs:    sentinelAddrs,
			PoolSize:         v.GetInt("redis_pool"),
			Prefix:           v.GetString("redis_prefix"),
			PubSubNumWorkers: v.GetInt("redis_pubsub_num_workers"),
			ConnectTimeout:   time.Duration(v.GetInt("redis_connect_timeout")) * time.Second,
			ReadTimeout:      time.Duration(v.GetInt("redis_read_timeout")) * time.Second,
			WriteTimeout:     time.Duration(v.GetInt("redis_write_timeout")) * time.Second,
		}
		shardConfigs = append(shardConfigs, conf)
	}

	return &centrifuge.RedisEngineConfig{
		Shards: shardConfigs,
	}, nil
}

func setLogHandler(n *centrifuge.Node) {
	v := viper.GetViper()
	level, ok := centrifuge.LogStringToLevel[strings.ToLower(v.GetString("log_level"))]
	if !ok {
		level = centrifuge.LogLevelInfo
	}
	handler := newLogHandler()
	n.SetLogHandler(level, handler.handle)
}

type logHandler struct {
	entries chan centrifuge.LogEntry
	handler func(entry centrifuge.LogEntry)
}

func newLogHandler() *logHandler {
	h := &logHandler{
		entries: make(chan centrifuge.LogEntry, 64),
	}
	go h.readEntries()
	return h
}

func (h *logHandler) readEntries() {
	for entry := range h.entries {
		var log *logger.LevelLogger
		switch entry.Level {
		case centrifuge.LogLevelDebug:
			log = logger.DEBUG
		case centrifuge.LogLevelInfo:
			log = logger.INFO
		case centrifuge.LogLevelError:
			log = logger.ERROR
		}
		if entry.Fields != nil {
			log.Printf("%s: %v", entry.Message, entry.Fields)
		} else {
			log.Println(entry.Message)
		}
	}
}

func (h *logHandler) handle(entry centrifuge.LogEntry) {
	select {
	case h.entries <- entry:
	default:
		return
	}
}

// HandlerFlag is a bit mask of handlers that must be enabled in mux.
type HandlerFlag int

const (
	// HandlerWebsocket enables Raw Websocket handler.
	HandlerWebsocket HandlerFlag = 1 << iota
	// HandlerSockJS enables SockJS handler.
	HandlerSockJS
	// HandlerAPI enables API handler.
	HandlerAPI
	// HandlerAdmin enables admin web interface.
	HandlerAdmin
	// HandlerDebug enables debug handlers.
	HandlerDebug
	// HandlerPrometheus enables Prometheus handler.
	HandlerPrometheus
)

var handlerText = map[HandlerFlag]string{
	HandlerWebsocket:  "websocket",
	HandlerSockJS:     "SockJS",
	HandlerAPI:        "API",
	HandlerAdmin:      "admin",
	HandlerDebug:      "debug",
	HandlerPrometheus: "prometheus",
}

func (flags HandlerFlag) String() string {
	flagsOrdered := []HandlerFlag{HandlerWebsocket, HandlerSockJS, HandlerAPI, HandlerAdmin, HandlerPrometheus, HandlerDebug}
	endpoints := []string{}
	for _, flag := range flagsOrdered {
		text, ok := handlerText[flag]
		if !ok {
			continue
		}
		if flags&flag != 0 {
			endpoints = append(endpoints, text)
		}
	}
	return strings.Join(endpoints, ", ")
}

// Mux returns a mux including set of default handlers for Centrifugo server.
func Mux(n *centrifuge.Node, flags HandlerFlag) *http.ServeMux {

	mux := http.NewServeMux()

	if flags&HandlerDebug != 0 {
		mux.Handle("/debug/pprof/", http.HandlerFunc(pprof.Index))
		mux.Handle("/debug/pprof/cmdline", http.HandlerFunc(pprof.Cmdline))
		mux.Handle("/debug/pprof/profile", http.HandlerFunc(pprof.Profile))
		mux.Handle("/debug/pprof/symbol", http.HandlerFunc(pprof.Symbol))
		mux.Handle("/debug/pprof/trace", http.HandlerFunc(pprof.Trace))
	}

	if flags&HandlerWebsocket != 0 {
		// register Websocket connection endpoint.
		mux.Handle("/connection/websocket", middleware.LogRequest(n, centrifuge.NewWebsocketHandler(n, websocketHandlerConfig())))
	}

	if flags&HandlerSockJS != 0 {
		// register SockJS connection endpoints.
		sockjsConfig := sockjsHandlerConfig()
		sockjsConfig.HandlerPrefix = "/connection/sockjs"
		mux.Handle(sockjsConfig.HandlerPrefix+"/", middleware.LogRequest(n, centrifuge.NewSockjsHandler(n, sockjsConfig)))
	}

	if flags&HandlerAPI != 0 {
		// register HTTP API endpoint.
		apiHandler := centrifuge.NewAPIHandler(n, centrifuge.APIConfig{})
		if viper.GetBool("api_insecure") {
			mux.Handle("/api", middleware.LogRequest(n, apiHandler))
		} else {
			mux.Handle("/api", middleware.LogRequest(n, middleware.APIKeyAuth(viper.GetString("api_key"), apiHandler)))
		}
	}

	if flags&HandlerPrometheus != 0 {
		// register Prometheus metrics export endpoint.
		mux.Handle("/metrics", middleware.LogRequest(n, promhttp.Handler()))
	}

	if flags&HandlerAdmin != 0 {
		// register admin web interface API endpoints.
		mux.Handle("/", middleware.LogRequest(n, admin.NewHandler(n, adminHandlerConfig())))
	}

	return mux
}
