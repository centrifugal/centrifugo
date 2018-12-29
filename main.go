package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"html/template"
	"io/ioutil"
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
	"github.com/centrifugal/centrifugo/internal/api"
	"github.com/centrifugal/centrifugo/internal/health"
	"github.com/centrifugal/centrifugo/internal/metrics/graphite"
	"github.com/centrifugal/centrifugo/internal/middleware"
	"github.com/centrifugal/centrifugo/internal/webui"

	"github.com/FZambia/viper-lite"
	"github.com/centrifugal/centrifuge"
	"github.com/mattn/go-isatty"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
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
				"engine", "debug", "secret", "publish", "subscribe_to_publish", "anonymous",
				"join_leave", "presence", "history_recover", "history_size", "history_lifetime",
				"client_insecure", "api_key", "api_insecure", "admin", "admin_password", "admin_secret",
				"admin_insecure", "redis_host", "redis_port", "redis_url", "redis_tls", "redis_tls_skip_verify",
				"port", "internal_port", "tls", "tls_cert", "tls_key",
			}
			for _, env := range bindEnvs {
				viper.BindEnv(env)
			}

			bindPFlags := []string{
				"engine", "log_level", "log_file", "pid_file", "debug", "name", "admin",
				"client_insecure", "admin_insecure", "api_insecure", "port",
				"address", "tls", "tls_cert", "tls_key", "internal_port", "prometheus", "health",
				"redis_host", "redis_port", "redis_password", "redis_db", "redis_url",
				"redis_tls", "redis_tls_skip_verify", "redis_master_name", "redis_sentinels", "grpc_api",
			}
			for _, flag := range bindPFlags {
				viper.BindPFlag(flag, cmd.Flags().Lookup(flag))
			}

			viper.SetConfigFile(configFile)

			absConfPath, err := filepath.Abs(configFile)
			if err != nil {
				log.Fatal().Msgf("Error retrieving config file absolute path: %v", err)
			}

			err = viper.ReadInConfig()
			configFound := true
			if err != nil {
				switch err.(type) {
				case viper.ConfigParseError:
					log.Fatal().Msgf("Error parsing configuration: %s\n", err)
				default:
					configFound = false
				}
			}

			file := setupLogging()
			if file != nil {
				defer file.Close()
			}

			err = writePidFile(viper.GetString("pid_file"))
			if err != nil {
				log.Fatal().Msgf("Error writing PID: %v", err)
			}

			if !configFound {
				log.Warn().Msg("No config file found")
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
				log.Fatal().Msgf("Error validating config: %v", err)
			}

			node, err := centrifuge.New(*c)
			if err != nil {
				log.Fatal().Msgf("Error creating Centrifuge Node: %v", err)
			}
			setLogHandler(node)

			engineName := viper.GetString("engine")

			var e centrifuge.Engine
			if engineName == "memory" {
				e, err = memoryEngine(node)
			} else if engineName == "redis" {
				e, err = redisEngine(node)
			} else {
				log.Fatal().Msgf("Unknown engine: %s", engineName)
			}

			if err != nil {
				log.Fatal().Msgf("Error creating engine: %v", err)
			}

			node.SetEngine(e)

			if err = node.Run(); err != nil {
				log.Fatal().Msgf("Error running node: %v", err)
			}

			version := VERSION
			if version == "" {
				version = "dev"
			}
			log.Info().Msgf("starting Centrifugo %s (%s)", version, runtime.Version())
			log.Info().Msgf("config path: %s", absConfPath)
			log.Info().Msgf("pid: %d", os.Getpid())
			log.Info().Msgf("engine: %s", strings.Title(engineName))
			log.Info().Msgf("gomaxprocs: %d", runtime.GOMAXPROCS(0))
			if viper.GetBool("client_insecure") {
				log.Warn().Msg("INSECURE client mode enabled")
			}
			if viper.GetBool("api_insecure") {
				log.Warn().Msg("INSECURE API mode enabled")
			}
			if viper.GetBool("admin_insecure") {
				log.Warn().Msg("INSECURE admin mode enabled")
			}
			if viper.GetBool("debug") {
				log.Warn().Msg("DEBUG mode enabled, see /debug/pprof")
			}

			var grpcAPIServer *grpc.Server
			var grpcAPIAddr string
			if viper.GetBool("grpc_api") {
				grpcAPIAddr = fmt.Sprintf(":%d", viper.GetInt("grpc_api_port"))
				grpcAPIConn, err := net.Listen("tcp", grpcAPIAddr)
				if err != nil {
					log.Fatal().Msgf("cannot listen to address %s", grpcAPIAddr)
				}
				grpcOpts := []grpc.ServerOption{}
				tlsConfig, err := getTLSConfig()
				if err != nil {
					log.Fatal().Msgf("error getting TLS config: %v", err)
				}
				if tlsConfig != nil {
					grpcOpts = append(grpcOpts, grpc.Creds(credentials.NewTLS(tlsConfig)))
				}
				grpcAPIServer = grpc.NewServer(grpcOpts...)
				api.RegisterGRPCServerAPI(node, grpcAPIServer, api.GRPCAPIServiceConfig{})
				go func() {
					if err := grpcAPIServer.Serve(grpcAPIConn); err != nil {
						log.Fatal().Msgf("serve GRPC: %v", err)
					}
				}()
			}

			if grpcAPIServer != nil {
				log.Info().Msgf("serving GRPC API service on %s", grpcAPIAddr)
			}

			servers, err := runHTTPServers(node)
			if err != nil {
				log.Fatal().Msgf("error running HTTP server: %v", err)
			}

			var exporter *graphite.Exporter
			if viper.GetBool("graphite") {
				exporter = graphite.New(graphite.Config{
					Address:  net.JoinHostPort(viper.GetString("graphite_host"), strconv.Itoa(viper.GetInt("graphite_port"))),
					Gatherer: prometheus.DefaultGatherer,
					Prefix:   strings.TrimSuffix(viper.GetString("graphite_prefix"), ".") + "." + graphite.PreparePathComponent(c.Name),
					Interval: time.Duration(viper.GetInt("graphite_interval")) * time.Second,
					Tags:     viper.GetBool("graphite_tags"),
				})
			}

			handleSignals(node, servers, grpcAPIServer, exporter)
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
	rootCmd.Flags().BoolP("health", "", false, "enable Health endpoint")

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

	rootCmd.Flags().StringP("redis_host", "", "127.0.0.1", "Redis host (Redis engine)")
	rootCmd.Flags().StringP("redis_port", "", "6379", "Redis port (Redis engine)")
	rootCmd.Flags().StringP("redis_password", "", "", "Redis auth password (Redis engine)")
	rootCmd.Flags().IntP("redis_db", "", 0, "Redis database (Redis engine)")
	rootCmd.Flags().StringP("redis_url", "", "", "Redis connection URL in format redis://:password@hostname:port/db (Redis engine)")
	rootCmd.Flags().BoolP("redis_tls", "", false, "enable Redis TLS connection")
	rootCmd.Flags().BoolP("redis_tls_skip_verify", "", false, "disable Redis TLS host verification")
	rootCmd.Flags().StringP("redis_master_name", "", "", "name of Redis master Sentinel monitors (Redis engine)")
	rootCmd.Flags().StringP("redis_sentinels", "", "", "comma-separated list of Sentinel addresses (Redis engine)")

	viper.SetEnvPrefix("centrifugo")

	var versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Centrifugo version information",
		Long:  `Print the version information of Centrifugo`,
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
				log.Fatal().Msgf("%v", err)
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
				log.Fatal().Msgf("%v", err)
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
	"gomaxprocs":                           0,
	"engine":                               "memory",
	"name":                                 "",
	"secret":                               "",
	"publish":                              false,
	"subscribe_to_publish":                 false,
	"anonymous":                            false,
	"presence":                             false,
	"history_size":                         0,
	"history_lifetime":                     0,
	"history_recover":                      false,
	"namespaces":                           "",
	"node_info_metrics_aggregate_interval": 60,
	"client_ping_interval":                 25,
	"client_expired_close_delay":           25,
	"client_expired_sub_close_delay":       25,
	"client_stale_close_delay":             25,
	"client_message_write_timeout":         0,
	"client_channel_limit":                 128,
	"client_request_max_size":              65536,    // 64KB
	"client_queue_max_size":                10485760, // 10MB
	"client_presence_ping_interval":        25,
	"client_presence_expire_interval":      60,
	"client_user_connection_limit":         0,
	"channel_max_length":                   255,
	"channel_private_prefix":               "$",
	"channel_namespace_boundary":           ":",
	"channel_user_boundary":                "#",
	"channel_user_separator":               ",",
	"debug":                                false,
	"prometheus":                           false,
	"health":                               false,
	"admin":                                false,
	"admin_password":                       "",
	"admin_secret":                         "",
	"admin_insecure":                       false,
	"admin_web_path":                       "",
	"sockjs_url":                           "https://cdn.jsdelivr.net/npm/sockjs-client@1.3/dist/sockjs.min.js",
	"sockjs_heartbeat_delay":               25,
	"websocket_compression":                false,
	"websocket_compression_min_size":       0,
	"websocket_compression_level":          1,
	"websocket_read_buffer_size":           0,
	"websocket_write_buffer_size":          0,
	"tls_autocert":                         false,
	"tls_autocert_host_whitelist":          "",
	"tls_autocert_cache_dir":               "",
	"tls_autocert_email":                   "",
	"tls_autocert_force_rsa":               false,
	"tls_autocert_server_name":             "",
	"tls_autocert_http":                    false,
	"tls_autocert_http_addr":               ":80",
	"redis_prefix":                         "centrifugo",
	"redis_connect_timeout":                1,
	"redis_read_timeout":                   5,
	"redis_write_timeout":                  1,
	"redis_idle_timeout":                   0,
	"redis_pubsub_num_workers":             0,
	"grpc_api":                             false,
	"grpc_api_port":                        10000,
	"shutdown_timeout":                     30,
	"shutdown_termination_delay":           1,
	"graphite":                             false,
	"graphite_host":                        "localhost",
	"graphite_port":                        2003,
	"graphite_prefix":                      "centrifugo",
	"graphite_interval":                    10,
	"graphite_tags":                        false,
}

func writePidFile(pidFile string) error {
	if pidFile == "" {
		return nil
	}
	pid := []byte(strconv.Itoa(os.Getpid()) + "\n")
	return ioutil.WriteFile(pidFile, pid, 0644)
}

var logLevelMatches = map[string]zerolog.Level{
	"NONE":  zerolog.NoLevel,
	"DEBUG": zerolog.DebugLevel,
	"INFO":  zerolog.InfoLevel,
	"WARN":  zerolog.WarnLevel,
	"ERROR": zerolog.ErrorLevel,
	"FATAL": zerolog.FatalLevel,
}

func setupLogging() *os.File {
	if isatty.IsTerminal(os.Stdout.Fd()) && runtime.GOOS != "windows" {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout})
	}

	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	logLevel, ok := logLevelMatches[strings.ToUpper(viper.GetString("log_level"))]
	if !ok {
		logLevel = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(logLevel)

	if viper.IsSet("log_file") && viper.GetString("log_file") != "" {
		f, err := os.OpenFile(viper.GetString("log_file"), os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		if err != nil {
			log.Fatal().Msgf("error opening log file: %v", err)
		}
		log.Logger = log.Output(f)
		return f
	}

	return nil
}

func handleSignals(n *centrifuge.Node, httpServers []*http.Server, grpcAPIServer *grpc.Server, exporter *graphite.Exporter) {
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGHUP, syscall.SIGINT, os.Interrupt, syscall.SIGTERM)
	for {
		sig := <-sigc
		log.Info().Msgf("signal received: %v", sig)
		switch sig {
		case syscall.SIGHUP:
			// reload application configuration on SIGHUP.
			log.Info().Msg("reloading configuration")
			err := viper.ReadInConfig()
			if err != nil {
				switch err.(type) {
				case viper.ConfigParseError:
					log.Error().Msgf("error parsing configuration: %s", err)
					continue
				default:
					log.Error().Msg("no config file found")
					continue
				}
			}
			cfg := nodeConfig()
			if err := n.Reload(*cfg); err != nil {
				log.Error().Msgf("error reloading: %v", err)
				continue
			}
			log.Info().Msg("configuration successfully reloaded")
		case syscall.SIGINT, os.Interrupt, syscall.SIGTERM:
			log.Info().Msg("shutting down, wait...")
			pidFile := viper.GetString("pid_file")
			shutdownTimeout := time.Duration(viper.GetInt("shutdown_timeout")) * time.Second
			go time.AfterFunc(shutdownTimeout, func() {
				if pidFile != "" {
					os.Remove(pidFile)
				}
				os.Exit(1)
			})

			if exporter != nil {
				exporter.Close()
			}

			var wg sync.WaitGroup

			if grpcAPIServer != nil {
				wg.Add(1)
				go func() {
					defer wg.Done()
					grpcAPIServer.GracefulStop()
				}()
			}

			ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
			defer cancel()

			for _, srv := range httpServers {
				wg.Add(1)
				go func(srv *http.Server) {
					defer wg.Done()
					srv.Shutdown(ctx)
				}(srv)
			}

			n.Shutdown(ctx)

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
	tlsAutocertHTTP := viper.GetBool("tls_autocert_http")
	tlsAutocertHTTPAddr := viper.GetString("tls_autocert_http_addr")

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

		if tlsAutocertHTTP {
			acmeHTTPserver := &http.Server{
				Handler: certManager.HTTPHandler(nil),
				Addr:    tlsAutocertHTTPAddr,
			}
			go func() {
				log.Info().Msgf("serving ACME http_01 challenge on %s", tlsAutocertHTTPAddr)
				if err := acmeHTTPserver.ListenAndServe(); err != nil {
					log.Fatal().Msgf("can't create server on %s to serve acme http challenge: %v", tlsAutocertHTTPAddr, err)
				}
			}()
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
	health := viper.GetBool("health")

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
	if health {
		portFlags |= HandlerHealth
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

		log.Info().Msgf("serving %s endpoints on %s", handlerFlags, addr)

		tlsConfig, err := getTLSConfig()
		if err != nil {
			log.Fatal().Msgf("can not get TLS config: %v", err)
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
						log.Fatal().Msgf("ListenAndServe: %v", err)
					}
				}
			} else {
				if err := server.ListenAndServe(); err != nil {
					if err != http.ErrServerClosed {
						log.Fatal().Msgf("ListenAndServe: %v", err)
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
  "secret": "{{.Secret}}",
  "admin_password": "{{.AdminPassword}}",
  "admin_secret": "{{.AdminSecret}}",
  "api_key": "{{.APIKey}}"
}
`

var tomlConfigTemplate = `secret = {{.Secret}}
admin_password = {{.AdminPassword}}
admin_secret = {{.AdminSecret}}
api_key = {{.APIKey}}
`

var yamlConfigTemplate = `secret: {{.Secret}}
admin_password: {{.AdminPassword}}
admin_secret: {{.AdminSecret}}
api_key: {{.APIKey}}
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
		Secret        string
		AdminPassword string
		AdminSecret   string
		APIKey        string
	}{
		uuid.NewV4().String(),
		uuid.NewV4().String(),
		uuid.NewV4().String(),
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
	viper.SetConfigFile(f)
	err := viper.ReadInConfig()
	if err != nil {
		switch err.(type) {
		case viper.ConfigParseError:
			return err
		default:
			return errors.New("unable to locate config file, use \"centrifugo genconfig -c " + f + "\" command to generate one")
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
	cfg.SubscribeToPublish = v.GetBool("subscribe_to_publish")
	cfg.Anonymous = v.GetBool("anonymous")
	cfg.Presence = v.GetBool("presence")
	cfg.JoinLeave = v.GetBool("join_leave")
	cfg.HistorySize = v.GetInt("history_size")
	cfg.HistoryLifetime = v.GetInt("history_lifetime")
	cfg.HistoryRecover = v.GetBool("history_recover")
	cfg.Namespaces = namespacesFromConfig(v)

	cfg.ChannelMaxLength = v.GetInt("channel_max_length")
	cfg.ChannelPrivatePrefix = v.GetString("channel_private_prefix")
	cfg.ChannelNamespaceBoundary = v.GetString("channel_namespace_boundary")
	cfg.ChannelUserBoundary = v.GetString("channel_user_boundary")
	cfg.ChannelUserSeparator = v.GetString("channel_user_separator")

	cfg.ClientPresencePingInterval = time.Duration(v.GetInt("client_presence_ping_interval")) * time.Second
	cfg.ClientPresenceExpireInterval = time.Duration(v.GetInt("client_presence_expire_interval")) * time.Second
	cfg.ClientPingInterval = time.Duration(v.GetInt("client_ping_interval")) * time.Second
	cfg.ClientMessageWriteTimeout = time.Duration(v.GetInt("client_message_write_timeout")) * time.Second
	cfg.ClientInsecure = v.GetBool("client_insecure")
	cfg.ClientExpiredCloseDelay = time.Duration(v.GetInt("client_expired_close_delay")) * time.Second
	cfg.ClientExpiredSubCloseDelay = time.Duration(v.GetInt("client_expired_sub_close_delay")) * time.Second
	cfg.ClientStaleCloseDelay = time.Duration(v.GetInt("client_stale_close_delay")) * time.Second
	cfg.ClientRequestMaxSize = v.GetInt("client_request_max_size")
	cfg.ClientQueueMaxSize = v.GetInt("client_queue_max_size")
	cfg.ClientChannelLimit = v.GetInt("client_channel_limit")
	cfg.ClientUserConnectionLimit = v.GetInt("client_user_connection_limit")

	cfg.NodeInfoMetricsAggregateInterval = time.Duration(v.GetInt("node_info_metrics_aggregate_interval")) * time.Second

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
	redisTLS := v.GetBool("redis_tls")
	redisTLSSkipVerify := v.GetBool("redis_tls_skip_verify")
	masterNamesConf := v.GetString("redis_master_name")
	sentinelsConf := v.GetString("redis_sentinels")

	password := v.GetString("redis_password")
	db := v.GetInt("redis_db")

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
		return nil, fmt.Errorf("provide at least one Sentinel address")
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
				return nil, fmt.Errorf("malformed Sentinel address: %s", addr)
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
		return nil, fmt.Errorf("malformed sharding configuration: wrong number of redis hosts")
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
		return nil, fmt.Errorf("malformed sharding configuration: wrong number of redis ports")
	}

	if len(urls) > 0 && len(urls) != numShards {
		return nil, fmt.Errorf("malformed sharding configuration: wrong number of redis urls")
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
		dbs[i] = db
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

	var shardConfigs []centrifuge.RedisShardConfig

	for i := 0; i < numShards; i++ {
		port, err := strconv.Atoi(ports[i])
		if err != nil {
			return nil, fmt.Errorf("malformed port: %v", err)
		}
		conf := centrifuge.RedisShardConfig{
			Host:             hosts[i],
			Port:             port,
			Password:         passwords[i],
			DB:               dbs[i],
			UseTLS:           redisTLS,
			TLSSkipVerify:    redisTLSSkipVerify,
			MasterName:       masterNames[i],
			SentinelAddrs:    sentinelAddrs,
			Prefix:           v.GetString("redis_prefix"),
			IdleTimeout:      time.Duration(v.GetInt("redis_idle_timeout")) * time.Second,
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
		var l *zerolog.Event
		switch entry.Level {
		case centrifuge.LogLevelDebug:
			l = log.Debug()
		case centrifuge.LogLevelInfo:
			l = log.Info()
		case centrifuge.LogLevelError:
			l = log.Error()
		}
		if entry.Fields != nil {
			l.Fields(entry.Fields).Msg(entry.Message)
		} else {
			l.Msg(entry.Message)
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
	// HandlerHealth enables Health check endpoint.
	HandlerHealth
)

var handlerText = map[HandlerFlag]string{
	HandlerWebsocket:  "websocket",
	HandlerSockJS:     "SockJS",
	HandlerAPI:        "API",
	HandlerAdmin:      "admin",
	HandlerDebug:      "debug",
	HandlerPrometheus: "prometheus",
	HandlerHealth:     "health",
}

func (flags HandlerFlag) String() string {
	flagsOrdered := []HandlerFlag{HandlerWebsocket, HandlerSockJS, HandlerAPI, HandlerAdmin, HandlerPrometheus, HandlerDebug, HandlerHealth}
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
		mux.Handle("/debug/pprof/", middleware.LogRequest(http.HandlerFunc(pprof.Index)))
		mux.Handle("/debug/pprof/cmdline", middleware.LogRequest(http.HandlerFunc(pprof.Cmdline)))
		mux.Handle("/debug/pprof/profile", middleware.LogRequest(http.HandlerFunc(pprof.Profile)))
		mux.Handle("/debug/pprof/symbol", middleware.LogRequest(http.HandlerFunc(pprof.Symbol)))
		mux.Handle("/debug/pprof/trace", middleware.LogRequest(http.HandlerFunc(pprof.Trace)))
	}

	if flags&HandlerWebsocket != 0 {
		// register Websocket connection endpoint.
		mux.Handle("/connection/websocket", middleware.LogRequest(centrifuge.NewWebsocketHandler(n, websocketHandlerConfig())))
	}

	if flags&HandlerSockJS != 0 {
		// register SockJS connection endpoints.
		sockjsConfig := sockjsHandlerConfig()
		sockjsConfig.HandlerPrefix = "/connection/sockjs"
		mux.Handle(sockjsConfig.HandlerPrefix+"/", middleware.LogRequest(centrifuge.NewSockjsHandler(n, sockjsConfig)))
	}

	if flags&HandlerAPI != 0 {
		// register HTTP API endpoint.
		apiHandler := api.NewHandler(n, api.Config{})
		if viper.GetBool("api_insecure") {
			mux.Handle("/api", middleware.LogRequest(apiHandler))
		} else {
			mux.Handle("/api", middleware.LogRequest(middleware.APIKeyAuth(viper.GetString("api_key"), apiHandler)))
		}
	}

	if flags&HandlerPrometheus != 0 {
		// register Prometheus metrics export endpoint.
		mux.Handle("/metrics", middleware.LogRequest(promhttp.Handler()))
	}

	if flags&HandlerAdmin != 0 {
		// register admin web interface API endpoints.
		mux.Handle("/", middleware.LogRequest(admin.NewHandler(n, adminHandlerConfig())))
	} else {
		mux.Handle("/", middleware.LogRequest(http.HandlerFunc(notFoundHandler)))
	}

	if flags&HandlerHealth != 0 {
		mux.Handle("/health", middleware.LogRequest(health.NewHandler(n, health.Config{})))
	}

	return mux
}

func notFoundHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
	w.Write([]byte("404 page not found"))
}
