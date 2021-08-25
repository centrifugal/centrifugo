// Copyright (c) 2015 Centrifugal
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
// WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
// License for the specific language governing permissions and limitations
// under the License.
package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	stdlog "log"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/centrifugal/centrifugo/v3/internal/admin"
	"github.com/centrifugal/centrifugo/v3/internal/api"
	"github.com/centrifugal/centrifugo/v3/internal/build"
	"github.com/centrifugal/centrifugo/v3/internal/cli"
	"github.com/centrifugal/centrifugo/v3/internal/client"
	"github.com/centrifugal/centrifugo/v3/internal/health"
	"github.com/centrifugal/centrifugo/v3/internal/jwtutils"
	"github.com/centrifugal/centrifugo/v3/internal/jwtverify"
	"github.com/centrifugal/centrifugo/v3/internal/logutils"
	"github.com/centrifugal/centrifugo/v3/internal/metrics/graphite"
	"github.com/centrifugal/centrifugo/v3/internal/middleware"
	"github.com/centrifugal/centrifugo/v3/internal/natsbroker"
	"github.com/centrifugal/centrifugo/v3/internal/origin"
	"github.com/centrifugal/centrifugo/v3/internal/proxy"
	"github.com/centrifugal/centrifugo/v3/internal/proxyproto"
	"github.com/centrifugal/centrifugo/v3/internal/rule"
	"github.com/centrifugal/centrifugo/v3/internal/survey"
	"github.com/centrifugal/centrifugo/v3/internal/tntengine"
	"github.com/centrifugal/centrifugo/v3/internal/tools"
	"github.com/centrifugal/centrifugo/v3/internal/unigrpc"
	"github.com/centrifugal/centrifugo/v3/internal/unihttpstream"
	"github.com/centrifugal/centrifugo/v3/internal/unisse"
	"github.com/centrifugal/centrifugo/v3/internal/uniws"
	"github.com/centrifugal/centrifugo/v3/internal/webui"

	"github.com/FZambia/viper-lite"
	"github.com/centrifugal/centrifuge"
	"github.com/mattn/go-isatty"
	"github.com/mitchellh/mapstructure"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
)

func bindCentrifugoConfig() {
	viper.SetEnvPrefix("centrifugo")

	var defaults = map[string]interface{}{
		"gomaxprocs": 0,
		"name":       "",
		"engine":     "memory",
		"broker":     "",

		"token_hmac_secret_key":      "",
		"token_rsa_public_key":       "",
		"token_ecdsa_public_key":     "",
		"token_jwks_public_endpoint": "",

		"protected":                   false,
		"publish":                     false,
		"subscribe_to_publish":        false,
		"anonymous":                   false,
		"presence":                    false,
		"presence_disable_for_client": false,
		"history_size":                0,
		"history_ttl":                 0,
		"history_disable_for_client":  false,
		"recover":                     false,
		"position":                    false,
		"proxy_subscribe":             false,
		"proxy_publish":               false,

		"node_info_metrics_aggregate_interval": 60 * time.Second,

		"client_anonymous":                    false,
		"client_expired_close_delay":          25 * time.Second,
		"client_expired_sub_close_delay":      25 * time.Second,
		"client_stale_close_delay":            25 * time.Second,
		"client_channel_limit":                128,
		"client_queue_max_size":               1048576, // 1 MB
		"client_presence_update_interval":     25 * time.Second,
		"client_user_connection_limit":        0,
		"client_concurrency":                  0,
		"client_channel_position_check_delay": 40 * time.Second,

		"channel_max_length":         255,
		"channel_private_prefix":     "$",
		"channel_namespace_boundary": ":",
		"channel_user_boundary":      "#",
		"channel_user_separator":     ",",

		"user_subscribe_to_personal":      false,
		"user_personal_channel_namespace": "",
		"user_personal_single_connection": false,

		"debug":      false,
		"prometheus": false,
		"health":     false,

		"admin":          false,
		"admin_password": "",
		"admin_secret":   "",
		"admin_insecure": false,
		"admin_web_path": "",

		"sockjs":                 false,
		"sockjs_url":             "https://cdn.jsdelivr.net/npm/sockjs-client@1/dist/sockjs.min.js",
		"sockjs_heartbeat_delay": 25 * time.Second,

		"websocket_compression":           false,
		"websocket_compression_min_size":  0,
		"websocket_compression_level":     1,
		"websocket_read_buffer_size":      0,
		"websocket_use_write_buffer_pool": false,
		"websocket_write_buffer_size":     0,
		"websocket_ping_interval":         25 * time.Second,
		"websocket_write_timeout":         time.Second,
		"websocket_message_size_limit":    65536, // 64KB

		"uni_websocket":                       false,
		"uni_websocket_compression":           false,
		"uni_websocket_compression_min_size":  0,
		"uni_websocket_compression_level":     1,
		"uni_websocket_read_buffer_size":      0,
		"uni_websocket_use_write_buffer_pool": false,
		"uni_websocket_write_buffer_size":     0,
		"uni_websocket_ping_interval":         25 * time.Second,
		"uni_websocket_write_timeout":         time.Second,
		"uni_websocket_message_size_limit":    65536, // 64KB

		"uni_http_stream_max_request_body_size": 65536, // 64KB

		"uni_sse_max_request_body_size": 65536, // 64KB

		"tls_autocert":                false,
		"tls_autocert_host_whitelist": "",
		"tls_autocert_cache_dir":      "",
		"tls_autocert_email":          "",
		"tls_autocert_server_name":    "",
		"tls_autocert_http":           false,
		"tls_autocert_http_addr":      ":80",

		"redis_prefix":          "centrifugo",
		"redis_connect_timeout": time.Second,
		"redis_read_timeout":    5 * time.Second,
		"redis_write_timeout":   time.Second,
		"redis_idle_timeout":    0,

		"history_meta_ttl": 0,
		"presence_ttl":     60 * time.Second,

		"grpc_api":         false,
		"grpc_api_address": "",
		"grpc_api_port":    10000,

		"shutdown_timeout":           30 * time.Second,
		"shutdown_termination_delay": 0,

		"graphite":          false,
		"graphite_host":     "localhost",
		"graphite_port":     2003,
		"graphite_prefix":   "centrifugo",
		"graphite_interval": 10 * time.Second,
		"graphite_tags":     false,

		"nats_prefix":        "centrifugo",
		"nats_url":           "",
		"nats_dial_timeout":  time.Second,
		"nats_write_timeout": time.Second,

		"websocket_disable": false,
		"api_disable":       false,

		"websocket_handler_prefix": "/connection/websocket",
		"sockjs_handler_prefix":    "/connection/sockjs",

		"uni_websocket_handler_prefix":      "/connection/uni_websocket",
		"uni_sse_handler_prefix":            "/connection/uni_sse",
		"uni_http_stream_handler_prefix":    "/connection/uni_http_stream",
		"uni_grpc":                          false,
		"uni_grpc_address":                  "",
		"uni_grpc_port":                     11000,
		"uni_grpc_max_receive_message_size": 65536,

		"admin_handler_prefix":      "",
		"api_handler_prefix":        "/api",
		"prometheus_handler_prefix": "/metrics",
		"health_handler_prefix":     "/health",

		"proxy_connect_timeout":   time.Second,
		"proxy_rpc_timeout":       time.Second,
		"proxy_refresh_timeout":   time.Second,
		"proxy_subscribe_timeout": time.Second,
		"proxy_publish_timeout":   time.Second,

		"client_history_max_publication_limit":  300,
		"client_recovery_max_publication_limit": 300,
	}

	for k, v := range defaults {
		viper.SetDefault(k, v)
	}

	replacer := strings.NewReplacer(".", "_")
	viper.SetEnvKeyReplacer(replacer)
	viper.AutomaticEnv()
}

func main() {
	var configFile string

	var rootCmd = &cobra.Command{
		Use:   "",
		Short: "Centrifugo",
		Long:  "Centrifugo – scalable real-time messaging server in language-agnostic way",
		Run: func(cmd *cobra.Command, args []string) {
			bindCentrifugoConfig()

			bindPFlags := []string{
				"engine", "log_level", "log_file", "pid_file", "debug", "name", "admin",
				"admin_external", "client_insecure", "admin_insecure", "api_insecure",
				"port", "address", "tls", "tls_cert", "tls_key", "tls_external", "internal_port",
				"internal_address", "prometheus", "health", "redis_address", "tarantool_address",
				"broker", "nats_url",
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
				defer func() { _ = file.Close() }()
			}

			err = writePidFile(viper.GetString("pid_file"))
			if err != nil {
				log.Fatal().Msgf("error writing PID: %v", err)
			}

			if viper.IsSet("v3_use_offset") {
				log.Fatal().Msg("v3_use_offset option is set which was removed in Centrifugo v3. " +
					"Make sure to adapt your configuration file to fit Centrifugo v3 changes. See " +
					"https://centrifugal.dev/docs/getting-started/migration_v3. If you had no intention to " +
					"update Centrifugo to v3 then this error may be caused by using `latest` tag for " +
					"Centrifugo Docker image in your deployment pipeline – pin to the specific Centrifugo " +
					"image tag in this case (at least to centrifugo/centrifugo:v2).")
			}

			if os.Getenv("GOMAXPROCS") == "" {
				if viper.IsSet("gomaxprocs") && viper.GetInt("gomaxprocs") > 0 {
					runtime.GOMAXPROCS(viper.GetInt("gomaxprocs"))
				} else {
					runtime.GOMAXPROCS(runtime.NumCPU())
				}
			}

			engineName := viper.GetString("engine")

			log.Info().
				Str("version", build.Version).
				Str("runtime", runtime.Version()).
				Int("pid", os.Getpid()).
				Str("engine", strings.Title(engineName)).
				Int("gomaxprocs", runtime.GOMAXPROCS(0)).Msg("starting Centrifugo")

			log.Info().Str("path", absConfPath).Msg("using config file")

			proxyConfig, _ := proxyConfig()

			ruleConfig := ruleConfig()
			err = ruleConfig.Validate()
			if err != nil {
				log.Fatal().Msgf("error validating config: %v", err)
			}
			ruleContainer := rule.NewContainer(ruleConfig)

			nodeConfig := nodeConfig(build.Version)

			node, err := centrifuge.New(nodeConfig)
			if err != nil {
				log.Fatal().Msgf("error creating Centrifuge Node: %v", err)
			}

			brokerName := viper.GetString("broker")
			if brokerName != "" && brokerName != "nats" {
				log.Fatal().Msgf("unknown broker: %s", brokerName)
			}

			var broker centrifuge.Broker
			var presenceManager centrifuge.PresenceManager

			if engineName == "memory" {
				broker, presenceManager, err = memoryEngine(node)
			} else if engineName == "redis" {
				broker, presenceManager, err = redisEngine(node)
			} else if engineName == "tarantool" {
				broker, presenceManager, err = tarantoolEngine(node)
			} else {
				log.Fatal().Msgf("unknown engine: %s", engineName)
			}
			if err != nil {
				log.Fatal().Msgf("error creating engine: %v", err)
			}

			tokenVerifier := jwtverify.NewTokenVerifierJWT(jwtVerifierConfig(), ruleContainer)

			if viper.GetBool("use_unlimited_history_by_default") {
				// See detailed comment about this by falling through to var definition.
				client.UseUnlimitedHistoryByDefault = true
			}
			clientHandler := client.NewHandler(node, ruleContainer, tokenVerifier, proxyConfig)
			err = clientHandler.Setup()
			if err != nil {
				log.Fatal().Msgf("error setting up client handler: %v", err)
			}

			surveyCaller := survey.NewCaller(node)

			httpAPIExecutor := api.NewExecutor(node, ruleContainer, surveyCaller, "http")
			grpcAPIExecutor := api.NewExecutor(node, ruleContainer, surveyCaller, "grpc")

			node.SetBroker(broker)
			node.SetPresenceManager(presenceManager)

			var disableHistoryPresence bool
			if engineName == "memory" && brokerName == "nats" {
				// Presence and History won't work with Memory engine in distributed case.
				disableHistoryPresence = true
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
					DialTimeout:  GetDuration("nats_dial_timeout"),
					WriteTimeout: GetDuration("nats_write_timeout"),
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
				log.Info().Str("endpoint", proxyConfig.ConnectEndpoint).Msg("connect proxy enabled")
			}
			if proxyConfig.RefreshEndpoint != "" {
				log.Info().Str("endpoint", proxyConfig.RefreshEndpoint).Msg("refresh proxy enabled")
			}
			if proxyConfig.RPCEndpoint != "" {
				log.Info().Str("endpoint", proxyConfig.RPCEndpoint).Msg("RPC proxy enabled")
			}
			if proxyConfig.SubscribeEndpoint != "" {
				log.Info().Str("endpoint", proxyConfig.SubscribeEndpoint).Msg("subscribe proxy enabled")
			}
			if proxyConfig.PublishEndpoint != "" {
				log.Info().Str("endpoint", proxyConfig.PublishEndpoint).Msg("publish proxy enabled")
			}

			if viper.GetBool("client_insecure") {
				log.Warn().Msg("INSECURE client mode enabled, make sure you understand risks")
			}
			if viper.GetBool("api_insecure") {
				log.Warn().Msg("INSECURE API mode enabled, make sure you understand risks")
			}
			if viper.GetBool("admin_insecure") {
				log.Warn().Msg("INSECURE admin mode enabled, make sure you understand risks")
			}
			if viper.GetBool("debug") {
				log.Warn().Msg("DEBUG mode enabled, see /debug/pprof")
			}

			var grpcAPIServer *grpc.Server
			var grpcAPIAddr string
			if viper.GetBool("grpc_api") {
				grpcAPIAddr = net.JoinHostPort(viper.GetString("grpc_api_address"), viper.GetString("grpc_api_port"))
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
				_ = api.RegisterGRPCServerAPI(node, grpcAPIExecutor, grpcAPIServer, api.GRPCAPIServiceConfig{})
				go func() {
					if err := grpcAPIServer.Serve(grpcAPIConn); err != nil {
						log.Fatal().Msgf("serve GRPC API: %v", err)
					}
				}()
			}

			if grpcAPIServer != nil {
				log.Info().Msgf("serving GRPC API service on %s", grpcAPIAddr)
			}

			var grpcUniServer *grpc.Server
			var grpcUniAddr string
			if viper.GetBool("uni_grpc") {
				grpcUniAddr = net.JoinHostPort(viper.GetString("uni_grpc_address"), viper.GetString("uni_grpc_port"))
				grpcUniConn, err := net.Listen("tcp", grpcUniAddr)
				if err != nil {
					log.Fatal().Msgf("cannot listen to address %s", grpcUniAddr)
				}
				var grpcOpts []grpc.ServerOption
				//nolint:staticcheck
				//goland:noinspection GoDeprecation
				grpcOpts = append(grpcOpts, grpc.CustomCodec(&unigrpc.RawCodec{}), grpc.MaxRecvMsgSize(viper.GetInt("uni_grpc_max_receive_message_size")))
				var tlsConfig *tls.Config
				var tlsErr error

				if viper.GetBool("uni_grpc_tls") {
					tlsConfig, tlsErr = tlsConfigForUniGRPC()
				} else if !viper.GetBool("uni_grpc_tls_disable") {
					tlsConfig, tlsErr = getTLSConfig()
				}
				if tlsErr != nil {
					log.Fatal().Msgf("error getting TLS config: %v", tlsErr)
				}
				if tlsConfig != nil {
					grpcOpts = append(grpcOpts, grpc.Creds(credentials.NewTLS(tlsConfig)))
				}
				keepAliveEnforcementPolicy := keepalive.EnforcementPolicy{
					MinTime: 5 * time.Second,
				}
				keepAliveServerParams := keepalive.ServerParameters{
					Time:    25 * time.Second,
					Timeout: 5 * time.Second,
				}
				grpcOpts = append(grpcOpts, grpc.KeepaliveEnforcementPolicy(keepAliveEnforcementPolicy))
				grpcOpts = append(grpcOpts, grpc.KeepaliveParams(keepAliveServerParams))
				grpcUniServer = grpc.NewServer(grpcOpts...)
				_ = unigrpc.RegisterService(grpcUniServer, unigrpc.NewService(node, uniGRPCHandlerConfig()))
				go func() {
					if err := grpcUniServer.Serve(grpcUniConn); err != nil {
						log.Fatal().Msgf("serve uni GRPC: %v", err)
					}
				}()
			}

			if grpcUniServer != nil {
				log.Info().Msgf("serving unidirectional GRPC on %s", grpcUniAddr)
			}

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
					Interval: GetDuration("graphite_interval"),
					Tags:     viper.GetBool("graphite_tags"),
				})
			}

			handleSignals(configFile, node, ruleContainer, tokenVerifier, servers, grpcAPIServer, grpcUniServer, exporter)
		},
	}

	rootCmd.Flags().StringVarP(&configFile, "config", "c", "config.json", "path to config file")
	rootCmd.Flags().StringP("engine", "e", "memory", "engine to use: memory or redis")
	rootCmd.Flags().StringP("broker", "", "", "custom broker to use: ex. nats")
	rootCmd.Flags().StringP("log_level", "", "info", "set the log level: trace, debug, info, error, fatal or none")
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
	rootCmd.Flags().StringP("port", "p", "8000", "port to bind HTTP server to")
	rootCmd.Flags().StringP("internal_address", "", "", "custom interface address to listen on for internal endpoints")
	rootCmd.Flags().StringP("internal_port", "", "", "custom port for internal endpoints")

	rootCmd.Flags().BoolP("tls", "", false, "enable TLS, requires an X509 certificate and a key file")
	rootCmd.Flags().StringP("tls_cert", "", "", "path to an X509 certificate file")
	rootCmd.Flags().StringP("tls_key", "", "", "path to an X509 certificate key")
	rootCmd.Flags().BoolP("tls_external", "", false, "enable TLS only for external endpoints")

	rootCmd.Flags().StringP("redis_address", "", "redis://127.0.0.1:6379", "Redis connection address (Redis engine)")
	rootCmd.Flags().StringP("tarantool_address", "", "tcp://127.0.0.1:3301", "Tarantool connection address (Tarantool engine)")
	rootCmd.Flags().StringP("nats_url", "", "nats://127.0.0.1:4222", "Nats connection URL in format nats://user:pass@localhost:4222 (Nats broker)")

	var versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Centrifugo version information",
		Long:  `Print the version information of Centrifugo`,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("Centrifugo v%s (Go version: %s)\n", build.Version, runtime.Version())
		},
	}

	var checkConfigFile string

	var checkConfigCmd = &cobra.Command{
		Use:   "checkconfig",
		Short: "Check configuration file",
		Long:  `Check Centrifugo configuration file`,
		Run: func(cmd *cobra.Command, args []string) {
			bindCentrifugoConfig()
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
			bindCentrifugoConfig()
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
			bindCentrifugoConfig()
			err := readConfig(genTokenConfigFile)
			if err != nil && err != errConfigFileNotFound {
				fmt.Printf("error: %v\n", err)
				os.Exit(1)
			}
			jwtVerifierConfig := jwtVerifierConfig()
			token, err := cli.GenerateToken(jwtVerifierConfig, genTokenUser, genTokenTTL)
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
			bindCentrifugoConfig()
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
			subject, claims, err := cli.CheckToken(jwtVerifierConfig, ruleConfig(), args[0])
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

	var serveDir string
	var servePort int
	var serveAddr string

	var serveCmd = &cobra.Command{
		Use:   "serve",
		Short: "Run static file server",
		Long:  `Run static file server`,
		Run: func(cmd *cobra.Command, args []string) {
			address := net.JoinHostPort(serveAddr, strconv.Itoa(servePort))
			fmt.Printf("start serving %s on %s\n", serveDir, address)
			if err := http.ListenAndServe(address, http.FileServer(http.Dir(serveDir))); err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
		},
	}
	serveCmd.Flags().StringVarP(&serveDir, "dir", "d", "./", "path to directory")
	serveCmd.Flags().IntVarP(&servePort, "port", "p", 3000, "port to serve on")
	serveCmd.Flags().StringVarP(&serveAddr, "address", "a", "", "interface to serve on (default: all interfaces)")

	rootCmd.AddCommand(serveCmd)
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
	return os.WriteFile(pidFile, pid, 0644)
}

var logLevelMatches = map[string]zerolog.Level{
	"NONE":  zerolog.NoLevel,
	"TRACE": zerolog.TraceLevel,
	"DEBUG": zerolog.DebugLevel,
	"INFO":  zerolog.InfoLevel,
	"WARN":  zerolog.WarnLevel,
	"ERROR": zerolog.ErrorLevel,
	"FATAL": zerolog.FatalLevel,
}

func configureConsoleWriter() {
	if isTerminalAttached() {
		log.Logger = log.Output(zerolog.ConsoleWriter{
			Out:                 os.Stdout,
			TimeFormat:          "2006-01-02 15:04:05",
			FormatLevel:         logutils.ConsoleFormatLevel(),
			FormatErrFieldName:  logutils.ConsoleFormatErrFieldName(),
			FormatErrFieldValue: logutils.ConsoleFormatErrFieldValue(),
		})
	}
}

func isTerminalAttached() bool {
	return isatty.IsTerminal(os.Stdout.Fd()) && runtime.GOOS != "windows"
}

func setupLogging() *os.File {
	configureConsoleWriter()

	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	logLevel, ok := logLevelMatches[strings.ToUpper(viper.GetString("log_level"))]
	if !ok {
		logLevel = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(logLevel)

	if viper.IsSet("log_file") && viper.GetString("log_file") != "" {
		f, err := os.OpenFile(viper.GetString("log_file"), os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
		if err != nil {
			log.Fatal().Msgf("error opening log file: %v", err)
		}
		log.Logger = log.Output(f)
		return f
	}

	return nil
}

func handleSignals(configFile string, n *centrifuge.Node, ruleContainer *rule.Container, tokenVerifier *jwtverify.VerifierJWT, httpServers []*http.Server, grpcAPIServer *grpc.Server, grpcUniServer *grpc.Server, exporter *graphite.Exporter) {
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
			shutdownTimeout := GetDuration("shutdown_timeout")
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

			if grpcUniServer != nil {
				wg.Add(1)
				go func() {
					defer wg.Done()
					grpcUniServer.GracefulStop()
				}()
			}

			ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)

			for _, srv := range httpServers {
				wg.Add(1)
				go func(srv *http.Server) {
					defer wg.Done()
					_ = srv.Shutdown(ctx)
				}(srv)
			}

			_ = n.Shutdown(ctx)

			wg.Wait()
			cancel()

			if pidFile != "" {
				_ = os.Remove(pidFile)
			}
			time.Sleep(GetDuration("shutdown_termination_delay"))
			os.Exit(0)
		}
	}
}

var startHTTPChallengeServerOnce sync.Once

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
	tlsAutocertServerName := viper.GetString("tls_autocert_server_name")
	tlsAutocertHTTP := viper.GetBool("tls_autocert_http")
	tlsAutocertHTTPAddr := viper.GetString("tls_autocert_http_addr")

	if tlsAutocertEnabled {
		certManager := autocert.Manager{
			Prompt: autocert.AcceptTOS,
			Email:  tlsAutocertEmail,
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

func tlsConfigForUniGRPC() (*tls.Config, error) {
	tlsCert := viper.GetString("uni_grpc_tls_cert")
	tlsKey := viper.GetString("uni_grpc_tls_key")
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
	if viper.GetBool("sockjs") {
		portFlags |= HandlerSockJS
	}
	if useAdmin && adminExternal {
		portFlags |= HandlerAdmin
	}
	if viper.GetBool("uni_websocket") {
		portFlags |= HandlerUniWebsocket
	}
	if viper.GetBool("uni_sse") {
		portFlags |= HandlerUniSSE
	}
	if viper.GetBool("uni_http_stream") {
		portFlags |= HandlerUniHTTPStream
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

func ruleConfig() rule.Config {
	v := viper.GetViper()
	cfg := rule.Config{}

	cfg.Publish = v.GetBool("publish")
	cfg.SubscribeToPublish = v.GetBool("subscribe_to_publish")
	cfg.Anonymous = v.GetBool("anonymous")
	cfg.Presence = v.GetBool("presence")
	cfg.PresenceDisableForClient = v.GetBool("presence_disable_for_client")
	cfg.JoinLeave = v.GetBool("join_leave")
	cfg.HistorySize = v.GetInt("history_size")
	cfg.HistoryTTL = tools.Duration(GetDuration("history_ttl", true))
	cfg.Position = v.GetBool("position")
	cfg.Recover = v.GetBool("recover")
	cfg.HistoryDisableForClient = v.GetBool("history_disable_for_client")
	cfg.Protected = v.GetBool("protected")
	cfg.ProxySubscribe = v.GetBool("proxy_subscribe")
	cfg.ProxyPublish = v.GetBool("proxy_publish")
	cfg.Namespaces = namespacesFromConfig(v)
	cfg.ChannelPrivatePrefix = v.GetString("channel_private_prefix")
	cfg.ChannelNamespaceBoundary = v.GetString("channel_namespace_boundary")
	cfg.ChannelUserBoundary = v.GetString("channel_user_boundary")
	cfg.ChannelUserSeparator = v.GetString("channel_user_separator")
	cfg.UserSubscribeToPersonal = v.GetBool("user_subscribe_to_personal")
	cfg.UserPersonalSingleConnection = v.GetBool("user_personal_single_connection")
	cfg.UserPersonalChannelNamespace = v.GetString("user_personal_channel_namespace")
	cfg.ClientInsecure = v.GetBool("client_insecure")
	cfg.ClientAnonymous = v.GetBool("client_anonymous")
	cfg.ClientConcurrency = v.GetInt("client_concurrency")
	return cfg
}

func jwtVerifierConfig() jwtverify.VerifierConfig {
	v := viper.GetViper()
	cfg := jwtverify.VerifierConfig{}

	cfg.HMACSecretKey = v.GetString("token_hmac_secret_key")

	rsaPublicKey := v.GetString("token_rsa_public_key")
	if rsaPublicKey != "" {
		pubKey, err := jwtutils.ParseRSAPublicKeyFromPEM([]byte(rsaPublicKey))
		if err != nil {
			log.Fatal().Msgf("error parsing RSA public key: %v", err)
		}
		cfg.RSAPublicKey = pubKey
	}

	ecdsaPublicKey := v.GetString("token_ecdsa_public_key")
	if ecdsaPublicKey != "" {
		pubKey, err := jwtutils.ParseECDSAPublicKeyFromPEM([]byte(ecdsaPublicKey))
		if err != nil {
			log.Fatal().Msgf("error parsing ECDSA public key: %v", err)
		}
		cfg.ECDSAPublicKey = pubKey
	}

	cfg.JWKSPublicEndpoint = v.GetString("token_jwks_public_endpoint")

	return cfg
}

func GetDuration(key string, secondsPrecision ...bool) time.Duration {
	durationString := viper.GetString(key)
	duration, err := time.ParseDuration(durationString)
	if err != nil {
		log.Fatal().Msgf("malformed duration for key '%s': %v", key, err)
	}
	if duration > 0 && duration < time.Millisecond {
		log.Fatal().Msgf("malformed duration for key '%s': %s, minimal duration resolution is 1ms – make sure correct time unit set", key, duration)
	}
	if duration > 0 && duration < time.Second && len(secondsPrecision) > 0 && secondsPrecision[0] {
		log.Fatal().Msgf("malformed duration for key '%s': %s, minimal duration resolution is 1s for this key", key, duration)
	}
	if duration > 0 && duration%time.Second != 0 && len(secondsPrecision) > 0 && secondsPrecision[0] {
		log.Fatal().Msgf("malformed duration for key '%s': %s, sub-second precision is not supported for this key", key, duration)
	}
	return duration
}

func proxyConfig() (proxy.Config, bool) {
	v := viper.GetViper()
	cfg := proxy.Config{}
	cfg.GRPCMetadata = v.GetStringSlice("proxy_grpc_metadata")
	cfg.HTTPHeaders = v.GetStringSlice("proxy_http_headers")
	cfg.BinaryEncoding = v.GetBool("proxy_binary_encoding")
	cfg.ConnectEndpoint = v.GetString("proxy_connect_endpoint")
	cfg.ConnectTimeout = GetDuration("proxy_connect_timeout")
	cfg.RefreshEndpoint = v.GetString("proxy_refresh_endpoint")
	cfg.RefreshTimeout = GetDuration("proxy_refresh_timeout")
	cfg.RPCEndpoint = v.GetString("proxy_rpc_endpoint")
	cfg.RPCTimeout = GetDuration("proxy_rpc_timeout")
	cfg.SubscribeEndpoint = v.GetString("proxy_subscribe_endpoint")
	cfg.SubscribeTimeout = GetDuration("proxy_subscribe_timeout")
	cfg.PublishEndpoint = v.GetString("proxy_publish_endpoint")
	cfg.PublishTimeout = GetDuration("proxy_publish_timeout")
	cfg.IncludeConnectionMeta = v.GetBool("proxy_include_connection_meta")

	var httpConfig proxy.HTTPConfig
	httpConfig.Encoder = &proxyproto.JSONEncoder{}
	httpConfig.Decoder = &proxyproto.JSONDecoder{}
	cfg.HTTPConfig = httpConfig

	grpcConfig := proxy.GRPCConfig{
		CertFile:         v.GetString("proxy_grpc_cert_file"),
		CredentialsKey:   v.GetString("proxy_grpc_credentials_key"),
		CredentialsValue: v.GetString("proxy_grpc_credentials_value"),
	}
	grpcConfig.Codec = &proxyproto.Codec{}
	cfg.GRPCConfig = grpcConfig

	proxyEnabled := cfg.ConnectEndpoint != "" || cfg.RefreshEndpoint != "" ||
		cfg.RPCEndpoint != "" || cfg.SubscribeEndpoint != "" || cfg.PublishEndpoint != ""

	return cfg, proxyEnabled
}

func nodeConfig(version string) centrifuge.Config {
	v := viper.GetViper()
	cfg := centrifuge.Config{}
	cfg.Version = version
	cfg.MetricsNamespace = "centrifugo"
	cfg.Name = applicationName()
	cfg.ChannelMaxLength = v.GetInt("channel_max_length")
	cfg.ClientPresenceUpdateInterval = GetDuration("client_presence_update_interval")
	cfg.ClientExpiredCloseDelay = GetDuration("client_expired_close_delay")
	cfg.ClientExpiredSubCloseDelay = GetDuration("client_expired_sub_close_delay")
	cfg.ClientStaleCloseDelay = GetDuration("client_stale_close_delay")
	cfg.ClientQueueMaxSize = v.GetInt("client_queue_max_size")
	cfg.ClientChannelLimit = v.GetInt("client_channel_limit")
	cfg.ClientChannelPositionCheckDelay = GetDuration("client_channel_position_check_delay")
	cfg.UserConnectionLimit = v.GetInt("client_user_connection_limit")
	cfg.NodeInfoMetricsAggregateInterval = GetDuration("node_info_metrics_aggregate_interval")
	cfg.HistoryMaxPublicationLimit = v.GetInt("client_history_max_publication_limit")
	cfg.RecoveryMaxPublicationLimit = v.GetInt("client_recovery_max_publication_limit")

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
	"trace": centrifuge.LogLevelTrace,
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
		decoderCfg := tools.DecoderConfig(&ns)
		decoder, newErr := mapstructure.NewDecoder(decoderCfg)
		if newErr != nil {
			log.Fatal().Msg(newErr.Error())
			return ns
		}
		err = decoder.Decode(v.Get("namespaces"))
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
	cfg.PingInterval = GetDuration("websocket_ping_interval")
	cfg.WriteTimeout = GetDuration("websocket_write_timeout")
	cfg.MessageSizeLimit = v.GetInt("websocket_message_size_limit")
	cfg.CheckOrigin = getCheckOrigin()
	return cfg
}

var warnAllowedOriginsOnce sync.Once

func getCheckOrigin() func(r *http.Request) bool {
	v := viper.GetViper()
	if !v.IsSet("allowed_origins") {
		log.Fatal().Msg("allowed_origins not set")
	}
	allowedOrigins := v.GetStringSlice("allowed_origins")
	if len(allowedOrigins) == 0 {
		log.Fatal().Msg("allowed_origins can't be empty")
	}
	originChecker, err := origin.NewPatternChecker(allowedOrigins)
	if err != nil {
		log.Fatal().Msgf("error creating origin checker: %v", err)
	}
	if len(allowedOrigins) == 1 && allowedOrigins[0] == "*" {
		// Fast path for *.
		warnAllowedOriginsOnce.Do(func() {
			log.Warn().Msgf("usage of allowed_origins * is discouraged for security reasons, consider setting exact list of origins")
		})
		return func(r *http.Request) bool {
			return true
		}
	}
	return func(r *http.Request) bool {
		err := originChecker.Check(r)
		if err != nil {
			log.Info().Str("error", err.Error()).Strs("allowed_origins", allowedOrigins).Msg("error checking request origin")
			return false
		}
		return true
	}
}

func uniWebsocketHandlerConfig() uniws.Config {
	v := viper.GetViper()
	return uniws.Config{
		Compression:        v.GetBool("uni_websocket_compression"),
		CompressionLevel:   v.GetInt("uni_websocket_compression_level"),
		CompressionMinSize: v.GetInt("uni_websocket_compression_min_size"),
		ReadBufferSize:     v.GetInt("uni_websocket_read_buffer_size"),
		WriteBufferSize:    v.GetInt("uni_websocket_write_buffer_size"),
		UseWriteBufferPool: v.GetBool("uni_websocket_use_write_buffer_pool"),
		PingInterval:       GetDuration("uni_websocket_ping_interval"),
		WriteTimeout:       GetDuration("uni_websocket_write_timeout"),
		MessageSizeLimit:   v.GetInt("uni_websocket_message_size_limit"),
		CheckOrigin:        getCheckOrigin(),
	}
}

func uniSSEHandlerConfig() unisse.Config {
	return unisse.Config{
		MaxRequestBodySize: viper.GetInt("uni_sse_max_request_body_size"),
	}
}

func uniStreamHandlerConfig() unihttpstream.Config {
	return unihttpstream.Config{
		MaxRequestBodySize: viper.GetInt("uni_http_stream_max_request_body_size"),
	}
}

func uniGRPCHandlerConfig() unigrpc.Config {
	return unigrpc.Config{}
}

func sockjsHandlerConfig() centrifuge.SockjsConfig {
	v := viper.GetViper()
	cfg := centrifuge.SockjsConfig{}
	cfg.URL = v.GetString("sockjs_url")
	cfg.HeartbeatDelay = GetDuration("sockjs_heartbeat_delay")
	cfg.WebsocketReadBufferSize = v.GetInt("websocket_read_buffer_size")
	cfg.WebsocketWriteBufferSize = v.GetInt("websocket_write_buffer_size")
	cfg.WebsocketUseWriteBufferPool = v.GetBool("websocket_use_write_buffer_pool")
	cfg.WebsocketWriteTimeout = GetDuration("websocket_write_timeout")
	cfg.CheckOrigin = getCheckOrigin()
	cfg.WebsocketCheckOrigin = getCheckOrigin()
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

func memoryEngine(n *centrifuge.Node) (centrifuge.Broker, centrifuge.PresenceManager, error) {
	brokerConf, err := memoryBrokerConfig()
	if err != nil {
		return nil, nil, err
	}
	broker, err := centrifuge.NewMemoryBroker(n, *brokerConf)
	if err != nil {
		return nil, nil, err
	}
	presenceManagerConf, err := memoryPresenceManagerConfig()
	if err != nil {
		return nil, nil, err
	}
	presenceManager, err := centrifuge.NewMemoryPresenceManager(n, *presenceManagerConf)
	if err != nil {
		return nil, nil, err
	}
	return broker, presenceManager, nil
}

func memoryBrokerConfig() (*centrifuge.MemoryBrokerConfig, error) {
	return &centrifuge.MemoryBrokerConfig{
		HistoryMetaTTL: GetDuration("history_meta_ttl", true),
	}, nil
}

func memoryPresenceManagerConfig() (*centrifuge.MemoryPresenceManagerConfig, error) {
	return &centrifuge.MemoryPresenceManagerConfig{}, nil
}

func addRedisShardCommonSettings(shardConf *centrifuge.RedisShardConfig) {
	shardConf.Password = viper.GetString("redis_password")
	shardConf.UseTLS = viper.GetBool("redis_tls")
	shardConf.TLSSkipVerify = viper.GetBool("redis_tls_skip_verify")
	shardConf.IdleTimeout = GetDuration("redis_idle_timeout")
	shardConf.ConnectTimeout = GetDuration("redis_connect_timeout")
	shardConf.ReadTimeout = GetDuration("redis_read_timeout")
	shardConf.WriteTimeout = GetDuration("redis_write_timeout")
}

func getRedisShardConfigs() ([]centrifuge.RedisShardConfig, error) {
	var shardConfigs []centrifuge.RedisShardConfig

	clusterShards := viper.GetStringSlice("redis_cluster_address")
	var useCluster bool
	if len(clusterShards) > 0 {
		useCluster = true
	}

	if useCluster {
		for _, clusterAddress := range clusterShards {
			clusterAddresses := strings.Split(clusterAddress, ",")
			for _, address := range clusterAddresses {
				if _, _, err := net.SplitHostPort(address); err != nil {
					return nil, fmt.Errorf("malformed Redis Cluster address: %s", address)
				}
			}
			conf := &centrifuge.RedisShardConfig{
				ClusterAddresses: clusterAddresses,
			}
			addRedisShardCommonSettings(conf)
			shardConfigs = append(shardConfigs, *conf)
		}
		return shardConfigs, nil
	}

	sentinelShards := viper.GetStringSlice("redis_sentinel_address")
	var useSentinel bool
	if len(sentinelShards) > 0 {
		useSentinel = true
	}

	if useSentinel {
		for _, sentinelAddress := range sentinelShards {
			sentinelAddresses := strings.Split(sentinelAddress, ",")
			for _, address := range sentinelAddresses {
				if _, _, err := net.SplitHostPort(address); err != nil {
					return nil, fmt.Errorf("malformed Redis Sentinel address: %s", address)
				}
			}
			conf := &centrifuge.RedisShardConfig{
				SentinelAddresses: sentinelAddresses,
			}
			addRedisShardCommonSettings(conf)
			conf.SentinelPassword = viper.GetString("redis_sentinel_password")
			conf.SentinelMasterName = viper.GetString("redis_sentinel_master_name")
			if conf.SentinelMasterName == "" {
				return nil, fmt.Errorf("master name must be set when using Redis Sentinel")
			}
			shardConfigs = append(shardConfigs, *conf)
		}
		return shardConfigs, nil
	}

	redisAddresses := viper.GetStringSlice("redis_address")
	if len(redisAddresses) == 0 {
		redisAddresses = []string{"127.0.0.1:6379"}
	}
	for _, redisAddress := range redisAddresses {
		conf := &centrifuge.RedisShardConfig{
			Address: redisAddress,
		}
		addRedisShardCommonSettings(conf)
		shardConfigs = append(shardConfigs, *conf)
	}
	return shardConfigs, nil
}

func getRedisShards(n *centrifuge.Node) ([]*centrifuge.RedisShard, error) {
	redisShardConfigs, err := getRedisShardConfigs()
	if err != nil {
		return nil, err
	}
	redisShards := make([]*centrifuge.RedisShard, 0, len(redisShardConfigs))

	for _, redisConf := range redisShardConfigs {
		redisShard, err := centrifuge.NewRedisShard(n, redisConf)
		if err != nil {
			return nil, err
		}
		redisShards = append(redisShards, redisShard)
	}
	return redisShards, nil
}

func redisEngine(n *centrifuge.Node) (centrifuge.Broker, centrifuge.PresenceManager, error) {
	redisShards, err := getRedisShards(n)
	if err != nil {
		return nil, nil, err
	}

	broker, err := centrifuge.NewRedisBroker(n, centrifuge.RedisBrokerConfig{
		Shards:         redisShards,
		Prefix:         viper.GetString("redis_prefix"),
		UseLists:       viper.GetBool("redis_use_lists"),
		HistoryMetaTTL: GetDuration("history_meta_ttl", true),
	})
	if err != nil {
		return nil, nil, err
	}

	presenceManager, err := centrifuge.NewRedisPresenceManager(n, centrifuge.RedisPresenceManagerConfig{
		Shards:      redisShards,
		Prefix:      viper.GetString("redis_prefix"),
		PresenceTTL: GetDuration("presence_ttl", true),
	})
	if err != nil {
		return nil, nil, err
	}

	return broker, presenceManager, nil
}

func getTarantoolShardConfigs() ([]tntengine.ShardConfig, error) {
	var shardConfigs []tntengine.ShardConfig

	mode := tntengine.ConnectionModeSingleInstance
	if viper.IsSet("tarantool_mode") {
		switch viper.GetString("tarantool_mode") {
		case "standalone":
			// default.
		case "leader-follower":
			mode = tntengine.ConnectionModeLeaderFollower
		case "leader-follower-raft":
			mode = tntengine.ConnectionModeLeaderFollowerRaft
		default:
			return nil, fmt.Errorf("unknown Tarantool mode: %s", viper.GetString("tarantool_mode"))
		}
	}

	var shardAddresses [][]string

	tarantoolAddresses := viper.GetStringSlice("tarantool_address")
	for _, shardPart := range tarantoolAddresses {
		shardAddresses = append(shardAddresses, strings.Split(shardPart, ","))
	}

	for _, tarantoolAddresses := range shardAddresses {
		conf := &tntengine.ShardConfig{
			Addresses:      tarantoolAddresses,
			User:           viper.GetString("tarantool_user"),
			Password:       viper.GetString("tarantool_password"),
			ConnectionMode: mode,
		}
		shardConfigs = append(shardConfigs, *conf)
	}
	return shardConfigs, nil
}

func getTarantoolShards() ([]*tntengine.Shard, error) {
	tarantoolShardConfigs, err := getTarantoolShardConfigs()
	if err != nil {
		return nil, err
	}
	tarantoolShards := make([]*tntengine.Shard, 0, len(tarantoolShardConfigs))

	for _, tarantoolConf := range tarantoolShardConfigs {
		tarantoolShard, err := tntengine.NewShard(tarantoolConf)
		if err != nil {
			return nil, err
		}
		tarantoolShards = append(tarantoolShards, tarantoolShard)
	}
	return tarantoolShards, nil
}

func tarantoolEngine(n *centrifuge.Node) (centrifuge.Broker, centrifuge.PresenceManager, error) {
	tarantoolShards, err := getTarantoolShards()
	if err != nil {
		return nil, nil, err
	}
	broker, err := tntengine.NewBroker(n, tntengine.BrokerConfig{
		Shards:         tarantoolShards,
		HistoryMetaTTL: GetDuration("history_meta_ttl", true),
	})
	if err != nil {
		return nil, nil, err
	}
	presenceManager, err := tntengine.NewPresenceManager(n, tntengine.PresenceManagerConfig{
		Shards:      tarantoolShards,
		PresenceTTL: GetDuration("presence_ttl", true),
	})
	if err != nil {
		return nil, nil, err
	}
	return broker, presenceManager, nil
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
		case centrifuge.LogLevelTrace:
			l = log.Trace()
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
	// HandlerUniWebsocket enables unidirectional websocket endpoint.
	HandlerUniWebsocket
	// HandlerUniSSE enables unidirectional SSE endpoint.
	HandlerUniSSE
	// HandlerUniHTTPStream enables unidirectional stream endpoint.
	HandlerUniHTTPStream
)

var handlerText = map[HandlerFlag]string{
	HandlerWebsocket:     "websocket",
	HandlerSockJS:        "SockJS",
	HandlerAPI:           "API",
	HandlerAdmin:         "admin",
	HandlerDebug:         "debug",
	HandlerPrometheus:    "prometheus",
	HandlerHealth:        "health",
	HandlerUniWebsocket:  "uni_websocket",
	HandlerUniSSE:        "uni_sse",
	HandlerUniHTTPStream: "uni_http_stream",
}

func (flags HandlerFlag) String() string {
	flagsOrdered := []HandlerFlag{HandlerWebsocket, HandlerSockJS, HandlerAPI, HandlerAdmin, HandlerPrometheus, HandlerDebug, HandlerHealth, HandlerUniWebsocket, HandlerUniSSE, HandlerUniHTTPStream}
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
		// register WebSocket connection endpoint.
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

	if flags&HandlerUniWebsocket != 0 {
		// register unidirectional WebSocket connection endpoint.
		wsPrefix := strings.TrimRight(v.GetString("uni_websocket_handler_prefix"), "/")
		if wsPrefix == "" {
			wsPrefix = "/"
		}
		mux.Handle(wsPrefix, middleware.LogRequest(middleware.HeadersToContext(proxyEnabled, uniws.NewHandler(n, uniWebsocketHandlerConfig()))))
	}

	if flags&HandlerUniSSE != 0 {
		// register unidirectional SSE connection endpoint.
		ssePrefix := strings.TrimRight(v.GetString("uni_sse_handler_prefix"), "/")
		if ssePrefix == "" {
			ssePrefix = "/"
		}
		mux.Handle(ssePrefix, middleware.LogRequest(middleware.HeadersToContext(proxyEnabled, middleware.CORS(getCheckOrigin(), unisse.NewHandler(n, uniSSEHandlerConfig())))))
	}

	if flags&HandlerUniHTTPStream != 0 {
		// register unidirectional HTTP stream connection endpoint.
		streamPrefix := strings.TrimRight(v.GetString("uni_http_stream_handler_prefix"), "/")
		if streamPrefix == "" {
			streamPrefix = "/"
		}
		mux.Handle(streamPrefix, middleware.LogRequest(middleware.HeadersToContext(proxyEnabled, middleware.CORS(getCheckOrigin(), unihttpstream.NewHandler(n, uniStreamHandlerConfig())))))
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
		mux.Handle(adminPrefix+"/", admin.NewHandler(n, apiExecutor, adminHandlerConfig()))
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
