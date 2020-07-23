package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	stdlog "log"
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
	"github.com/centrifugal/centrifugo/internal/client"
	"github.com/centrifugal/centrifugo/internal/health"
	"github.com/centrifugal/centrifugo/internal/jwtutils"
	"github.com/centrifugal/centrifugo/internal/jwtverify"
	"github.com/centrifugal/centrifugo/internal/logutils"
	"github.com/centrifugal/centrifugo/internal/metrics/graphite"
	"github.com/centrifugal/centrifugo/internal/middleware"
	"github.com/centrifugal/centrifugo/internal/natsbroker"
	"github.com/centrifugal/centrifugo/internal/proxy"
	"github.com/centrifugal/centrifugo/internal/rule"
	"github.com/centrifugal/centrifugo/internal/tools"
	"github.com/centrifugal/centrifugo/internal/webui"

	"github.com/FZambia/viper-lite"
	"github.com/centrifugal/centrifuge"
	"github.com/mattn/go-isatty"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// VERSION of Centrifugo server. Set on build stage.
var VERSION string

var configDefaults = map[string]interface{}{
	"gomaxprocs":                           0,
	"engine":                               "memory",
	"broker":                               "",
	"name":                                 "",
	"secret":                               "",
	"token_hmac_secret_key":                "",
	"token_rsa_public_key":                 "",
	"server_side":                          false,
	"publish":                              false,
	"subscribe_to_publish":                 false,
	"anonymous":                            false,
	"presence":                             false,
	"presence_disable_for_client":          false,
	"history_size":                         0,
	"history_lifetime":                     0,
	"history_recover":                      false,
	"history_disable_for_client":           false,
	"proxy_subscribe":                      false,
	"proxy_publish":                        false,
	"node_info_metrics_aggregate_interval": 60,
	"client_anonymous":                     false,
	"client_expired_close_delay":           25,
	"client_expired_sub_close_delay":       25,
	"client_stale_close_delay":             25,
	"client_channel_limit":                 128,
	"client_queue_max_size":                10485760, // 10MB
	"client_presence_ping_interval":        25,
	"client_presence_expire_interval":      60,
	"client_user_connection_limit":         0,
	"client_channel_position_check_delay":  40,
	"channel_max_length":                   255,
	"channel_private_prefix":               "$",
	"channel_namespace_boundary":           ":",
	"channel_user_boundary":                "#",
	"channel_user_separator":               ",",
	"user_subscribe_to_personal":           false,
	"user_personal_channel_namespace":      "",
	"debug":                                false,
	"prometheus":                           false,
	"health":                               false,
	"admin":                                false,
	"admin_password":                       "",
	"admin_secret":                         "",
	"admin_insecure":                       false,
	"admin_web_path":                       "",
	"sockjs_url":                           "https://cdn.jsdelivr.net/npm/sockjs-client@1/dist/sockjs.min.js",
	"sockjs_heartbeat_delay":               25,
	"websocket_compression":                false,
	"websocket_compression_min_size":       0,
	"websocket_compression_level":          1,
	"websocket_read_buffer_size":           0,
	"websocket_use_write_buffer_pool":      false,
	"websocket_write_buffer_size":          0,
	"client_ping_interval":                 25, // TODO v3: remove.
	"websocket_ping_interval":              25,
	"client_message_write_timeout":         1, // TODO v3: remove.
	"websocket_write_timeout":              1,
	"client_request_max_size":              65536, // TODO v3: remove.
	"websocket_message_size_limit":         65536, // 64KB
	"tls_autocert":                         false,
	"tls_autocert_host_whitelist":          "",
	"tls_autocert_cache_dir":               "",
	"tls_autocert_email":                   "",
	"tls_autocert_force_rsa":               false, // TODO v3: remove.
	"tls_autocert_server_name":             "",
	"tls_autocert_http":                    false,
	"tls_autocert_http_addr":               ":80",
	"redis_prefix":                         "centrifugo",
	"redis_connect_timeout":                1, // TODO v3: make all timeouts float.
	"redis_read_timeout":                   5,
	"redis_write_timeout":                  1,
	"redis_idle_timeout":                   0,
	"redis_pubsub_num_workers":             0, // TODO v3: remove.
	"redis_sequence_ttl":                   0,
	"grpc_api":                             false,
	"grpc_api_port":                        10000,
	"shutdown_timeout":                     30,
	"shutdown_termination_delay":           0,
	"graphite":                             false,
	"graphite_host":                        "localhost",
	"graphite_port":                        2003,
	"graphite_prefix":                      "centrifugo",
	"graphite_interval":                    10,
	"graphite_tags":                        false,
	"nats_prefix":                          "centrifugo",
	"nats_url":                             "",
	"nats_dial_timeout":                    1,
	"nats_write_timeout":                   1,
	"websocket_disable":                    false,
	"sockjs_disable":                       false,
	"api_disable":                          false,
	"admin_handler_prefix":                 "",
	"websocket_handler_prefix":             "/connection/websocket",
	"sockjs_handler_prefix":                "/connection/sockjs",
	"api_handler_prefix":                   "/api",
	"prometheus_handler_prefix":            "/metrics",
	"health_handler_prefix":                "/health",
	"proxy_connect_endpoint":               "",
	"proxy_connect_timeout":                1,
	"proxy_rpc_endpoint":                   "",
	"proxy_rpc_timeout":                    1,
	"proxy_refresh_endpoint":               "",
	"proxy_refresh_timeout":                1,
	"memory_history_meta_ttl":              0,
	"redis_history_meta_ttl":               0,
	"v3_use_offset":                        false, // TODO v3: remove.
}

func main() {
	var configFile string

	viper.SetEnvPrefix("centrifugo")

	bindConfig := func() {
		for k, v := range configDefaults {
			viper.SetDefault(k, v)
		}

		bindEnvs := []string{
			"address", "admin", "admin_external", "admin_insecure", "admin_password",
			"admin_secret", "admin_web_path", "anonymous", "api_insecure", "api_key",
			"channel_max_length", "channel_namespace_boundary", "channel_private_prefix",
			"channel_user_boundary", "channel_user_separator", "client_anonymous",
			"client_channel_limit", "client_channel_position_check_delay",
			"client_expired_close_delay", "client_expired_sub_close_delay",
			"client_insecure", "client_message_write_timeout", "client_ping_interval",
			"client_presence_expire_interval", "client_presence_ping_interval",
			"client_queue_max_size", "client_request_max_size", "client_stale_close_delay",
			"debug", "engine", "graphite", "graphite_host", "graphite_interval",
			"graphite_port", "graphite_prefix", "graphite_tags", "grpc_api",
			"grpc_api_port", "health", "history_lifetime", "history_recover",
			"history_size", "internal_address", "internal_port", "join_leave", "log_file",
			"log_level", "name", "namespaces", "node_info_metrics_aggregate_interval",
			"pid_file", "port", "presence", "prometheus", "publish", "redis_connect_timeout",
			"redis_db", "redis_host", "redis_idle_timeout", "redis_master_name",
			"redis_password", "redis_port", "redis_prefix", "redis_pubsub_num_workers",
			"redis_read_timeout", "redis_sentinels", "redis_tls", "redis_tls_skip_verify",
			"redis_url", "redis_write_timeout", "secret", "shutdown_termination_delay",
			"shutdown_timeout", "sockjs_heartbeat_delay", "sockjs_url", "subscribe_to_publish",
			"tls", "tls_autocert", "tls_autocert_cache_dir", "tls_autocert_email",
			"tls_autocert_force_rsa", "tls_autocert_host_whitelist", "tls_autocert_http",
			"tls_autocert_http_addr", "tls_autocert_server_name", "tls_cert", "tls_external",
			"tls_key", "websocket_compression", "websocket_compression_level",
			"websocket_compression_min_size", "websocket_read_buffer_size",
			"websocket_write_buffer_size", "history_disable_for_client",
			"presence_disable_for_client", "admin_handler_prefix", "websocket_handler_prefix",
			"sockjs_handler_prefix", "api_handler_prefix", "prometheus_handler_prefix",
			"health_handler_prefix", "grpc_api_tls", "grpc_api_tls_disable",
			"grpc_api_tls_cert", "grpc_api_tls_key", "proxy_connect_endpoint",
			"proxy_connect_timeout", "proxy_rpc_endpoint", "proxy_rpc_timeout",
			"proxy_refresh_endpoint", "proxy_refresh_timeout",
			"token_rsa_public_key", "token_hmac_secret_key", "redis_sequence_ttl",
			"proxy_extra_http_headers", "server_side", "user_subscribe_to_personal",
			"user_personal_channel_namespace", "websocket_use_write_buffer_pool",
			"websocket_disable", "sockjs_disable", "api_disable", "redis_cluster_addrs",
			"broker", "nats_prefix", "nats_url", "nats_dial_timeout", "nats_write_timeout",
			"v3_use_offset", "redis_history_meta_ttl", "redis_streams", "memory_history_meta_ttl",
			"websocket_ping_interval", "websocket_write_timeout", "websocket_message_size_limit",
			"proxy_publish_endpoint", "proxy_publish_timeout", "proxy_subscribe_endpoint",
			"proxy_subscribe_timeout", "proxy_subscribe", "proxy_publish", "redis_sentinel_password",
			"grpc_api_key",
		}

		for _, env := range bindEnvs {
			_ = viper.BindEnv(env)
		}
	}

	var rootCmd = &cobra.Command{
		Use:   "",
		Short: "Centrifugo",
		Long:  "Centrifugo – scalable real-time messaging server in language-agnostic way",
		Run: func(cmd *cobra.Command, args []string) {
			bindConfig()

			bindPFlags := []string{
				"engine", "log_level", "log_file", "pid_file", "debug", "name", "admin",
				"admin_external", "client_insecure", "admin_insecure", "api_insecure",
				"port", "address", "tls", "tls_cert", "tls_key", "tls_external", "internal_port",
				"internal_address", "prometheus", "health", "redis_host", "redis_port",
				"redis_password", "redis_db", "redis_url", "redis_tls", "redis_tls_skip_verify",
				"redis_master_name", "redis_sentinels", "redis_sentinel_password", "grpc_api",
				"grpc_api_tls", "grpc_api_tls_disable", "grpc_api_tls_cert", "grpc_api_tls_key",
				"grpc_api_port", "broker", "nats_url",
			}
			for _, flag := range bindPFlags {
				_ = viper.BindPFlag(flag, cmd.Flags().Lookup(flag))
			}
			viper.SetConfigFile(configFile)

			absConfPath, err := filepath.Abs(configFile)
			if err != nil {
				log.Fatal().Msgf("error retrieving config file absolute path: %v", err)
			}

			err = viper.ReadInConfig()
			configFound := true
			if err != nil {
				switch err.(type) {
				case viper.ConfigParseError:
					log.Fatal().Msgf("error parsing configuration: %s\n", err)
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
				log.Fatal().Msgf("error writing PID: %v", err)
			}

			if os.Getenv("GOMAXPROCS") == "" {
				if viper.IsSet("gomaxprocs") && viper.GetInt("gomaxprocs") > 0 {
					runtime.GOMAXPROCS(viper.GetInt("gomaxprocs"))
				} else {
					runtime.GOMAXPROCS(runtime.NumCPU())
				}
			}

			version := VERSION
			if version == "" {
				version = "dev"
			}

			engineName := viper.GetString("engine")

			log.Info().Str(
				"version", version).Str(
				"runtime", runtime.Version()).Int(
				"pid", os.Getpid()).Str(
				"engine", strings.Title(engineName)).Int(
				"gomaxprocs", runtime.GOMAXPROCS(0)).Msg("starting Centrifugo")

			log.Info().Str("path", absConfPath).Msg("using config file")

			proxyConfig, _ := proxyConfig()

			ruleConfig := ruleConfig()
			err = ruleConfig.Validate()
			if err != nil {
				log.Fatal().Msgf("error validating config: %v", err)
			}
			ruleContainer := rule.NewNamespaceRuleContainer(ruleConfig)

			nodeConfig := nodeConfig(VERSION, ruleContainer.ChannelOptions)

			if !viper.GetBool("v3_use_offset") {
				log.Warn().Msgf("consider migrating to offset protocol field, details: https://github.com/centrifugal/centrifugo/releases/tag/v2.5.0")
				// TODO v3: remove compatibility flags.
				centrifuge.CompatibilityFlags |= centrifuge.UseSeqGen
			}

			node, err := centrifuge.New(nodeConfig)
			if err != nil {
				log.Fatal().Msgf("error creating Centrifuge Node: %v", err)
			}

			brokerName := viper.GetString("broker")

			var e centrifuge.Engine
			if engineName == "memory" {
				e, err = memoryEngine(node)
			} else if engineName == "redis" {
				e, err = redisEngine(node, brokerName == "")
			} else {
				log.Fatal().Msgf("unknown engine: %s", engineName)
			}
			if err != nil {
				log.Fatal().Msgf("error creating engine: %v", err)
			}

			tokenVerifier := jwtverify.NewTokenVerifierJWT(jwtVerifierConfig())

			clientHandler := client.NewHandler(node, ruleContainer, tokenVerifier, proxyConfig)
			clientHandler.Setup()

			node.SetEngine(e)

			var disableHistoryPresence bool
			if engineName == "memory" && brokerName == "nats" {
				// Presence and History won't work with Memory engine in distributed case.
				disableHistoryPresence = true
				node.SetHistoryManager(nil)
				node.SetPresenceManager(nil)
			}

			if disableHistoryPresence {
				log.Warn().Msgf("presence, history and recovery disabled with Memory engine and Nats broker")
			}

			if !configFound {
				log.Warn().Msg("config file not found")
			}

			if brokerName == "nats" {
				broker, err := natsbroker.New(node, natsbroker.Config{
					URL:          viper.GetString("nats_url"),
					Prefix:       viper.GetString("nats_prefix"),
					DialTimeout:  time.Duration(viper.GetInt("nats_dial_timeout")) * time.Second,
					WriteTimeout: time.Duration(viper.GetInt("nats_write_timeout")) * time.Second,
				})
				if err != nil {
					log.Fatal().Msgf("Error creating broker: %v", err)
				}
				node.SetBroker(broker)
			}

			if err = node.Run(); err != nil {
				log.Fatal().Msgf("error running node: %v", err)
			}

			if proxyConfig.ConnectEndpoint != "" {
				log.Info().Str("endpoint", proxyConfig.ConnectEndpoint).Msg("proxy connect over HTTP")
			}
			if proxyConfig.RefreshEndpoint != "" {
				log.Info().Str("endpoint", proxyConfig.RefreshEndpoint).Msg("proxy refresh over HTTP")
			}
			if proxyConfig.RPCEndpoint != "" {
				log.Info().Str("endpoint", proxyConfig.RPCEndpoint).Msg("proxy RPC over HTTP")
			}
			if proxyConfig.SubscribeEndpoint != "" {
				log.Info().Str("endpoint", proxyConfig.SubscribeEndpoint).Msg("proxy subscribe over HTTP")
			}
			if proxyConfig.PublishEndpoint != "" {
				log.Info().Str("endpoint", proxyConfig.PublishEndpoint).Msg("proxy publish over HTTP")
			}

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
				var grpcOpts []grpc.ServerOption
				var tlsConfig *tls.Config
				var tlsErr error

				if viper.GetString("grpc_api_key") != "" {
					grpcOpts = append(grpcOpts, api.GRPCKeyAuth(viper.GetString("grpc_api_key")))
				}
				if viper.GetBool("grpc_api_tls") {
					tlsConfig, tlsErr = tlsConfigForGRPC()
				} else if !viper.GetBool("grpc_api_tls_disable") {
					tlsConfig, tlsErr = getTLSConfig()
				}
				if tlsErr != nil {
					log.Fatal().Msgf("error getting TLS config: %v", tlsErr)
				}
				if tlsConfig != nil {
					grpcOpts = append(grpcOpts, grpc.Creds(credentials.NewTLS(tlsConfig)))
				}
				grpcAPIServer = grpc.NewServer(grpcOpts...)
				apiExecutor := api.NewExecutor(node, ruleContainer, "grpc")
				_ = api.RegisterGRPCServerAPI(node, apiExecutor, grpcAPIServer, api.GRPCAPIServiceConfig{})
				go func() {
					if err := grpcAPIServer.Serve(grpcAPIConn); err != nil {
						log.Fatal().Msgf("serve GRPC: %v", err)
					}
				}()
			}

			if grpcAPIServer != nil {
				log.Info().Msgf("serving GRPC API service on %s", grpcAPIAddr)
			}

			httpAPIExecutor := api.NewExecutor(node, ruleContainer, "http")
			servers, err := runHTTPServers(node, httpAPIExecutor)
			if err != nil {
				log.Fatal().Msgf("error running HTTP server: %v", err)
			}

			var exporter *graphite.Exporter
			if viper.GetBool("graphite") {
				exporter = graphite.New(graphite.Config{
					Address:  net.JoinHostPort(viper.GetString("graphite_host"), strconv.Itoa(viper.GetInt("graphite_port"))),
					Gatherer: prometheus.DefaultGatherer,
					Prefix:   strings.TrimSuffix(viper.GetString("graphite_prefix"), ".") + "." + graphite.PreparePathComponent(nodeConfig.Name),
					Interval: time.Duration(viper.GetInt("graphite_interval")) * time.Second,
					Tags:     viper.GetBool("graphite_tags"),
				})
			}

			handleSignals(configFile, node, ruleContainer, tokenVerifier, servers, grpcAPIServer, exporter)
		},
	}

	rootCmd.Flags().StringVarP(&configFile, "config", "c", "config.json", "path to config file")
	rootCmd.Flags().StringP("engine", "e", "memory", "engine to use: memory or redis")
	rootCmd.Flags().StringP("broker", "", "", "custom broker to use: ex. nats")
	rootCmd.Flags().StringP("log_level", "", "info", "set the log level: debug, info, error, fatal or none")
	rootCmd.Flags().StringP("log_file", "", "", "optional log file - if not specified logs go to STDOUT")
	rootCmd.Flags().StringP("pid_file", "", "", "optional path to create PID file")
	rootCmd.Flags().StringP("name", "n", "", "unique node name")

	rootCmd.Flags().BoolP("debug", "", false, "enable debug endpoints")
	rootCmd.Flags().BoolP("admin", "", false, "enable admin web interface")
	rootCmd.Flags().BoolP("admin_external", "", false, "enable admin web interface on external port")
	rootCmd.Flags().BoolP("prometheus", "", false, "enable Prometheus metrics endpoint")
	rootCmd.Flags().BoolP("health", "", false, "enable health check endpoint")

	rootCmd.Flags().BoolP("client_insecure", "", false, "start in insecure client mode")
	rootCmd.Flags().BoolP("api_insecure", "", false, "use insecure API mode")
	rootCmd.Flags().BoolP("admin_insecure", "", false, "use insecure admin mode – no auth required for admin socket")

	rootCmd.Flags().StringP("address", "a", "", "interface address to listen on")
	// TODO v3: make ports integer.
	rootCmd.Flags().StringP("port", "p", "8000", "port to bind HTTP server to")
	rootCmd.Flags().StringP("internal_address", "", "", "custom interface address to listen on for internal endpoints")
	rootCmd.Flags().StringP("internal_port", "", "", "custom port for internal endpoints")

	rootCmd.Flags().BoolP("tls", "", false, "enable TLS, requires an X509 certificate and a key file")
	rootCmd.Flags().StringP("tls_cert", "", "", "path to an X509 certificate file")
	rootCmd.Flags().StringP("tls_key", "", "", "path to an X509 certificate key")
	rootCmd.Flags().BoolP("tls_external", "", false, "enable TLS only for external endpoints")

	rootCmd.Flags().BoolP("grpc_api", "", false, "enable GRPC API server")
	rootCmd.Flags().IntP("grpc_api_port", "", 10000, "port to bind GRPC API server to")
	rootCmd.Flags().BoolP("grpc_api_tls", "", false, "enable TLS for GRPC API server, requires an X509 certificate and a key file")
	rootCmd.Flags().StringP("grpc_api_tls_cert", "", "", "path to an X509 certificate file for GRPC API server")
	rootCmd.Flags().StringP("grpc_api_tls_key", "", "", "path to an X509 certificate key for GRPC API server")
	rootCmd.Flags().BoolP("grpc_api_tls_disable", "", false, "disable general TLS for GRPC API server")

	rootCmd.Flags().StringP("redis_host", "", "127.0.0.1", "Redis host (Redis engine)")
	rootCmd.Flags().StringP("redis_port", "", "6379", "Redis port (Redis engine)")
	rootCmd.Flags().StringP("redis_password", "", "", "Redis auth password (Redis engine)")
	rootCmd.Flags().IntP("redis_db", "", 0, "Redis database (Redis engine)")
	rootCmd.Flags().StringP("redis_url", "", "", "Redis connection URL in format redis://:password@hostname:port/db (Redis engine)")
	rootCmd.Flags().BoolP("redis_tls", "", false, "enable Redis TLS connection")
	rootCmd.Flags().BoolP("redis_tls_skip_verify", "", false, "disable Redis TLS host verification")
	rootCmd.Flags().StringP("redis_master_name", "", "", "name of Redis master Sentinel monitors (Redis engine)")
	rootCmd.Flags().StringP("redis_sentinels", "", "", "comma-separated list of Sentinel addresses (Redis engine)")
	rootCmd.Flags().StringP("redis_sentinel_password", "", "", "Redis Sentinel auth password (Redis engine)")

	rootCmd.Flags().StringP("nats_url", "", "", "Nats connection URL in format nats://user:pass@localhost:4222 (Nats broker)")

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
			bindConfig()
			err := validateConfig(checkConfigFile)
			if err != nil {
				fmt.Printf("error: %v\n", err)
				os.Exit(1)
			}
		},
	}
	checkConfigCmd.Flags().StringVarP(&checkConfigFile, "config", "c", "config.json", "path to config file to check")

	var outputConfigFile string

	var genConfigCmd = &cobra.Command{
		Use:   "genconfig",
		Short: "Generate minimal configuration file to start with",
		Long:  `Generate minimal configuration file to start with`,
		Run: func(cmd *cobra.Command, args []string) {
			err := tools.GenerateConfig(outputConfigFile)
			if err != nil {
				fmt.Printf("error: %v\n", err)
				os.Exit(1)
			}
			err = validateConfig(outputConfigFile)
			if err != nil {
				_ = os.Remove(outputConfigFile)
				fmt.Printf("error: %v\n", err)
				os.Exit(1)
			}
		},
	}
	genConfigCmd.Flags().StringVarP(&outputConfigFile, "config", "c", "config.json", "path to output config file")

	var genTokenConfigFile string
	var genTokenUser string
	var genTokenTTL int64

	var genTokenCmd = &cobra.Command{
		Use:   "gentoken",
		Short: "Generate sample connection JWT for user",
		Long:  `Generate sample connection JWT for user`,
		Run: func(cmd *cobra.Command, args []string) {
			bindConfig()
			err := readConfig(genTokenConfigFile)
			if err != nil && err != errConfigFileNotFound {
				fmt.Printf("error: %v\n", err)
				os.Exit(1)
			}
			jwtVerifierConfig := jwtVerifierConfig()
			token, err := tools.GenerateToken(jwtVerifierConfig, genTokenUser, genTokenTTL)
			if err != nil {
				fmt.Printf("error: %v\n", err)
				os.Exit(1)
			}
			var user = fmt.Sprintf("user %s", genTokenUser)
			if genTokenUser == "" {
				user = "anonymous user"
			}
			fmt.Printf("HMAC SHA-256 JWT for %s with expiration TTL %s:\n%s\n", user, time.Duration(genTokenTTL)*time.Second, token)
		},
	}
	genTokenCmd.Flags().StringVarP(&genTokenConfigFile, "config", "c", "config.json", "path to config file")
	genTokenCmd.Flags().StringVarP(&genTokenUser, "user", "u", "", "user ID")
	genTokenCmd.Flags().Int64VarP(&genTokenTTL, "ttl", "t", 3600*24*7, "token TTL in seconds")

	var checkTokenConfigFile string

	var checkTokenCmd = &cobra.Command{
		Use:   "checktoken [TOKEN]",
		Short: "Check connection JWT",
		Long:  `Check connection JWT`,
		Run: func(cmd *cobra.Command, args []string) {
			bindConfig()
			err := readConfig(checkTokenConfigFile)
			if err != nil && err != errConfigFileNotFound {
				fmt.Printf("error: %v\n", err)
				os.Exit(1)
			}
			jwtVerifierConfig := jwtVerifierConfig()
			if len(args) != 1 {
				fmt.Printf("error: provide token to check [centrifugo checktoken <TOKEN>]\n")
				os.Exit(1)
			}
			subject, claims, err := tools.CheckToken(jwtVerifierConfig, args[0])
			if err != nil {
				fmt.Printf("error: %v\n", err)
				os.Exit(1)
			}
			var user = fmt.Sprintf("user %s", subject)
			if subject == "" {
				user = "anonymous user"
			}
			fmt.Printf("valid token for %s\npayload: %s\n", user, string(claims))
		},
	}
	checkTokenCmd.Flags().StringVarP(&checkTokenConfigFile, "config", "c", "config.json", "path to config file")

	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(checkConfigCmd)
	rootCmd.AddCommand(genConfigCmd)
	rootCmd.AddCommand(genTokenCmd)
	rootCmd.AddCommand(checkTokenCmd)
	_ = rootCmd.Execute()
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

func detectTerminalAttached() {
	if isatty.IsTerminal(os.Stdout.Fd()) && runtime.GOOS != "windows" {
		log.Logger = log.Output(zerolog.ConsoleWriter{
			Out:                 os.Stdout,
			TimeFormat:          "2006-01-02 15:04:05",
			FormatLevel:         logutils.ConsoleFormatLevel(),
			FormatErrFieldName:  logutils.ConsoleFormatErrFieldName(),
			FormatErrFieldValue: logutils.ConsoleFormatErrFieldValue(),
		})
	}
}

func setupLogging() *os.File {
	detectTerminalAttached()

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

func handleSignals(configFile string, n *centrifuge.Node, ruleContainer *rule.ChannelRuleContainer, tokenVerifier *jwtverify.VerifierJWT, httpServers []*http.Server, grpcAPIServer *grpc.Server, exporter *graphite.Exporter) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGHUP, syscall.SIGINT, os.Interrupt, syscall.SIGTERM)
	for {
		sig := <-sigCh
		log.Info().Msgf("signal received: %v", sig)
		switch sig {
		case syscall.SIGHUP:
			// reload application configuration on SIGHUP.
			log.Info().Msg("reloading configuration")
			err := validateConfig(configFile)
			if err != nil {
				log.Error().Msgf("error parsing configuration: %s", err)
				continue
			}
			ruleConfig := ruleConfig()
			if err := tokenVerifier.Reload(jwtVerifierConfig()); err != nil {
				log.Error().Msgf("error reloading: %v", err)
				continue
			}
			if err := ruleContainer.Reload(ruleConfig); err != nil {
				log.Error().Msgf("error reloading: %v", err)
				continue
			}
			log.Info().Msg("configuration successfully reloaded")
		case syscall.SIGINT, os.Interrupt, syscall.SIGTERM:
			log.Info().Msg("shutting down ...")
			pidFile := viper.GetString("pid_file")
			shutdownTimeout := time.Duration(viper.GetInt("shutdown_timeout")) * time.Second
			go time.AfterFunc(shutdownTimeout, func() {
				if pidFile != "" {
					_ = os.Remove(pidFile)
				}
				os.Exit(1)
			})

			if exporter != nil {
				_ = exporter.Close()
			}

			var wg sync.WaitGroup

			if grpcAPIServer != nil {
				wg.Add(1)
				go func() {
					defer wg.Done()
					grpcAPIServer.GracefulStop()
				}()
			}

			ctx, _ := context.WithTimeout(context.Background(), shutdownTimeout)

			for _, srv := range httpServers {
				wg.Add(1)
				go func(srv *http.Server) {
					defer wg.Done()
					_ = srv.Shutdown(ctx)
				}(srv)
			}

			_ = n.Shutdown(ctx)

			wg.Wait()

			if pidFile != "" {
				_ = os.Remove(pidFile)
			}
			time.Sleep(time.Duration(viper.GetInt("shutdown_termination_delay")) * time.Second)
			os.Exit(0)
		}
	}
}

var startHTTPChallengeServerOnce sync.Once // TODO: refactor to get rid of global here.

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
			startHTTPChallengeServerOnce.Do(func() {
				// getTLSConfig can be called several times.
				acmeHTTPServer := &http.Server{
					Handler:  certManager.HTTPHandler(nil),
					Addr:     tlsAutocertHTTPAddr,
					ErrorLog: stdlog.New(&httpErrorLogWriter{log.Logger}, "", 0),
				}
				go func() {
					log.Info().Msgf("serving ACME http_01 challenge on %s", tlsAutocertHTTPAddr)
					if err := acmeHTTPServer.ListenAndServe(); err != nil {
						log.Fatal().Msgf("can't create server on %s to serve acme http challenge: %v", tlsAutocertHTTPAddr, err)
					}
				}()
			})
		}

		return &tls.Config{
			GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
				// See https://github.com/centrifugal/centrifugo/issues/144#issuecomment-279393819
				if tlsAutocertServerName != "" && hello.ServerName == "" {
					hello.ServerName = tlsAutocertServerName
				}
				return certManager.GetCertificate(hello)
			},
			NextProtos: []string{
				"h2", "http/1.1", acme.ALPNProto,
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

func tlsConfigForGRPC() (*tls.Config, error) {
	tlsCert := viper.GetString("grpc_api_tls_cert")
	tlsKey := viper.GetString("grpc_api_tls_key")
	tlsConfig := &tls.Config{}
	tlsConfig.Certificates = make([]tls.Certificate, 1)
	var err error
	tlsConfig.Certificates[0], err = tls.LoadX509KeyPair(tlsCert, tlsKey)
	if err != nil {
		return nil, err
	}
	return tlsConfig, nil
}

type httpErrorLogWriter struct {
	zerolog.Logger
}

func (w *httpErrorLogWriter) Write(data []byte) (int, error) {
	w.Logger.Warn().Msg(strings.TrimSpace(string(data)))
	return len(data), nil
}

func runHTTPServers(n *centrifuge.Node, apiExecutor *api.Executor) ([]*http.Server, error) {
	debug := viper.GetBool("debug")
	useAdmin := viper.GetBool("admin")
	usePrometheus := viper.GetBool("prometheus")
	useHealth := viper.GetBool("health")

	adminExternal := viper.GetBool("admin_external")

	httpAddress := viper.GetString("address")
	httpPort := viper.GetString("port")
	httpInternalAddress := viper.GetString("internal_address")
	httpInternalPort := viper.GetString("internal_port")

	if httpInternalAddress == "" && httpAddress != "" {
		// If custom internal address not explicitly set we try to reuse main
		// address for internal endpoints too.
		httpInternalAddress = httpAddress
	}

	if httpInternalPort == "" {
		// If custom internal port not set we use default http port for
		// internal endpoints too.
		httpInternalPort = httpPort
	}

	// addrToHandlerFlags contains mapping between HTTP server address and
	// handler flags to serve on this address.
	addrToHandlerFlags := map[string]HandlerFlag{}

	var portFlags HandlerFlag

	externalAddr := net.JoinHostPort(httpAddress, httpPort)
	portFlags = addrToHandlerFlags[externalAddr]
	if !viper.GetBool("websocket_disable") {
		portFlags |= HandlerWebsocket
	}
	if !viper.GetBool("sockjs_disable") {
		portFlags |= HandlerSockJS
	}
	if useAdmin && adminExternal {
		portFlags |= HandlerAdmin
	}
	addrToHandlerFlags[externalAddr] = portFlags

	internalAddr := net.JoinHostPort(httpInternalAddress, httpInternalPort)
	portFlags = addrToHandlerFlags[internalAddr]
	if !viper.GetBool("api_disable") {
		portFlags |= HandlerAPI
	}

	if useAdmin && !adminExternal {
		portFlags |= HandlerAdmin
	}
	if usePrometheus {
		portFlags |= HandlerPrometheus
	}
	if debug {
		portFlags |= HandlerDebug
	}
	if useHealth {
		portFlags |= HandlerHealth
	}
	addrToHandlerFlags[internalAddr] = portFlags

	var servers []*http.Server

	tlsConfig, err := getTLSConfig()
	if err != nil {
		log.Fatal().Msgf("can not get TLS config: %v", err)
	}

	// Iterate over port to flags mapping and start HTTP servers
	// on separate ports serving handlers specified in flags.
	for addr, handlerFlags := range addrToHandlerFlags {
		if handlerFlags == 0 {
			continue
		}
		mux := Mux(n, apiExecutor, handlerFlags)

		log.Info().Msgf("serving %s endpoints on %s", handlerFlags, addr)

		var addrTLSConfig *tls.Config
		if !viper.GetBool("tls_external") || addr == externalAddr {
			addrTLSConfig = tlsConfig
		}
		server := &http.Server{
			Addr:      addr,
			Handler:   mux,
			TLSConfig: addrTLSConfig,
			ErrorLog:  stdlog.New(&httpErrorLogWriter{log.Logger}, "", 0),
		}

		servers = append(servers, server)

		go func() {
			if addrTLSConfig != nil {
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

var errConfigFileNotFound = errors.New("unable to find configuration file")

// readConfig reads config.
func readConfig(f string) error {
	viper.SetConfigFile(f)
	err := viper.ReadInConfig()
	if err != nil {
		switch err.(type) {
		case viper.ConfigParseError:
			return err
		default:
			return errConfigFileNotFound
		}
	}
	return nil
}

// validateConfig validates config file located at provided path.
func validateConfig(f string) error {
	err := readConfig(f)
	if err != nil {
		return err
	}
	ruleConfig := ruleConfig()
	if err := ruleConfig.Validate(); err != nil {
		return err
	}
	return nil
}

func ruleConfig() rule.ChannelRuleConfig {
	v := viper.GetViper()
	cfg := rule.ChannelRuleConfig{}

	cfg.Publish = v.GetBool("publish")
	cfg.SubscribeToPublish = v.GetBool("subscribe_to_publish")
	cfg.Anonymous = v.GetBool("anonymous")
	cfg.Presence = v.GetBool("presence")
	cfg.PresenceDisableForClient = v.GetBool("presence_disable_for_client")
	cfg.JoinLeave = v.GetBool("join_leave")
	cfg.HistorySize = v.GetInt("history_size")
	cfg.HistoryLifetime = v.GetInt("history_lifetime")
	cfg.HistoryRecover = v.GetBool("history_recover")
	cfg.HistoryDisableForClient = v.GetBool("history_disable_for_client")
	cfg.ServerSide = v.GetBool("server_side")
	cfg.ProxySubscribe = v.GetBool("proxy_subscribe")
	cfg.ProxyPublish = v.GetBool("proxy_publish")
	cfg.Namespaces = namespacesFromConfig(v)

	// TODO v3: replace option name to token_channel_prefix.
	cfg.TokenChannelPrefix = v.GetString("channel_private_prefix")
	cfg.ChannelNamespaceBoundary = v.GetString("channel_namespace_boundary")
	cfg.ChannelUserBoundary = v.GetString("channel_user_boundary")
	cfg.ChannelUserSeparator = v.GetString("channel_user_separator")
	cfg.UserSubscribeToPersonal = v.GetBool("user_subscribe_to_personal")
	cfg.UserPersonalChannelNamespace = v.GetString("user_personal_channel_namespace")
	cfg.ClientInsecure = v.GetBool("client_insecure")
	cfg.ClientAnonymous = v.GetBool("client_anonymous")
	return cfg
}

func jwtVerifierConfig() jwtverify.VerifierConfig {
	v := viper.GetViper()
	cfg := jwtverify.VerifierConfig{}

	hmacSecretKey := v.GetString("token_hmac_secret_key")
	if hmacSecretKey != "" {
		cfg.HMACSecretKey = hmacSecretKey
	} else {
		if v.GetString("secret") != "" {
			log.Warn().Msg("secret is deprecated, use token_hmac_secret_key instead")
		}
		cfg.HMACSecretKey = v.GetString("secret")
	}

	rsaPublicKey := v.GetString("token_rsa_public_key")
	if rsaPublicKey != "" {
		pubKey, err := jwtutils.ParseRSAPublicKeyFromPEM([]byte(rsaPublicKey))
		if err != nil {
			log.Fatal().Msgf("error parsing RSA public key: %v", err)
		}
		cfg.RSAPublicKey = pubKey
	}

	return cfg
}

func proxyConfig() (proxy.Config, bool) {
	v := viper.GetViper()
	cfg := proxy.Config{}
	cfg.ExtraHTTPHeaders = v.GetStringSlice("proxy_extra_http_headers")
	cfg.ConnectEndpoint = v.GetString("proxy_connect_endpoint")
	cfg.ConnectTimeout = time.Duration(v.GetFloat64("proxy_connect_timeout")*1000) * time.Millisecond
	cfg.RefreshEndpoint = v.GetString("proxy_refresh_endpoint")
	cfg.RefreshTimeout = time.Duration(v.GetFloat64("proxy_refresh_timeout")*1000) * time.Millisecond
	cfg.RPCEndpoint = v.GetString("proxy_rpc_endpoint")
	cfg.RPCTimeout = time.Duration(v.GetFloat64("proxy_rpc_timeout")*1000) * time.Millisecond
	cfg.SubscribeEndpoint = v.GetString("proxy_subscribe_endpoint")
	cfg.SubscribeTimeout = time.Duration(v.GetFloat64("proxy_subscribe_timeout")*1000) * time.Millisecond
	cfg.PublishEndpoint = v.GetString("proxy_publish_endpoint")
	cfg.PublishTimeout = time.Duration(v.GetFloat64("proxy_publish_timeout")*1000) * time.Millisecond

	proxyEnabled := cfg.ConnectEndpoint != "" || cfg.RefreshEndpoint != "" ||
		cfg.RPCEndpoint != "" || cfg.SubscribeEndpoint != "" || cfg.PublishEndpoint != ""

	return cfg, proxyEnabled
}

func nodeConfig(version string, chOptsFunc centrifuge.ChannelOptionsFunc) centrifuge.Config {
	v := viper.GetViper()
	cfg := centrifuge.Config{}
	cfg.Version = version
	cfg.Name = applicationName()
	cfg.ChannelMaxLength = v.GetInt("channel_max_length")
	cfg.ClientPresenceUpdateInterval = time.Duration(v.GetInt("client_presence_ping_interval")) * time.Second
	cfg.ClientPresenceExpireInterval = time.Duration(v.GetInt("client_presence_expire_interval")) * time.Second
	cfg.ClientExpiredCloseDelay = time.Duration(v.GetInt("client_expired_close_delay")) * time.Second
	cfg.ClientExpiredSubCloseDelay = time.Duration(v.GetInt("client_expired_sub_close_delay")) * time.Second
	cfg.ClientStaleCloseDelay = time.Duration(v.GetInt("client_stale_close_delay")) * time.Second
	cfg.ClientQueueMaxSize = v.GetInt("client_queue_max_size")
	cfg.ClientChannelLimit = v.GetInt("client_channel_limit")
	cfg.ClientUserConnectionLimit = v.GetInt("client_user_connection_limit")
	cfg.ClientChannelPositionCheckDelay = time.Duration(v.GetInt("client_channel_position_check_delay")) * time.Second
	cfg.NodeInfoMetricsAggregateInterval = time.Duration(v.GetInt("node_info_metrics_aggregate_interval")) * time.Second
	cfg.ChannelOptionsFunc = chOptsFunc

	level, ok := logStringToLevel[strings.ToLower(v.GetString("log_level"))]
	if !ok {
		level = centrifuge.LogLevelInfo
	}
	cfg.LogLevel = level
	cfg.LogHandler = newLogHandler().handle
	return cfg
}

// LogStringToLevel matches level string to Centrifuge LogLevel.
var logStringToLevel = map[string]centrifuge.LogLevel{
	"debug": centrifuge.LogLevelDebug,
	"info":  centrifuge.LogLevelInfo,
	"error": centrifuge.LogLevelError,
	"none":  centrifuge.LogLevelNone,
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
func namespacesFromConfig(v *viper.Viper) []rule.ChannelNamespace {
	var ns []rule.ChannelNamespace
	if !v.IsSet("namespaces") {
		return ns
	}
	var err error
	switch val := v.Get("namespaces").(type) {
	case string:
		err = json.Unmarshal([]byte(val), &ns)
	case []interface{}:
		err = v.UnmarshalKey("namespaces", &ns)
	default:
		err = fmt.Errorf("unknown namespaces type: %T", val)
	}
	if err != nil {
		log.Error().Err(err).Msg("malformed namespaces")
		os.Exit(1)
	}
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
	cfg.UseWriteBufferPool = v.GetBool("websocket_use_write_buffer_pool")
	if v.IsSet("websocket_ping_interval") {
		cfg.PingInterval = time.Duration(v.GetInt("websocket_ping_interval")) * time.Second
	} else {
		cfg.PingInterval = time.Duration(v.GetInt("client_ping_interval")) * time.Second
	}
	if v.IsSet("websocket_write_timeout") {
		cfg.WriteTimeout = time.Duration(v.GetInt("websocket_write_timeout")) * time.Second
	} else {
		cfg.WriteTimeout = time.Duration(v.GetInt("client_message_write_timeout")) * time.Second
	}
	if v.IsSet("websocket_message_size_limit") {
		cfg.MessageSizeLimit = v.GetInt("websocket_message_size_limit")
	} else {
		cfg.MessageSizeLimit = v.GetInt("client_request_max_size")
	}
	return cfg
}

func sockjsHandlerConfig() centrifuge.SockjsConfig {
	v := viper.GetViper()
	cfg := centrifuge.SockjsConfig{}
	cfg.URL = v.GetString("sockjs_url")
	cfg.HeartbeatDelay = time.Duration(v.GetInt("sockjs_heartbeat_delay")) * time.Second
	cfg.CheckOrigin = func(*http.Request) bool { return true }
	cfg.WebsocketCheckOrigin = func(r *http.Request) bool { return true }
	cfg.WebsocketReadBufferSize = v.GetInt("websocket_read_buffer_size")
	cfg.WebsocketWriteBufferSize = v.GetInt("websocket_write_buffer_size")
	cfg.WebsocketUseWriteBufferPool = v.GetBool("websocket_use_write_buffer_pool")
	if v.IsSet("websocket_write_timeout") {
		cfg.WebsocketWriteTimeout = time.Duration(v.GetInt("websocket_write_timeout")) * time.Second
	} else {
		cfg.WebsocketWriteTimeout = time.Duration(v.GetInt("client_message_write_timeout")) * time.Second
	}
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
	cfg.Prefix = v.GetString("admin_handler_prefix")
	return cfg
}

func memoryEngine(n *centrifuge.Node) (centrifuge.Engine, error) {
	c, err := memoryEngineConfig()
	if err != nil {
		return nil, err
	}
	return centrifuge.NewMemoryEngine(n, *c)
}

func redisEngine(n *centrifuge.Node, publishOnHistoryAdd bool) (centrifuge.Engine, error) {
	c, err := redisEngineConfig(publishOnHistoryAdd)
	if err != nil {
		return nil, err
	}
	return centrifuge.NewRedisEngine(n, *c)
}

func memoryEngineConfig() (*centrifuge.MemoryEngineConfig, error) {
	return &centrifuge.MemoryEngineConfig{
		HistoryMetaTTL: time.Duration(viper.GetInt("memory_history_meta_ttl")) * time.Second,
	}, nil
}

func addRedisShardCommonSettings(shardConf *centrifuge.RedisShardConfig) {
	v := viper.GetViper()
	shardConf.Password = v.GetString("redis_password")
	shardConf.Prefix = v.GetString("redis_prefix")
	shardConf.UseTLS = v.GetBool("redis_tls")
	shardConf.TLSSkipVerify = v.GetBool("redis_tls_skip_verify")
	shardConf.IdleTimeout = time.Duration(v.GetInt("redis_idle_timeout")) * time.Second
	shardConf.PubSubNumWorkers = v.GetInt("redis_pubsub_num_workers")
	shardConf.ConnectTimeout = time.Duration(v.GetInt("redis_connect_timeout")) * time.Second
	shardConf.ReadTimeout = time.Duration(v.GetInt("redis_read_timeout")) * time.Second
	shardConf.WriteTimeout = time.Duration(v.GetInt("redis_write_timeout")) * time.Second
}

func redisEngineConfig(publishOnHistoryAdd bool) (*centrifuge.RedisEngineConfig, error) {
	v := viper.GetViper()

	clusterConf := v.GetStringSlice("redis_cluster_addrs")
	var useCluster bool
	if len(clusterConf) > 0 {
		useCluster = true
	}

	var shardConfigs []centrifuge.RedisShardConfig

	if useCluster {
		for _, clusterAddrsStr := range clusterConf {
			clusterAddrs := strings.Split(clusterAddrsStr, ",")
			conf := &centrifuge.RedisShardConfig{
				ClusterAddrs: clusterAddrs,
			}
			addRedisShardCommonSettings(conf)
			shardConfigs = append(shardConfigs, *conf)
		}
	} else {
		numShards := 1

		hostsConf := v.GetString("redis_host")
		portsConf := v.GetString("redis_port")
		urlsConf := v.GetString("redis_url")
		masterNamesConf := v.GetString("redis_master_name")
		sentinelsConf := v.GetString("redis_sentinels")

		password := v.GetString("redis_password")
		db := v.GetInt("redis_db")

		sentinelPassword := v.GetString("redis_sentinel_password")

		var hosts []string
		if hostsConf != "" {
			hosts = strings.Split(hostsConf, ",")
			if len(hosts) > numShards {
				numShards = len(hosts)
			}
		}

		var ports []string
		if portsConf != "" {
			ports = strings.Split(portsConf, ",")
			if len(ports) > numShards {
				numShards = len(ports)
			}
		}

		var urls []string
		if urlsConf != "" {
			urls = strings.Split(urlsConf, ",")
			if len(urls) > numShards {
				numShards = len(urls)
			}
		}

		var masterNames []string
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
			return nil, fmt.Errorf("master name must be set for every Redis shard when Sentinel used")
		}

		var sentinelAddrs []string
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
		sentinelPasswords := make([]string, numShards)
		for i := 0; i < numShards; i++ {
			passwords[i] = password
			sentinelPasswords[i] = sentinelPassword
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

		for i := 0; i < numShards; i++ {
			port, err := strconv.Atoi(ports[i])
			if err != nil {
				return nil, fmt.Errorf("malformed port: %v", err)
			}
			conf := &centrifuge.RedisShardConfig{
				Host:               hosts[i],
				Port:               port,
				DB:                 dbs[i],
				SentinelMasterName: masterNames[i],
				SentinelAddrs:      sentinelAddrs,
			}
			addRedisShardCommonSettings(conf)
			conf.Password = passwords[i]
			conf.SentinelPassword = sentinelPasswords[i]
			shardConfigs = append(shardConfigs, *conf)
		}
	}

	var historyMetaTTL time.Duration
	if v.IsSet("redis_history_meta_ttl") {
		historyMetaTTL = time.Duration(v.GetInt("redis_history_meta_ttl")) * time.Second
	} else {
		// TODO v3: remove compatibility.
		historyMetaTTL = time.Duration(v.GetInt("redis_sequence_ttl")) * time.Second
	}

	return &centrifuge.RedisEngineConfig{
		PublishOnHistoryAdd: publishOnHistoryAdd,
		UseStreams:          v.GetBool("redis_streams"),
		HistoryMetaTTL:      historyMetaTTL,
		Shards:              shardConfigs,
	}, nil
}

type logHandler struct {
	entries chan centrifuge.LogEntry
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
		default:
			continue
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
	var endpoints []string
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
func Mux(n *centrifuge.Node, apiExecutor *api.Executor, flags HandlerFlag) *http.ServeMux {

	mux := http.NewServeMux()

	v := viper.GetViper()

	if flags&HandlerDebug != 0 {
		mux.Handle("/debug/pprof/", middleware.LogRequest(http.HandlerFunc(pprof.Index)))
		mux.Handle("/debug/pprof/cmdline", middleware.LogRequest(http.HandlerFunc(pprof.Cmdline)))
		mux.Handle("/debug/pprof/profile", middleware.LogRequest(http.HandlerFunc(pprof.Profile)))
		mux.Handle("/debug/pprof/symbol", middleware.LogRequest(http.HandlerFunc(pprof.Symbol)))
		mux.Handle("/debug/pprof/trace", middleware.LogRequest(http.HandlerFunc(pprof.Trace)))
	}

	_, proxyEnabled := proxyConfig()

	if flags&HandlerWebsocket != 0 {
		// register Websocket connection endpoint.
		wsPrefix := strings.TrimRight(v.GetString("websocket_handler_prefix"), "/")
		if wsPrefix == "" {
			wsPrefix = "/"
		}
		mux.Handle(wsPrefix, middleware.LogRequest(middleware.HeadersToContext(proxyEnabled, centrifuge.NewWebsocketHandler(n, websocketHandlerConfig()))))
	}

	if flags&HandlerSockJS != 0 {
		// register SockJS connection endpoints.
		sockjsConfig := sockjsHandlerConfig()
		sockjsPrefix := strings.TrimRight(v.GetString("sockjs_handler_prefix"), "/")
		sockjsConfig.HandlerPrefix = sockjsPrefix
		mux.Handle(sockjsPrefix+"/", middleware.LogRequest(middleware.HeadersToContext(proxyEnabled, centrifuge.NewSockjsHandler(n, sockjsConfig))))
	}

	if flags&HandlerAPI != 0 {
		// register HTTP API endpoint.
		apiHandler := api.NewHandler(n, apiExecutor, api.Config{})
		apiPrefix := strings.TrimRight(v.GetString("api_handler_prefix"), "/")
		if apiPrefix == "" {
			apiPrefix = "/"
		}
		if viper.GetBool("api_insecure") {
			mux.Handle(apiPrefix, middleware.LogRequest(middleware.Post(apiHandler)))
		} else {
			mux.Handle(apiPrefix, middleware.LogRequest(middleware.Post(middleware.APIKeyAuth(viper.GetString("api_key"), apiHandler))))
		}
	}

	if flags&HandlerPrometheus != 0 {
		// register Prometheus metrics export endpoint.
		prometheusPrefix := strings.TrimRight(v.GetString("prometheus_handler_prefix"), "/")
		if prometheusPrefix == "" {
			prometheusPrefix = "/"
		}
		mux.Handle(prometheusPrefix, middleware.LogRequest(promhttp.Handler()))
	}

	if flags&HandlerAdmin != 0 {
		// register admin web interface API endpoints.
		adminPrefix := strings.TrimRight(v.GetString("admin_handler_prefix"), "/")
		mux.Handle(adminPrefix+"/", middleware.LogRequest(admin.NewHandler(n, apiExecutor, adminHandlerConfig())))
	}

	if flags&HandlerHealth != 0 {
		healthPrefix := strings.TrimRight(v.GetString("health_handler_prefix"), "/")
		if healthPrefix == "" {
			healthPrefix = "/"
		}
		mux.Handle(healthPrefix, middleware.LogRequest(health.NewHandler(n, health.Config{})))
	}

	return mux
}
