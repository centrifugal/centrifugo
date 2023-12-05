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
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/centrifugal/centrifugo/v5/internal/admin"
	"github.com/centrifugal/centrifugo/v5/internal/api"
	"github.com/centrifugal/centrifugo/v5/internal/build"
	"github.com/centrifugal/centrifugo/v5/internal/cli"
	"github.com/centrifugal/centrifugo/v5/internal/client"
	"github.com/centrifugal/centrifugo/v5/internal/health"
	"github.com/centrifugal/centrifugo/v5/internal/jwtutils"
	"github.com/centrifugal/centrifugo/v5/internal/jwtverify"
	"github.com/centrifugal/centrifugo/v5/internal/logutils"
	"github.com/centrifugal/centrifugo/v5/internal/metrics/graphite"
	"github.com/centrifugal/centrifugo/v5/internal/middleware"
	"github.com/centrifugal/centrifugo/v5/internal/natsbroker"
	"github.com/centrifugal/centrifugo/v5/internal/notify"
	"github.com/centrifugal/centrifugo/v5/internal/origin"
	"github.com/centrifugal/centrifugo/v5/internal/proxy"
	"github.com/centrifugal/centrifugo/v5/internal/rule"
	"github.com/centrifugal/centrifugo/v5/internal/survey"
	"github.com/centrifugal/centrifugo/v5/internal/swaggerui"
	"github.com/centrifugal/centrifugo/v5/internal/telemetry"
	"github.com/centrifugal/centrifugo/v5/internal/tntengine"
	"github.com/centrifugal/centrifugo/v5/internal/tools"
	"github.com/centrifugal/centrifugo/v5/internal/unigrpc"
	"github.com/centrifugal/centrifugo/v5/internal/unihttpstream"
	"github.com/centrifugal/centrifugo/v5/internal/unisse"
	"github.com/centrifugal/centrifugo/v5/internal/uniws"
	"github.com/centrifugal/centrifugo/v5/internal/usage"
	"github.com/centrifugal/centrifugo/v5/internal/webui"
	"github.com/centrifugal/centrifugo/v5/internal/wt"

	"github.com/FZambia/viper-lite"
	"github.com/centrifugal/centrifuge"
	"github.com/justinas/alice"
	"github.com/mattn/go-isatty"
	"github.com/mitchellh/mapstructure"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/quic-go/quic-go/http3"
	"github.com/quic-go/webtransport-go"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.uber.org/automaxprocs/maxprocs"
	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	_ "google.golang.org/grpc/encoding/gzip"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/reflection"
)

//go:generate go run internal/gen/api/main.go

var defaults = map[string]any{
	"gomaxprocs": 0,
	"name":       "",
	"engine":     "memory",
	"broker":     "",

	"pid_file": "",

	"granular_proxy_mode": false,

	"opentelemetry":     false,
	"opentelemetry_api": false,

	"client_insecure": false,
	"client_insecure_skip_token_signature_verify": false,
	"api_insecure": false,

	"client_user_id_http_header": "",

	"token_hmac_secret_key":      "",
	"token_rsa_public_key":       "",
	"token_ecdsa_public_key":     "",
	"token_jwks_public_endpoint": "",
	"token_audience":             "",
	"token_audience_regex":       "",
	"token_issuer":               "",
	"token_issuer_regex":         "",

	"separate_subscription_token_config":      false,
	"subscription_token_hmac_secret_key":      "",
	"subscription_token_rsa_public_key":       "",
	"subscription_token_ecdsa_public_key":     "",
	"subscription_token_jwks_public_endpoint": "",
	"subscription_token_audience":             "",
	"subscription_token_audience_regex":       "",
	"subscription_token_issuer":               "",
	"subscription_token_issuer_regex":         "",

	"allowed_origins": []string{},

	"global_history_meta_ttl": 30 * 24 * time.Hour,
	"global_presence_ttl":     60 * time.Second,

	"presence":                      false,
	"join_leave":                    false,
	"force_push_join_leave":         false,
	"history_size":                  0,
	"history_ttl":                   0,
	"history_meta_ttl":              0,
	"force_positioning":             false,
	"allow_positioning":             false,
	"force_recovery":                false,
	"allow_recovery":                false,
	"allow_subscribe_for_anonymous": false,
	"allow_subscribe_for_client":    false,
	"allow_publish_for_anonymous":   false,
	"allow_publish_for_client":      false,
	"allow_publish_for_subscriber":  false,
	"allow_presence_for_anonymous":  false,
	"allow_presence_for_client":     false,
	"allow_presence_for_subscriber": false,
	"allow_history_for_anonymous":   false,
	"allow_history_for_client":      false,
	"allow_history_for_subscriber":  false,
	"allow_user_limited_channels":   false,
	"channel_regex":                 "",
	"proxy_subscribe":               false,
	"proxy_publish":                 false,
	"proxy_sub_refresh":             false,
	"subscribe_proxy_name":          "",
	"publish_proxy_name":            "",
	"sub_refresh_proxy_name":        "",

	"node_info_metrics_aggregate_interval": 60 * time.Second,

	"allow_anonymous_connect_without_token": false,
	"disallow_anonymous_connection_tokens":  false,

	"client_expired_close_delay":          25 * time.Second,
	"client_expired_sub_close_delay":      25 * time.Second,
	"client_stale_close_delay":            10 * time.Second,
	"client_channel_limit":                128,
	"client_queue_max_size":               1048576, // 1 MB
	"client_presence_update_interval":     27 * time.Second,
	"client_user_connection_limit":        0,
	"client_concurrency":                  0,
	"client_channel_position_check_delay": 40 * time.Second,
	"client_connection_limit":             0,
	"client_connection_rate_limit":        0,

	"channel_max_length":         255,
	"channel_private_prefix":     "$",
	"channel_namespace_boundary": ":",
	"channel_user_boundary":      "#",
	"channel_user_separator":     ",",

	"rpc_namespace_boundary": ":",

	"user_subscribe_to_personal":      false,
	"user_personal_channel_namespace": "",
	"user_personal_single_connection": false,

	"debug":      false,
	"prometheus": false,
	"health":     false,

	"admin":                   false,
	"admin_password":          "",
	"admin_secret":            "",
	"admin_insecure":          false,
	"admin_web_path":          "",
	"admin_web_proxy_address": "",

	"sockjs":     false,
	"sockjs_url": "https://cdn.jsdelivr.net/npm/sockjs-client@1/dist/sockjs.min.js",

	"websocket_compression":           false,
	"websocket_compression_min_size":  0,
	"websocket_compression_level":     1,
	"websocket_read_buffer_size":      0,
	"websocket_use_write_buffer_pool": false,
	"websocket_write_buffer_size":     0,
	"websocket_write_timeout":         time.Second,
	"websocket_message_size_limit":    65536, // 64KB

	"uni_websocket":                       false,
	"uni_websocket_compression":           false,
	"uni_websocket_compression_min_size":  0,
	"uni_websocket_compression_level":     1,
	"uni_websocket_read_buffer_size":      0,
	"uni_websocket_use_write_buffer_pool": false,
	"uni_websocket_write_buffer_size":     0,
	"uni_websocket_write_timeout":         time.Second,
	"uni_websocket_message_size_limit":    65536, // 64KB

	"uni_sse":         false,
	"uni_http_stream": false,

	"log_level": "info",
	"log_file":  "",

	"tls":                      false,
	"tls_key":                  "",
	"tls_cert":                 "",
	"tls_cert_pem":             "",
	"tls_key_pem":              "",
	"tls_root_ca":              "",
	"tls_root_ca_pem":          "",
	"tls_client_ca":            "",
	"tls_client_ca_pem":        "",
	"tls_server_name":          "",
	"tls_insecure_skip_verify": false,

	"swagger":          false,
	"admin_external":   false,
	"api_external":     false,
	"address":          "",
	"port":             "8000",
	"internal_address": "",
	"internal_port":    "",

	"webtransport": false,
	"http3":        false,

	"tls_external": false,

	"connect_proxy_name": "",
	"refresh_proxy_name": "",
	"rpc_proxy_name":     "",

	"proxy_connect_endpoint":          "",
	"proxy_refresh_endpoint":          "",
	"proxy_subscribe_endpoint":        "",
	"proxy_publish_endpoint":          "",
	"proxy_sub_refresh_endpoint":      "",
	"proxy_rpc_endpoint":              "",
	"proxy_subscribe_stream_endpoint": "",

	"proxy_connect_timeout":          time.Second,
	"proxy_rpc_timeout":              time.Second,
	"proxy_refresh_timeout":          time.Second,
	"proxy_subscribe_timeout":        time.Second,
	"proxy_publish_timeout":          time.Second,
	"proxy_sub_refresh_timeout":      time.Second,
	"proxy_subscribe_stream_timeout": time.Second,

	"proxy_grpc_metadata":           []string{},
	"proxy_http_headers":            []string{},
	"proxy_static_http_headers":     map[string]string{},
	"proxy_binary_encoding":         false,
	"proxy_include_connection_meta": false,
	"proxy_grpc_cert_file":          "",
	"proxy_grpc_compression":        false,

	"tarantool_mode":     "standalone",
	"tarantool_address":  "tcp://127.0.0.1:3301",
	"tarantool_user":     "",
	"tarantool_password": "",

	"api_key":        "",
	"api_error_mode": "",

	"uni_http_stream_max_request_body_size": 65536, // 64KB
	"uni_sse_max_request_body_size":         65536, // 64KB
	"http_stream_max_request_body_size":     65536, // 64KB
	"sse_max_request_body_size":             65536, // 64KB

	"tls_autocert":                false,
	"tls_autocert_host_whitelist": "",
	"tls_autocert_cache_dir":      "",
	"tls_autocert_email":          "",
	"tls_autocert_server_name":    "",
	"tls_autocert_http":           false,
	"tls_autocert_http_addr":      ":80",

	"grpc_api":                          false,
	"grpc_api_error_mode":               "",
	"grpc_api_address":                  "",
	"grpc_api_port":                     10000,
	"grpc_api_key":                      "",
	"grpc_api_tls_disable":              false,
	"grpc_api_reflection":               false,
	"grpc_api_tls":                      false,
	"grpc_api_tls_key":                  "",
	"grpc_api_tls_cert":                 "",
	"grpc_api_tls_cert_pem":             "",
	"grpc_api_tls_key_pem":              "",
	"grpc_api_tls_root_ca":              "",
	"grpc_api_tls_root_ca_pem":          "",
	"grpc_api_tls_client_ca":            "",
	"grpc_api_tls_client_ca_pem":        "",
	"grpc_api_tls_server_name":          "",
	"grpc_api_tls_insecure_skip_verify": false,

	"shutdown_timeout":           30 * time.Second,
	"shutdown_termination_delay": 0,

	"graphite":          false,
	"graphite_host":     "localhost",
	"graphite_port":     2003,
	"graphite_prefix":   "centrifugo",
	"graphite_interval": 10 * time.Second,
	"graphite_tags":     false,

	"nats_prefix":        "centrifugo",
	"nats_url":           "nats://127.0.0.1:4222",
	"nats_dial_timeout":  time.Second,
	"nats_write_timeout": time.Second,

	"websocket_disable": false,
	"api_disable":       false,

	"websocket_handler_prefix":       "/connection/websocket",
	"webtransport_handler_prefix":    "/connection/webtransport",
	"sockjs_handler_prefix":          "/connection/sockjs",
	"http_stream_handler_prefix":     "/connection/http_stream",
	"sse_handler_prefix":             "/connection/sse",
	"uni_websocket_handler_prefix":   "/connection/uni_websocket",
	"uni_sse_handler_prefix":         "/connection/uni_sse",
	"uni_http_stream_handler_prefix": "/connection/uni_http_stream",

	"uni_grpc":                          false,
	"uni_grpc_address":                  "",
	"uni_grpc_port":                     11000,
	"uni_grpc_max_receive_message_size": 65536,
	"uni_grpc_tls_disable":              false,
	"uni_grpc_tls":                      false,
	"uni_grpc_tls_key":                  "",
	"uni_grpc_tls_cert":                 "",
	"uni_grpc_tls_cert_pem":             "",
	"uni_grpc_tls_key_pem":              "",
	"uni_grpc_tls_root_ca":              "",
	"uni_grpc_tls_root_ca_pem":          "",
	"uni_grpc_tls_client_ca":            "",
	"uni_grpc_tls_client_ca_pem":        "",
	"uni_grpc_tls_server_name":          "",
	"uni_grpc_tls_insecure_skip_verify": false,

	"http_stream": false,
	"sse":         false,

	"emulation_handler_prefix":        "/emulation",
	"emulation_max_request_body_size": 65536, // 64KB

	"admin_handler_prefix":      "",
	"api_handler_prefix":        "/api",
	"prometheus_handler_prefix": "/metrics",
	"health_handler_prefix":     "/health",
	"swagger_handler_prefix":    "/swagger",

	"client_history_max_publication_limit":  300,
	"client_recovery_max_publication_limit": 300,

	"usage_stats_disable": false,

	"ping_interval": 25 * time.Second,
	"pong_timeout":  8 * time.Second,

	"namespaces":     []any{},
	"rpc_namespaces": []any{},

	"proxies": []any{},

	"proxy_grpc_credentials_key":   "",
	"proxy_grpc_credentials_value": "",
}

func init() {
	redisConfigPrefixes := []string{
		"",
	}
	for _, prefix := range redisConfigPrefixes {
		keyMap := map[string]any{
			prefix + "redis_address":                           "redis://127.0.0.1:6379",
			prefix + "redis_prefix":                            "centrifugo",
			prefix + "redis_connect_timeout":                   time.Second,
			prefix + "redis_io_timeout":                        4 * time.Second,
			prefix + "redis_use_lists":                         false,
			prefix + "redis_db":                                0,
			prefix + "redis_user":                              "",
			prefix + "redis_password":                          "",
			prefix + "redis_client_name":                       "",
			prefix + "redis_force_resp2":                       false,
			prefix + "redis_cluster_address":                   []string{},
			prefix + "redis_sentinel_address":                  []string{},
			prefix + "redis_sentinel_user":                     "",
			prefix + "redis_sentinel_password":                 "",
			prefix + "redis_sentinel_master_name":              "",
			prefix + "redis_sentinel_client_name":              "",
			prefix + "redis_tls":                               false,
			prefix + "redis_tls_key":                           "",
			prefix + "redis_tls_cert":                          "",
			prefix + "redis_tls_cert_pem":                      "",
			prefix + "redis_tls_key_pem":                       "",
			prefix + "redis_tls_root_ca":                       "",
			prefix + "redis_tls_root_ca_pem":                   "",
			prefix + "redis_tls_client_ca":                     "",
			prefix + "redis_tls_client_ca_pem":                 "",
			prefix + "redis_tls_server_name":                   "",
			prefix + "redis_tls_insecure_skip_verify":          false,
			prefix + "redis_sentinel_tls":                      false,
			prefix + "redis_sentinel_tls_key":                  "",
			prefix + "redis_sentinel_tls_cert":                 "",
			prefix + "redis_sentinel_tls_cert_pem":             "",
			prefix + "redis_sentinel_tls_key_pem":              "",
			prefix + "redis_sentinel_tls_root_ca":              "",
			prefix + "redis_sentinel_tls_root_ca_pem":          "",
			prefix + "redis_sentinel_tls_client_ca":            "",
			prefix + "redis_sentinel_tls_client_ca_pem":        "",
			prefix + "redis_sentinel_tls_server_name":          "",
			prefix + "redis_sentinel_tls_insecure_skip_verify": false,
		}
		for k, v := range keyMap {
			defaults[k] = v
		}
	}
}

func bindCentrifugoConfig() {
	viper.SetEnvPrefix("centrifugo")

	for k, v := range defaults {
		viper.SetDefault(k, v)
	}

	replacer := strings.NewReplacer(".", "_")
	viper.SetEnvKeyReplacer(replacer)
	viper.AutomaticEnv()
}

const edition = "oss"

const transportErrorMode = "transport"

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
				"admin_external", "client_insecure", "admin_insecure", "api_insecure", "api_external",
				"port", "address", "tls", "tls_cert", "tls_key", "tls_external", "internal_port",
				"internal_address", "prometheus", "health", "redis_address", "tarantool_address",
				"broker", "nats_url", "grpc_api", "grpc_api_tls", "grpc_api_tls_disable",
				"grpc_api_tls_cert", "grpc_api_tls_key", "grpc_api_port", "sockjs", "uni_grpc",
				"uni_grpc_port", "uni_websocket", "uni_sse", "uni_http_stream", "sse", "http_stream",
				"swagger",
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
					log.Fatal().Msg(tools.ErrorMessageFromConfigError(err, absConfPath))
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

			if os.Getenv("GOMAXPROCS") == "" {
				if viper.IsSet("gomaxprocs") && viper.GetInt("gomaxprocs") > 0 {
					runtime.GOMAXPROCS(viper.GetInt("gomaxprocs"))
				} else {
					_, _ = maxprocs.Set()
				}
			}

			engineName := viper.GetString("engine")

			log.Info().
				Str("version", build.Version).
				Str("runtime", runtime.Version()).
				Int("pid", os.Getpid()).
				Str("engine", engineName).
				Int("gomaxprocs", runtime.GOMAXPROCS(0)).Msg("starting Centrifugo")

			log.Info().Str("path", absConfPath).Msg("using config file")

			ruleConfig := ruleConfig()
			err = ruleConfig.Validate()
			if err != nil {
				log.Fatal().Msgf("error validating config: %v", err)
			}
			ruleContainer, err := rule.NewContainer(ruleConfig)
			if err != nil {
				log.Fatal().Msgf("error creating config: %v", err)
			}
			ruleContainer.ChannelOptionsCacheTTL = 200 * time.Millisecond

			granularProxyMode := viper.GetBool("granular_proxy_mode")
			var proxyMap *client.ProxyMap
			var proxyEnabled bool
			if granularProxyMode {
				proxyMap, proxyEnabled = granularProxyMapConfig(ruleConfig)
				log.Info().Msg("using granular proxy configuration")
			} else {
				proxyMap, proxyEnabled = proxyMapConfig()
			}

			nodeCfg := nodeConfig(build.Version)

			node, err := centrifuge.New(nodeCfg)
			if err != nil {
				log.Fatal().Msgf("error creating Centrifuge Node: %v", err)
			}

			if viper.GetBool("opentelemetry") {
				_, err := telemetry.SetupTracing(context.Background())
				if err != nil {
					log.Fatal().Msgf("error setting up opentelemetry tracing: %v", err)
				}
			}

			brokerName := viper.GetString("broker")
			if brokerName != "" && brokerName != "nats" {
				log.Fatal().Msgf("unknown broker: %s", brokerName)
			}

			var broker centrifuge.Broker
			var presenceManager centrifuge.PresenceManager

			var engineMode string

			if engineName == "memory" {
				broker, presenceManager, engineMode, err = memoryEngine(node)
			} else if engineName == "redis" {
				broker, presenceManager, engineMode, err = redisEngine(node)
			} else if engineName == "tarantool" {
				broker, presenceManager, engineMode, err = tarantoolEngine(node)
			} else {
				log.Fatal().Msgf("unknown engine: %s", engineName)
			}
			if err != nil {
				log.Fatal().Msgf("error creating engine: %v", err)
			}

			tokenVerifier, err := jwtverify.NewTokenVerifierJWT(jwtVerifierConfig(), ruleContainer)
			if err != nil {
				log.Fatal().Msgf("error creating token verifier: %v", err)
			}

			var subTokenVerifier *jwtverify.VerifierJWT
			if viper.GetBool("separate_subscription_token_config") {
				log.Info().Msg("initializing separate verifier for subscription tokens")
				var err error
				subTokenVerifier, err = jwtverify.NewTokenVerifierJWT(subJWTVerifierConfig(), ruleContainer)
				if err != nil {
					log.Fatal().Msgf("error creating token verifier: %v", err)
				}
			}

			clientHandler := client.NewHandler(node, ruleContainer, tokenVerifier, subTokenVerifier, proxyMap, granularProxyMode)
			err = clientHandler.Setup()
			if err != nil {
				log.Fatal().Msgf("error setting up client handler: %v", err)
			}

			surveyCaller := survey.NewCaller(node)

			useAPIOpentelemetry := viper.GetBool("opentelemetry") && viper.GetBool("opentelemetry_api")

			httpAPIExecutor := api.NewExecutor(node, ruleContainer, surveyCaller, "http", useAPIOpentelemetry)
			grpcAPIExecutor := api.NewExecutor(node, ruleContainer, surveyCaller, "grpc", useAPIOpentelemetry)

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
				if useAPIOpentelemetry {
					grpcOpts = append(grpcOpts, grpc.StatsHandler(otelgrpc.NewServerHandler()))
				}
				grpcErrorMode, err := tools.OptionalStringChoice(viper.GetViper(), "grpc_api_error_mode", []string{transportErrorMode})
				if err != nil {
					log.Fatal().Msgf("error in config: %v", err)
				}
				grpcAPIServer = grpc.NewServer(grpcOpts...)
				_ = api.RegisterGRPCServerAPI(node, grpcAPIExecutor, grpcAPIServer, api.GRPCAPIServiceConfig{
					UseOpenTelemetry:      useAPIOpentelemetry,
					UseTransportErrorMode: grpcErrorMode == transportErrorMode,
				})
				if viper.GetBool("grpc_api_reflection") {
					reflection.Register(grpcAPIServer)
				}
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

			keepHeadersInContext := proxyEnabled
			servers, err := runHTTPServers(node, ruleContainer, httpAPIExecutor, keepHeadersInContext)
			if err != nil {
				log.Fatal().Msgf("error running HTTP server: %v", err)
			}

			var exporter *graphite.Exporter
			if viper.GetBool("graphite") {
				exporter = graphite.New(graphite.Config{
					Address:  net.JoinHostPort(viper.GetString("graphite_host"), strconv.Itoa(viper.GetInt("graphite_port"))),
					Gatherer: prometheus.DefaultGatherer,
					Prefix:   strings.TrimSuffix(viper.GetString("graphite_prefix"), ".") + "." + graphite.PreparePathComponent(nodeCfg.Name),
					Interval: GetDuration("graphite_interval"),
					Tags:     viper.GetBool("graphite_tags"),
				})
			}

			var statsSender *usage.Sender
			if !viper.GetBool("usage_stats_disable") {
				statsSender = usage.NewSender(node, ruleContainer, usage.Features{
					Edition:    edition,
					Version:    build.Version,
					Engine:     engineName,
					EngineMode: engineMode,
					Broker:     brokerName,
					BrokerMode: "",

					Websocket:     !viper.GetBool("websocket_disable"),
					HTTPStream:    viper.GetBool("http_stream"),
					SSE:           viper.GetBool("sse"),
					SockJS:        viper.GetBool("sockjs"),
					UniWebsocket:  viper.GetBool("uni_websocket"),
					UniHTTPStream: viper.GetBool("uni_http_stream"),
					UniSSE:        viper.GetBool("uni_sse"),
					UniGRPC:       viper.GetBool("uni_grpc"),

					GrpcAPI:             viper.GetBool("grpc_api"),
					SubscribeToPersonal: viper.GetBool("user_subscribe_to_personal"),
					Admin:               viper.GetBool("admin"),

					ConnectProxy:         proxyMap.ConnectProxy != nil,
					RefreshProxy:         proxyMap.RefreshProxy != nil,
					SubscribeProxy:       len(proxyMap.SubscribeProxies) > 0,
					PublishProxy:         len(proxyMap.PublishProxies) > 0,
					RPCProxy:             len(proxyMap.RpcProxies) > 0,
					SubRefreshProxy:      len(proxyMap.SubRefreshProxies) > 0,
					SubscribeStreamProxy: len(proxyMap.SubscribeStreamProxies) > 0,

					ClickhouseAnalytics: false,
					UserStatus:          false,
					Throttling:          false,
					Singleflight:        false,
				})
				go statsSender.Start(context.Background())
			}

			notify.RegisterHandlers(node, statsSender)

			tools.CheckPlainConfigKeys(defaults, viper.AllKeys())

			handleSignals(configFile, node, ruleContainer, tokenVerifier, subTokenVerifier, servers, grpcAPIServer, grpcUniServer, exporter)
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
	rootCmd.Flags().BoolP("admin_external", "", false, "expose admin web interface on external port")
	rootCmd.Flags().BoolP("prometheus", "", false, "enable Prometheus metrics endpoint")
	rootCmd.Flags().BoolP("swagger", "", false, "enable Swagger UI endpoint describing server HTTP API")
	rootCmd.Flags().BoolP("health", "", false, "enable health check endpoint")
	rootCmd.Flags().BoolP("sockjs", "", false, "enable SockJS endpoint")
	rootCmd.Flags().BoolP("uni_websocket", "", false, "enable unidirectional websocket endpoint")
	rootCmd.Flags().BoolP("uni_sse", "", false, "enable unidirectional SSE (EventSource) endpoint")
	rootCmd.Flags().BoolP("uni_http_stream", "", false, "enable unidirectional HTTP-streaming endpoint")
	rootCmd.Flags().BoolP("sse", "", false, "enable bidirectional SSE (EventSource) endpoint (with emulation layer)")
	rootCmd.Flags().BoolP("http_stream", "", false, "enable bidirectional HTTP-streaming endpoint (with emulation layer)")

	rootCmd.Flags().BoolP("client_insecure", "", false, "start in insecure client mode")
	rootCmd.Flags().BoolP("api_insecure", "", false, "use insecure API mode")
	rootCmd.Flags().BoolP("api_external", "", false, "expose API handler on external port")
	rootCmd.Flags().BoolP("admin_insecure", "", false, "use insecure admin mode – no auth required for admin socket")

	rootCmd.Flags().StringP("address", "a", "", "interface address to listen on")
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

	rootCmd.Flags().BoolP("uni_grpc", "", false, "enable unidirectional GRPC endpoint")
	rootCmd.Flags().IntP("uni_grpc_port", "", 11000, "port to bind unidirectional GRPC server to")

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
				fmt.Printf("%s\n", tools.ErrorMessageFromConfigError(err, checkConfigFile))
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
	var genTokenQuiet bool

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
			var user = fmt.Sprintf("user \"%s\"", genTokenUser)
			if genTokenUser == "" {
				user = "anonymous user"
			}
			exp := "without expiration"
			if genTokenTTL >= 0 {
				exp = fmt.Sprintf("with expiration TTL %s", time.Duration(genTokenTTL)*time.Second)
			}
			if genTokenQuiet {
				fmt.Print(token)
				return
			}
			fmt.Printf("HMAC SHA-256 JWT for %s %s:\n%s\n", user, exp, token)
		},
	}
	genTokenCmd.Flags().StringVarP(&genTokenConfigFile, "config", "c", "config.json", "path to config file")
	genTokenCmd.Flags().StringVarP(&genTokenUser, "user", "u", "", "user ID, by default anonymous")
	genTokenCmd.Flags().Int64VarP(&genTokenTTL, "ttl", "t", 3600*24*7, "token TTL in seconds, use -1 for token without expiration")
	genTokenCmd.Flags().BoolVarP(&genTokenQuiet, "quiet", "q", false, "only output the token without anything else")

	var genSubTokenConfigFile string
	var genSubTokenUser string
	var genSubTokenChannel string
	var genSubTokenTTL int64
	var genSubTokenQuiet bool

	var genSubTokenCmd = &cobra.Command{
		Use:   "gensubtoken",
		Short: "Generate sample subscription JWT for user",
		Long:  `Generate sample subscription JWT for user`,
		Run: func(cmd *cobra.Command, args []string) {
			bindCentrifugoConfig()
			err := readConfig(genSubTokenConfigFile)
			if err != nil && err != errConfigFileNotFound {
				fmt.Printf("error: %v\n", err)
				os.Exit(1)
			}
			if genSubTokenChannel == "" {
				fmt.Println("channel is required")
				os.Exit(1)
			}
			verifierConfig := jwtVerifierConfig()
			if viper.GetBool("separate_subscription_token_config") {
				verifierConfig = subJWTVerifierConfig()
			}
			token, err := cli.GenerateSubToken(verifierConfig, genSubTokenUser, genSubTokenChannel, genSubTokenTTL)
			if err != nil {
				fmt.Printf("error: %v\n", err)
				os.Exit(1)
			}
			var user = fmt.Sprintf("user \"%s\"", genSubTokenUser)
			if genSubTokenUser == "" {
				user = "anonymous user"
			}
			exp := "without expiration"
			if genTokenTTL >= 0 {
				exp = fmt.Sprintf("with expiration TTL %s", time.Duration(genTokenTTL)*time.Second)
			}
			if genSubTokenQuiet {
				fmt.Print(token)
				return
			}
			fmt.Printf("HMAC SHA-256 JWT for %s and channel \"%s\" %s:\n%s\n", user, genSubTokenChannel, exp, token)
		},
	}
	genSubTokenCmd.Flags().StringVarP(&genSubTokenConfigFile, "config", "c", "config.json", "path to config file")
	genSubTokenCmd.Flags().StringVarP(&genSubTokenUser, "user", "u", "", "user ID")
	genSubTokenCmd.Flags().StringVarP(&genSubTokenChannel, "channel", "s", "", "channel")
	genSubTokenCmd.Flags().Int64VarP(&genSubTokenTTL, "ttl", "t", 3600*24*7, "token TTL in seconds, use -1 for token without expiration")
	genSubTokenCmd.Flags().BoolVarP(&genSubTokenQuiet, "quiet", "q", false, "only output the token without anything else")

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
			verifierConfig := jwtVerifierConfig()
			if len(args) != 1 {
				fmt.Printf("error: provide token to check [centrifugo checktoken <TOKEN>]\n")
				os.Exit(1)
			}
			subject, claims, err := cli.CheckToken(verifierConfig, ruleConfig(), args[0])
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

	var checkSubTokenConfigFile string

	var checkSubTokenCmd = &cobra.Command{
		Use:   "checksubtoken [TOKEN]",
		Short: "Check subscription JWT",
		Long:  `Check subscription JWT`,
		Run: func(cmd *cobra.Command, args []string) {
			bindCentrifugoConfig()
			err := readConfig(checkSubTokenConfigFile)
			if err != nil && err != errConfigFileNotFound {
				fmt.Printf("error: %v\n", err)
				os.Exit(1)
			}
			verifierConfig := jwtVerifierConfig()
			if viper.GetBool("separate_subscription_token_config") {
				verifierConfig = subJWTVerifierConfig()
			}
			if len(args) != 1 {
				fmt.Printf("error: provide token to check [centrifugo checksubtoken <TOKEN>]\n")
				os.Exit(1)
			}
			subject, channel, claims, err := cli.CheckSubToken(verifierConfig, ruleConfig(), args[0])
			if err != nil {
				fmt.Printf("error: %v\n", err)
				os.Exit(1)
			}
			var user = fmt.Sprintf("user \"%s\"", subject)
			if subject == "" {
				user = "anonymous user"
			}
			fmt.Printf("valid subscription token for %s and channel \"%s\"\npayload: %s\n", user, channel, string(claims))
		},
	}
	checkSubTokenCmd.Flags().StringVarP(&checkSubTokenConfigFile, "config", "c", "config.json", "path to config file")

	var serveDir string
	var servePort int
	var serveAddr string

	var serveCmd = &cobra.Command{
		Use:   "serve",
		Short: "Run static file server (for development only)",
		Long:  `Run static file server (for development only)`,
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
	rootCmd.AddCommand(genSubTokenCmd)
	rootCmd.AddCommand(checkTokenCmd)
	rootCmd.AddCommand(checkSubTokenCmd)
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
	//goland:noinspection GoBoolExpressions – Goland is not smart enough here.
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

func handleSignals(configFile string, n *centrifuge.Node, ruleContainer *rule.Container, tokenVerifier *jwtverify.VerifierJWT, subTokenVerifier *jwtverify.VerifierJWT, httpServers []*http.Server, grpcAPIServer *grpc.Server, grpcUniServer *grpc.Server, exporter *graphite.Exporter) {
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
				log.Error().Msg(tools.ErrorMessageFromConfigError(err, configFile))
				continue
			}
			ruleConfig := ruleConfig()
			if err := tokenVerifier.Reload(jwtVerifierConfig()); err != nil {
				log.Error().Msgf("error reloading: %v", err)
				continue
			}
			if subTokenVerifier != nil {
				if err := subTokenVerifier.Reload(subJWTVerifierConfig()); err != nil {
					log.Error().Msgf("error reloading: %v", err)
					continue
				}
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
		return tools.MakeTLSConfig(viper.GetViper(), "", os.ReadFile)
	}

	return nil, nil
}

func tlsConfigForGRPC() (*tls.Config, error) {
	return tools.MakeTLSConfig(viper.GetViper(), "grpc_api_", os.ReadFile)
}

func tlsConfigForUniGRPC() (*tls.Config, error) {
	return tools.MakeTLSConfig(viper.GetViper(), "uni_grpc_", os.ReadFile)
}

type httpErrorLogWriter struct {
	zerolog.Logger
}

func (w *httpErrorLogWriter) Write(data []byte) (int, error) {
	w.Logger.Warn().Msg(strings.TrimSpace(string(data)))
	return len(data), nil
}

func runHTTPServers(n *centrifuge.Node, ruleContainer *rule.Container, apiExecutor *api.Executor, keepHeadersInContext bool) ([]*http.Server, error) {
	debug := viper.GetBool("debug")
	useAdmin := viper.GetBool("admin")
	usePrometheus := viper.GetBool("prometheus")
	useHealth := viper.GetBool("health")
	useSwagger := viper.GetBool("swagger")

	adminExternal := viper.GetBool("admin_external")
	apiExternal := viper.GetBool("api_external")

	apiDisabled := viper.GetBool("api_disable")

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
	if viper.GetBool("webtransport") {
		if !viper.GetBool("http3") {
			log.Fatal().Msg("can not enable webtransport without experimental HTTP/3")
		}
		portFlags |= HandlerWebtransport
	}
	if viper.GetBool("sockjs") {
		portFlags |= HandlerSockJS
	}
	if viper.GetBool("sse") {
		portFlags |= HandlerSSE
	}
	if viper.GetBool("http_stream") {
		portFlags |= HandlerHTTPStream
	}
	if viper.GetBool("sse") || viper.GetBool("http_stream") {
		portFlags |= HandlerEmulation
	}
	if useAdmin && adminExternal {
		portFlags |= HandlerAdmin
	}
	if !apiDisabled && apiExternal {
		portFlags |= HandlerAPI
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
	if !apiDisabled && !apiExternal {
		portFlags |= HandlerAPI
	}

	if useAdmin && !adminExternal {
		portFlags |= HandlerAdmin
	}
	if usePrometheus {
		portFlags |= HandlerPrometheus
	}
	if useSwagger {
		portFlags |= HandlerSwagger
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

	// Iterate over port-to-flags mapping and start HTTP servers
	// on separate ports serving handlers specified in flags.
	for addr, handlerFlags := range addrToHandlerFlags {
		addr := addr
		if handlerFlags == 0 {
			continue
		}
		var addrTLSConfig *tls.Config
		if !viper.GetBool("tls_external") || addr == externalAddr {
			addrTLSConfig = tlsConfig
		}

		useHTTP3 := viper.GetBool("http3") && addr == externalAddr

		var wtServer *webtransport.Server
		if useHTTP3 {
			wtServer = &webtransport.Server{
				CheckOrigin: getCheckOrigin(),
			}
		}

		mux := Mux(n, ruleContainer, apiExecutor, handlerFlags, keepHeadersInContext, wtServer)

		if useHTTP3 {
			wtServer.H3 = http3.Server{
				Addr:      addr,
				TLSConfig: addrTLSConfig,
				Handler:   mux,
			}
		}

		var protoSuffix string
		if useHTTP3 {
			protoSuffix = " with HTTP/3 (experimental)"
		}
		log.Info().Msgf("serving %s endpoints on %s%s", handlerFlags, addr, protoSuffix)

		server := &http.Server{
			Addr:      addr,
			Handler:   mux,
			TLSConfig: addrTLSConfig,
			ErrorLog:  stdlog.New(&httpErrorLogWriter{log.Logger}, "", 0),
		}

		if useHTTP3 {
			server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = wtServer.H3.SetQuicHeaders(w.Header())
				mux.ServeHTTP(w, r)
			})
		}

		servers = append(servers, server)

		go func() {
			if useHTTP3 {
				if addrTLSConfig == nil {
					log.Fatal().Msgf("HTTP/3 requires TLS configured")
				}
				if viper.GetBool("tls_autocert") {
					log.Fatal().Msgf("can not use HTTP/3 with autocert")
				}

				udpAddr, err := net.ResolveUDPAddr("udp", addr)
				if err != nil {
					log.Fatal().Msgf("can not start HTTP/3, resolve UDP: %v", err)
				}
				udpConn, err := net.ListenUDP("udp", udpAddr)
				if err != nil {
					log.Fatal().Msgf("can not start HTTP/3, listen UDP: %v", err)
				}
				defer func() { _ = udpConn.Close() }()

				tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
				if err != nil {
					log.Fatal().Msgf("can not start HTTP/3, resolve TCP: %v", err)
				}
				tcpConn, err := net.ListenTCP("tcp", tcpAddr)
				if err != nil {
					log.Fatal().Msgf("can not start HTTP/3, listen TCP: %v", err)
				}
				defer func() { _ = tcpConn.Close() }()

				tlsConn := tls.NewListener(tcpConn, addrTLSConfig)
				defer func() { _ = tlsConn.Close() }()

				hErr := make(chan error)
				qErr := make(chan error)
				go func() {
					hErr <- server.Serve(tlsConn)
				}()
				go func() {
					qErr <- wtServer.Serve(udpConn)
				}()

				select {
				case err := <-hErr:
					_ = wtServer.Close()
					if err != http.ErrServerClosed {
						log.Fatal().Msgf("ListenAndServe: %v", err)
					}
				case err := <-qErr:
					// Cannot close the HTTP server or wait for requests to complete properly.
					log.Fatal().Msgf("ListenAndServe HTTP/3: %v", err)
				}
			} else {
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

	cfg.Presence = v.GetBool("presence")
	cfg.JoinLeave = v.GetBool("join_leave")
	cfg.ForcePushJoinLeave = v.GetBool("force_push_join_leave")
	cfg.HistorySize = v.GetInt("history_size")
	cfg.HistoryTTL = tools.Duration(GetDuration("history_ttl", true))
	cfg.HistoryMetaTTL = tools.Duration(GetDuration("history_meta_ttl", true))
	cfg.ForcePositioning = v.GetBool("force_positioning")
	cfg.AllowPositioning = v.GetBool("allow_positioning")
	cfg.AllowRecovery = v.GetBool("allow_recovery")
	cfg.ForceRecovery = v.GetBool("force_recovery")
	cfg.SubscribeForAnonymous = v.GetBool("allow_subscribe_for_anonymous")
	cfg.SubscribeForClient = v.GetBool("allow_subscribe_for_client")
	cfg.PublishForAnonymous = v.GetBool("allow_publish_for_anonymous")
	cfg.PublishForClient = v.GetBool("allow_publish_for_client")
	cfg.PublishForSubscriber = v.GetBool("allow_publish_for_subscriber")
	cfg.PresenceForAnonymous = v.GetBool("allow_presence_for_anonymous")
	cfg.PresenceForClient = v.GetBool("allow_presence_for_client")
	cfg.PresenceForSubscriber = v.GetBool("allow_presence_for_subscriber")
	cfg.HistoryForAnonymous = v.GetBool("allow_history_for_anonymous")
	cfg.HistoryForClient = v.GetBool("allow_history_for_client")
	cfg.HistoryForSubscriber = v.GetBool("allow_history_for_subscriber")
	cfg.UserLimitedChannels = v.GetBool("allow_user_limited_channels")
	cfg.ChannelRegex = v.GetString("channel_regex")
	cfg.ProxySubscribe = v.GetBool("proxy_subscribe")
	cfg.ProxyPublish = v.GetBool("proxy_publish")
	cfg.ProxySubRefresh = v.GetBool("proxy_sub_refresh")
	cfg.SubscribeProxyName = v.GetString("subscribe_proxy_name")
	cfg.PublishProxyName = v.GetString("publish_proxy_name")
	cfg.SubRefreshProxyName = v.GetString("sub_refresh_proxy_name")
	cfg.ProxySubscribeStream = v.GetBool("proxy_stream_subscribe")
	cfg.ProxySubscribeStreamBidirectional = v.GetBool("proxy_subscribe_stream_bidirectional")

	cfg.Namespaces = namespacesFromConfig(v)

	cfg.ChannelPrivatePrefix = v.GetString("channel_private_prefix")
	cfg.ChannelNamespaceBoundary = v.GetString("channel_namespace_boundary")
	cfg.ChannelUserBoundary = v.GetString("channel_user_boundary")
	cfg.ChannelUserSeparator = v.GetString("channel_user_separator")
	cfg.UserSubscribeToPersonal = v.GetBool("user_subscribe_to_personal")
	cfg.UserPersonalSingleConnection = v.GetBool("user_personal_single_connection")
	cfg.UserPersonalChannelNamespace = v.GetString("user_personal_channel_namespace")
	cfg.ClientInsecure = v.GetBool("client_insecure")
	cfg.ClientInsecureSkipTokenSignatureVerify = v.GetBool("client_insecure_skip_token_signature_verify")
	cfg.AnonymousConnectWithoutToken = v.GetBool("allow_anonymous_connect_without_token")
	cfg.DisallowAnonymousConnectionTokens = v.GetBool("disallow_anonymous_connection_tokens")
	cfg.ClientConcurrency = v.GetInt("client_concurrency")
	cfg.RpcNamespaceBoundary = v.GetString("rpc_namespace_boundary")
	cfg.RpcProxyName = v.GetString("rpc_proxy_name")
	cfg.RpcNamespaces = rpcNamespacesFromConfig(v)
	cfg.ClientConnectionLimit = v.GetInt("client_connection_limit")
	cfg.ClientConnectionRateLimit = v.GetInt("client_connection_rate_limit")

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
	cfg.Audience = v.GetString("token_audience")
	cfg.AudienceRegex = v.GetString("token_audience_regex")
	cfg.Issuer = v.GetString("token_issuer")
	cfg.IssuerRegex = v.GetString("token_issuer_regex")
	return cfg
}

func subJWTVerifierConfig() jwtverify.VerifierConfig {
	v := viper.GetViper()
	cfg := jwtverify.VerifierConfig{}

	cfg.HMACSecretKey = v.GetString("subscription_token_hmac_secret_key")

	rsaPublicKey := v.GetString("subscription_token_rsa_public_key")
	if rsaPublicKey != "" {
		pubKey, err := jwtutils.ParseRSAPublicKeyFromPEM([]byte(rsaPublicKey))
		if err != nil {
			log.Fatal().Msgf("error parsing RSA public key: %v", err)
		}
		cfg.RSAPublicKey = pubKey
	}

	ecdsaPublicKey := v.GetString("subscription_token_ecdsa_public_key")
	if ecdsaPublicKey != "" {
		pubKey, err := jwtutils.ParseECDSAPublicKeyFromPEM([]byte(ecdsaPublicKey))
		if err != nil {
			log.Fatal().Msgf("error parsing ECDSA public key: %v", err)
		}
		cfg.ECDSAPublicKey = pubKey
	}

	cfg.JWKSPublicEndpoint = v.GetString("subscription_token_jwks_public_endpoint")
	cfg.Audience = v.GetString("subscription_token_audience")
	cfg.AudienceRegex = v.GetString("subscription_token_audience_regex")
	cfg.Issuer = v.GetString("subscription_token_issuer")
	cfg.IssuerRegex = v.GetString("subscription_token_issuer_regex")
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

func proxyMapConfig() (*client.ProxyMap, bool) {
	v := viper.GetViper()

	proxyMap := &client.ProxyMap{
		SubscribeProxies:       map[string]proxy.SubscribeProxy{},
		PublishProxies:         map[string]proxy.PublishProxy{},
		RpcProxies:             map[string]proxy.RPCProxy{},
		SubRefreshProxies:      map[string]proxy.SubRefreshProxy{},
		SubscribeStreamProxies: map[string]*proxy.SubscribeStreamProxy{},
	}

	proxyConfig := proxy.Config{
		BinaryEncoding:        v.GetBool("proxy_binary_encoding"),
		IncludeConnectionMeta: v.GetBool("proxy_include_connection_meta"),
		GrpcCertFile:          v.GetString("proxy_grpc_cert_file"),
		GrpcCredentialsKey:    v.GetString("proxy_grpc_credentials_key"),
		GrpcCredentialsValue:  v.GetString("proxy_grpc_credentials_value"),
		GrpcMetadata:          v.GetStringSlice("proxy_grpc_metadata"),
		GrpcCompression:       v.GetBool("proxy_grpc_compression"),
	}

	proxyConfig.HttpHeaders = v.GetStringSlice("proxy_http_headers")
	for i, header := range proxyConfig.HttpHeaders {
		proxyConfig.HttpHeaders[i] = strings.ToLower(header)
	}

	staticHttpHeaders, err := tools.MapStringString(v, "proxy_static_http_headers")
	if err != nil {
		log.Fatal().Err(err).Msg("malformed configuration for proxy_static_http_headers")
	}
	proxyConfig.StaticHttpHeaders = staticHttpHeaders

	connectEndpoint := v.GetString("proxy_connect_endpoint")
	connectTimeout := GetDuration("proxy_connect_timeout")
	refreshEndpoint := v.GetString("proxy_refresh_endpoint")
	refreshTimeout := GetDuration("proxy_refresh_timeout")
	rpcEndpoint := v.GetString("proxy_rpc_endpoint")
	rpcTimeout := GetDuration("proxy_rpc_timeout")
	subscribeEndpoint := v.GetString("proxy_subscribe_endpoint")
	subscribeTimeout := GetDuration("proxy_subscribe_timeout")
	publishEndpoint := v.GetString("proxy_publish_endpoint")
	publishTimeout := GetDuration("proxy_publish_timeout")
	subRefreshEndpoint := v.GetString("proxy_sub_refresh_endpoint")
	subRefreshTimeout := GetDuration("proxy_sub_refresh_timeout")
	proxyStreamSubscribeEndpoint := v.GetString("proxy_subscribe_stream_endpoint")
	if strings.HasPrefix(proxyStreamSubscribeEndpoint, "http") {
		log.Fatal().Msg("error creating subscribe stream proxy: only GRPC endpoints supported")
	}
	proxyStreamSubscribeTimeout := GetDuration("proxy_subscribe_stream_timeout")

	if connectEndpoint != "" {
		proxyConfig.Endpoint = connectEndpoint
		proxyConfig.Timeout = tools.Duration(connectTimeout)
		var err error
		proxyMap.ConnectProxy, err = proxy.GetConnectProxy(proxyConfig)
		if err != nil {
			log.Fatal().Msgf("error creating connect proxy: %v", err)
		}
		log.Info().Str("endpoint", connectEndpoint).Msg("connect proxy enabled")
	}

	if refreshEndpoint != "" {
		proxyConfig.Endpoint = refreshEndpoint
		proxyConfig.Timeout = tools.Duration(refreshTimeout)
		var err error
		proxyMap.RefreshProxy, err = proxy.GetRefreshProxy(proxyConfig)
		if err != nil {
			log.Fatal().Msgf("error creating refresh proxy: %v", err)
		}
		log.Info().Str("endpoint", refreshEndpoint).Msg("refresh proxy enabled")
	}

	if subscribeEndpoint != "" {
		proxyConfig.Endpoint = subscribeEndpoint
		proxyConfig.Timeout = tools.Duration(subscribeTimeout)
		sp, err := proxy.GetSubscribeProxy(proxyConfig)
		if err != nil {
			log.Fatal().Msgf("error creating subscribe proxy: %v", err)
		}
		proxyMap.SubscribeProxies[""] = sp
		log.Info().Str("endpoint", subscribeEndpoint).Msg("subscribe proxy enabled")
	}

	if publishEndpoint != "" {
		proxyConfig.Endpoint = publishEndpoint
		proxyConfig.Timeout = tools.Duration(publishTimeout)
		pp, err := proxy.GetPublishProxy(proxyConfig)
		if err != nil {
			log.Fatal().Msgf("error creating publish proxy: %v", err)
		}
		proxyMap.PublishProxies[""] = pp
		log.Info().Str("endpoint", publishEndpoint).Msg("publish proxy enabled")
	}

	if rpcEndpoint != "" {
		proxyConfig.Endpoint = rpcEndpoint
		proxyConfig.Timeout = tools.Duration(rpcTimeout)
		rp, err := proxy.GetRpcProxy(proxyConfig)
		if err != nil {
			log.Fatal().Msgf("error creating rpc proxy: %v", err)
		}
		proxyMap.RpcProxies[""] = rp
		log.Info().Str("endpoint", rpcEndpoint).Msg("RPC proxy enabled")
	}

	if subRefreshEndpoint != "" {
		proxyConfig.Endpoint = subRefreshEndpoint
		proxyConfig.Timeout = tools.Duration(subRefreshTimeout)
		srp, err := proxy.GetSubRefreshProxy(proxyConfig)
		if err != nil {
			log.Fatal().Msgf("error creating sub refresh proxy: %v", err)
		}
		proxyMap.SubRefreshProxies[""] = srp
		log.Info().Str("endpoint", subRefreshEndpoint).Msg("sub refresh proxy enabled")
	}

	if proxyStreamSubscribeEndpoint != "" {
		proxyConfig.Endpoint = proxyStreamSubscribeEndpoint
		proxyConfig.Timeout = tools.Duration(proxyStreamSubscribeTimeout)
		streamProxy, err := proxy.NewSubscribeStreamProxy(proxyConfig)
		if err != nil {
			log.Fatal().Msgf("error creating subscribe stream proxy: %v", err)
		}
		proxyMap.SubscribeStreamProxies[""] = streamProxy
		log.Info().Str("endpoint", proxyStreamSubscribeEndpoint).Msg("subscribe stream proxy enabled")
	}

	proxyEnabled := connectEndpoint != "" || refreshEndpoint != "" ||
		rpcEndpoint != "" || subscribeEndpoint != "" || publishEndpoint != "" ||
		subRefreshEndpoint != "" || proxyStreamSubscribeEndpoint != ""

	return proxyMap, proxyEnabled
}

func granularProxyMapConfig(ruleConfig rule.Config) (*client.ProxyMap, bool) {
	proxyMap := &client.ProxyMap{
		RpcProxies:             map[string]proxy.RPCProxy{},
		PublishProxies:         map[string]proxy.PublishProxy{},
		SubscribeProxies:       map[string]proxy.SubscribeProxy{},
		SubRefreshProxies:      map[string]proxy.SubRefreshProxy{},
		SubscribeStreamProxies: map[string]*proxy.SubscribeStreamProxy{},
	}
	proxyList := granularProxiesFromConfig(viper.GetViper())
	proxies := make(map[string]proxy.Config)
	for _, p := range proxyList {
		for i, header := range p.HttpHeaders {
			p.HttpHeaders[i] = strings.ToLower(header)
		}
		proxies[p.Name] = p
	}

	var proxyEnabled bool

	connectProxyName := viper.GetString("connect_proxy_name")
	if connectProxyName != "" {
		p, ok := proxies[connectProxyName]
		if !ok {
			log.Fatal().Msgf("connect proxy not found: %s", connectProxyName)
		}
		var err error
		proxyMap.ConnectProxy, err = proxy.GetConnectProxy(p)
		if err != nil {
			log.Fatal().Msgf("error creating connect proxy: %v", err)
		}
		proxyEnabled = true
	}
	refreshProxyName := viper.GetString("refresh_proxy_name")
	if refreshProxyName != "" {
		p, ok := proxies[refreshProxyName]
		if !ok {
			log.Fatal().Msgf("refresh proxy not found: %s", refreshProxyName)
		}
		var err error
		proxyMap.RefreshProxy, err = proxy.GetRefreshProxy(p)
		if err != nil {
			log.Fatal().Msgf("error creating refresh proxy: %v", err)
		}
		proxyEnabled = true
	}
	subscribeProxyName := ruleConfig.SubscribeProxyName
	if subscribeProxyName != "" {
		p, ok := proxies[subscribeProxyName]
		if !ok {
			log.Fatal().Msgf("subscribe proxy not found: %s", subscribeProxyName)
		}
		sp, err := proxy.GetSubscribeProxy(p)
		if err != nil {
			log.Fatal().Msgf("error creating subscribe proxy: %v", err)
		}
		proxyMap.SubscribeProxies[subscribeProxyName] = sp
		proxyEnabled = true
	}

	publishProxyName := ruleConfig.PublishProxyName
	if publishProxyName != "" {
		p, ok := proxies[publishProxyName]
		if !ok {
			log.Fatal().Msgf("publish proxy not found: %s", publishProxyName)
		}
		pp, err := proxy.GetPublishProxy(p)
		if err != nil {
			log.Fatal().Msgf("error creating publish proxy: %v", err)
		}
		proxyMap.PublishProxies[publishProxyName] = pp
		proxyEnabled = true
	}

	subRefreshProxyName := ruleConfig.SubRefreshProxyName
	if subRefreshProxyName != "" {
		p, ok := proxies[subRefreshProxyName]
		if !ok {
			log.Fatal().Msgf("sub refresh proxy not found: %s", subRefreshProxyName)
		}
		srp, err := proxy.GetSubRefreshProxy(p)
		if err != nil {
			log.Fatal().Msgf("error creating publish proxy: %v", err)
		}
		proxyMap.SubRefreshProxies[subRefreshProxyName] = srp
		proxyEnabled = true
	}

	subscribeStreamProxyName := ruleConfig.SubscribeStreamProxyName
	if subscribeStreamProxyName != "" {
		p, ok := proxies[subscribeStreamProxyName]
		if !ok {
			log.Fatal().Msgf("subscribe stream proxy not found: %s", subscribeStreamProxyName)
		}
		if strings.HasPrefix(p.Endpoint, "http") {
			log.Fatal().Msgf("error creating subscribe stream proxy %s only GRPC endpoints supported", subscribeStreamProxyName)
		}
		sp, err := proxy.NewSubscribeStreamProxy(p)
		if err != nil {
			log.Fatal().Msgf("error creating subscribe proxy: %v", err)
		}
		proxyMap.SubscribeStreamProxies[subscribeProxyName] = sp
		proxyEnabled = true
	}

	for _, ns := range ruleConfig.Namespaces {
		subscribeProxyName := ns.SubscribeProxyName
		publishProxyName := ns.PublishProxyName
		subRefreshProxyName := ns.SubRefreshProxyName
		subscribeStreamProxyName := ns.SubscribeStreamProxyName

		if subscribeProxyName != "" {
			p, ok := proxies[subscribeProxyName]
			if !ok {
				log.Fatal().Msgf("subscribe proxy not found: %s", subscribeProxyName)
			}
			sp, err := proxy.GetSubscribeProxy(p)
			if err != nil {
				log.Fatal().Msgf("error creating subscribe proxy: %v", err)
			}
			proxyMap.SubscribeProxies[subscribeProxyName] = sp
			proxyEnabled = true
		}

		if publishProxyName != "" {
			p, ok := proxies[publishProxyName]
			if !ok {
				log.Fatal().Msgf("publish proxy not found: %s", publishProxyName)
			}
			pp, err := proxy.GetPublishProxy(p)
			if err != nil {
				log.Fatal().Msgf("error creating publish proxy: %v", err)
			}
			proxyMap.PublishProxies[publishProxyName] = pp
			proxyEnabled = true
		}

		if subRefreshProxyName != "" {
			p, ok := proxies[subRefreshProxyName]
			if !ok {
				log.Fatal().Msgf("sub refresh proxy not found: %s", subRefreshProxyName)
			}
			srp, err := proxy.GetSubRefreshProxy(p)
			if err != nil {
				log.Fatal().Msgf("error creating sub refresh proxy: %v", err)
			}
			proxyMap.SubRefreshProxies[subRefreshProxyName] = srp
			proxyEnabled = true
		}

		if subscribeStreamProxyName != "" {
			p, ok := proxies[subscribeStreamProxyName]
			if !ok {
				log.Fatal().Msgf("subscribe stream proxy not found: %s", subscribeStreamProxyName)
			}
			if strings.HasPrefix(p.Endpoint, "http") {
				log.Fatal().Msgf("error creating subscribe stream proxy %s only GRPC endpoints supported", subscribeStreamProxyName)
			}
			ssp, err := proxy.NewSubscribeStreamProxy(p)
			if err != nil {
				log.Fatal().Msgf("error creating subscribe stream proxy: %v", err)
			}
			proxyMap.SubscribeStreamProxies[subscribeStreamProxyName] = ssp
			proxyEnabled = true
		}
	}

	rpcProxyName := ruleConfig.RpcProxyName
	if rpcProxyName != "" {
		p, ok := proxies[rpcProxyName]
		if !ok {
			log.Fatal().Msgf("rpc proxy not found: %s", rpcProxyName)
		}
		rp, err := proxy.GetRpcProxy(p)
		if err != nil {
			log.Fatal().Msgf("error creating rpc proxy: %v", err)
		}
		proxyMap.RpcProxies[rpcProxyName] = rp
		proxyEnabled = true
	}

	for _, ns := range ruleConfig.RpcNamespaces {
		rpcProxyName := ns.RpcProxyName
		if rpcProxyName != "" {
			p, ok := proxies[rpcProxyName]
			if !ok {
				log.Fatal().Msgf("rpc proxy not found: %s", rpcProxyName)
			}
			rp, err := proxy.GetRpcProxy(p)
			if err != nil {
				log.Fatal().Msgf("error creating rpc proxy: %v", err)
			}
			proxyMap.RpcProxies[rpcProxyName] = rp
			proxyEnabled = true
		}
	}

	return proxyMap, proxyEnabled
}

var proxyNamePattern = "^[-a-zA-Z0-9_.]{2,}$"
var proxyNameRe = regexp.MustCompile(proxyNamePattern)

func granularProxiesFromConfig(v *viper.Viper) []proxy.Config {
	var proxies []proxy.Config
	if !v.IsSet("proxies") {
		return proxies
	}
	var jsonData []byte
	var err error
	switch val := v.Get("proxies").(type) {
	case string:
		jsonData = []byte(val)
		err = json.Unmarshal([]byte(val), &proxies)
	case []any:
		jsonData, _ = json.Marshal(val)
		decoderCfg := tools.DecoderConfig(&proxies)
		decoder, newErr := mapstructure.NewDecoder(decoderCfg)
		if newErr != nil {
			log.Fatal().Msg(newErr.Error())
			return proxies
		}
		err = decoder.Decode(v.Get("proxies"))
	default:
		err = fmt.Errorf("unknown proxies type: %T", val)
	}
	if err != nil {
		log.Fatal().Err(err).Msg("malformed proxies")
	}
	names := map[string]struct{}{}
	for _, p := range proxies {
		if !proxyNameRe.Match([]byte(p.Name)) {
			log.Fatal().Msgf("invalid proxy name: %s, must match %s regular expression", p.Name, proxyNamePattern)
		}
		if _, ok := names[p.Name]; ok {
			log.Fatal().Msgf("duplicate proxy name: %s", p.Name)
		}
		if p.Timeout == 0 {
			p.Timeout = tools.Duration(time.Second)
		}
		if p.Endpoint == "" {
			log.Fatal().Msgf("no endpoint set for proxy %s", p.Name)
		}
		names[p.Name] = struct{}{}
	}

	proxy.WarnUnknownProxyKeys(jsonData)

	return proxies
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
	cfg.HistoryMetaTTL = GetDuration("global_history_meta_ttl", true)

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
	var jsonData []byte
	var err error
	switch val := v.Get("namespaces").(type) {
	case string:
		jsonData = []byte(val)
		err = json.Unmarshal([]byte(val), &ns)
	case []any:
		jsonData, _ = json.Marshal(val)
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
	rule.WarnUnknownNamespaceKeys(jsonData)
	return ns
}

// rpcNamespacesFromConfig allows to unmarshal rpc namespaces.
func rpcNamespacesFromConfig(v *viper.Viper) []rule.RpcNamespace {
	var ns []rule.RpcNamespace
	if !v.IsSet("rpc_namespaces") {
		return ns
	}
	var jsonData []byte
	var err error
	switch val := v.Get("rpc_namespaces").(type) {
	case string:
		jsonData, _ = json.Marshal(val)
		err = json.Unmarshal([]byte(val), &ns)
	case []any:
		jsonData, _ = json.Marshal(val)
		decoderCfg := tools.DecoderConfig(&ns)
		decoder, newErr := mapstructure.NewDecoder(decoderCfg)
		if newErr != nil {
			log.Fatal().Msg(newErr.Error())
			return ns
		}
		err = decoder.Decode(v.Get("rpc_namespaces"))
	default:
		err = fmt.Errorf("unknown rpc_namespaces type: %T", val)
	}
	if err != nil {
		log.Error().Err(err).Msg("malformed rpc_namespaces")
		os.Exit(1)
	}
	rule.WarnUnknownRpcNamespaceKeys(jsonData)
	return ns
}

func getPingPongConfig() centrifuge.PingPongConfig {
	pingInterval := GetDuration("ping_interval")
	pongTimeout := GetDuration("pong_timeout")
	if pingInterval <= pongTimeout {
		log.Fatal().Msgf("ping_interval (%s) must be greater than pong_timeout (%s)", pingInterval, pongTimeout)
	}
	return centrifuge.PingPongConfig{
		PingInterval: pingInterval,
		PongTimeout:  pongTimeout,
	}
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
	cfg.WriteTimeout = GetDuration("websocket_write_timeout")
	cfg.MessageSizeLimit = v.GetInt("websocket_message_size_limit")
	cfg.CheckOrigin = getCheckOrigin()
	cfg.PingPongConfig = getPingPongConfig()
	return cfg
}

func httpStreamHandlerConfig() centrifuge.HTTPStreamConfig {
	return centrifuge.HTTPStreamConfig{
		MaxRequestBodySize: viper.GetInt("http_stream_max_request_body_size"),
		PingPongConfig:     getPingPongConfig(),
	}
}

func sseHandlerConfig() centrifuge.SSEConfig {
	return centrifuge.SSEConfig{
		MaxRequestBodySize: viper.GetInt("sse_max_request_body_size"),
		PingPongConfig:     getPingPongConfig(),
	}
}

func emulationHandlerConfig() centrifuge.EmulationConfig {
	return centrifuge.EmulationConfig{
		MaxRequestBodySize: viper.GetInt("emulation_max_request_body_size"),
	}
}

var warnAllowedOriginsOnce sync.Once

func getCheckOrigin() func(r *http.Request) bool {
	v := viper.GetViper()
	allowedOrigins := v.GetStringSlice("allowed_origins")
	if len(allowedOrigins) == 0 {
		return func(r *http.Request) bool {
			// Only allow connections without Origin in this case.
			originHeader := r.Header.Get("Origin")
			if originHeader == "" {
				return true
			}
			log.Info().Str("origin", originHeader).Msg("request Origin is not authorized due to empty allowed_origins")
			return false
		}
	}
	originChecker, err := origin.NewPatternChecker(allowedOrigins)
	if err != nil {
		log.Fatal().Msgf("error creating origin checker: %v", err)
	}
	if len(allowedOrigins) == 1 && allowedOrigins[0] == "*" {
		// Fast path for *.
		warnAllowedOriginsOnce.Do(func() {
			log.Warn().Msg("usage of allowed_origins * is discouraged for security reasons, consider setting exact list of origins")
		})
		return func(r *http.Request) bool {
			return true
		}
	}
	return func(r *http.Request) bool {
		ok := originChecker.Check(r)
		if !ok {
			log.Info().Str("origin", r.Header.Get("Origin")).Strs("allowed_origins", allowedOrigins).Msg("request Origin is not authorized")
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
		WriteTimeout:       GetDuration("uni_websocket_write_timeout"),
		MessageSizeLimit:   v.GetInt("uni_websocket_message_size_limit"),
		CheckOrigin:        getCheckOrigin(),
		PingPongConfig:     getPingPongConfig(),
	}
}

func uniSSEHandlerConfig() unisse.Config {
	return unisse.Config{
		MaxRequestBodySize: viper.GetInt("uni_sse_max_request_body_size"),
		PingPongConfig:     getPingPongConfig(),
	}
}

func uniStreamHandlerConfig() unihttpstream.Config {
	return unihttpstream.Config{
		MaxRequestBodySize: viper.GetInt("uni_http_stream_max_request_body_size"),
		PingPongConfig:     getPingPongConfig(),
	}
}

func uniGRPCHandlerConfig() unigrpc.Config {
	return unigrpc.Config{}
}

func sockjsHandlerConfig() centrifuge.SockjsConfig {
	v := viper.GetViper()
	cfg := centrifuge.SockjsConfig{}
	cfg.URL = v.GetString("sockjs_url")
	cfg.WebsocketReadBufferSize = v.GetInt("websocket_read_buffer_size")
	cfg.WebsocketWriteBufferSize = v.GetInt("websocket_write_buffer_size")
	cfg.WebsocketUseWriteBufferPool = v.GetBool("websocket_use_write_buffer_pool")
	cfg.WebsocketWriteTimeout = GetDuration("websocket_write_timeout")
	cfg.CheckOrigin = getCheckOrigin()
	cfg.WebsocketCheckOrigin = getCheckOrigin()
	cfg.PingPongConfig = getPingPongConfig()
	return cfg
}

func webTransportHandlerConfig() wt.Config {
	return wt.Config{
		PingPongConfig: getPingPongConfig(),
	}
}

func adminHandlerConfig() admin.Config {
	v := viper.GetViper()
	cfg := admin.Config{}
	cfg.WebFS = webui.FS
	cfg.WebPath = v.GetString("admin_web_path")
	cfg.WebProxyAddress = v.GetString("admin_web_proxy_address")
	cfg.Password = v.GetString("admin_password")
	cfg.Secret = v.GetString("admin_secret")
	cfg.Insecure = v.GetBool("admin_insecure")
	cfg.Prefix = v.GetString("admin_handler_prefix")
	return cfg
}

func memoryEngine(n *centrifuge.Node) (centrifuge.Broker, centrifuge.PresenceManager, string, error) {
	brokerConf, err := memoryBrokerConfig()
	if err != nil {
		return nil, nil, "", err
	}
	broker, err := centrifuge.NewMemoryBroker(n, *brokerConf)
	if err != nil {
		return nil, nil, "", err
	}
	presenceManagerConf, err := memoryPresenceManagerConfig()
	if err != nil {
		return nil, nil, "", err
	}
	presenceManager, err := centrifuge.NewMemoryPresenceManager(n, *presenceManagerConf)
	if err != nil {
		return nil, nil, "", err
	}
	return broker, presenceManager, "", nil
}

func memoryBrokerConfig() (*centrifuge.MemoryBrokerConfig, error) {
	return &centrifuge.MemoryBrokerConfig{}, nil
}

func memoryPresenceManagerConfig() (*centrifuge.MemoryPresenceManagerConfig, error) {
	return &centrifuge.MemoryPresenceManagerConfig{}, nil
}

func addRedisShardCommonSettings(shardConf *centrifuge.RedisShardConfig) {
	shardConf.DB = viper.GetInt("redis_db")
	shardConf.User = viper.GetString("redis_user")
	shardConf.Password = viper.GetString("redis_password")
	shardConf.ClientName = viper.GetString("redis_client_name")

	if viper.GetBool("redis_tls") {
		tlsConfig, err := tools.MakeTLSConfig(viper.GetViper(), "redis_", os.ReadFile)
		if err != nil {
			log.Fatal().Msgf("error creating Redis TLS config: %v", err)
		}
		shardConf.TLSConfig = tlsConfig
	}
	shardConf.ConnectTimeout = GetDuration("redis_connect_timeout")
	shardConf.IOTimeout = GetDuration("redis_io_timeout")
	shardConf.ForceRESP2 = viper.GetBool("redis_force_resp2")
}

func getRedisShardConfigs() ([]centrifuge.RedisShardConfig, string, error) {
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
					return nil, "", fmt.Errorf("malformed Redis Cluster address: %s", address)
				}
			}
			conf := &centrifuge.RedisShardConfig{
				ClusterAddresses: clusterAddresses,
			}
			addRedisShardCommonSettings(conf)
			shardConfigs = append(shardConfigs, *conf)
		}
		return shardConfigs, "cluster", nil
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
					return nil, "", fmt.Errorf("malformed Redis Sentinel address: %s", address)
				}
			}
			conf := &centrifuge.RedisShardConfig{
				SentinelAddresses: sentinelAddresses,
			}
			addRedisShardCommonSettings(conf)
			conf.SentinelUser = viper.GetString("redis_sentinel_user")
			conf.SentinelPassword = viper.GetString("redis_sentinel_password")
			conf.SentinelMasterName = viper.GetString("redis_sentinel_master_name")
			if conf.SentinelMasterName == "" {
				return nil, "", fmt.Errorf("master name must be set when using Redis Sentinel")
			}
			conf.SentinelClientName = viper.GetString("redis_sentinel_client_name")
			if viper.GetBool("redis_sentinel_tls") {
				tlsConfig, err := tools.MakeTLSConfig(viper.GetViper(), "redis_sentinel_", os.ReadFile)
				if err != nil {
					log.Fatal().Msgf("error creating Redis Sentinel TLS config: %v", err)
				}
				conf.SentinelTLSConfig = tlsConfig
			}
			shardConfigs = append(shardConfigs, *conf)
		}
		return shardConfigs, "sentinel", nil
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

	return shardConfigs, "standalone", nil
}

func getRedisShards(n *centrifuge.Node) ([]*centrifuge.RedisShard, string, error) {
	redisShardConfigs, mode, err := getRedisShardConfigs()
	if err != nil {
		return nil, "", err
	}
	redisShards := make([]*centrifuge.RedisShard, 0, len(redisShardConfigs))

	for _, redisConf := range redisShardConfigs {
		redisShard, err := centrifuge.NewRedisShard(n, redisConf)
		if err != nil {
			return nil, "", err
		}
		redisShards = append(redisShards, redisShard)
	}

	if len(redisShards) > 1 {
		mode += "_sharded"
	}

	return redisShards, mode, nil
}

func redisEngine(n *centrifuge.Node) (centrifuge.Broker, centrifuge.PresenceManager, string, error) {
	redisShards, mode, err := getRedisShards(n)
	if err != nil {
		return nil, nil, "", err
	}

	broker, err := centrifuge.NewRedisBroker(n, centrifuge.RedisBrokerConfig{
		Shards:   redisShards,
		Prefix:   viper.GetString("redis_prefix"),
		UseLists: viper.GetBool("redis_use_lists"),
	})
	if err != nil {
		return nil, nil, "", err
	}

	presenceManager, err := centrifuge.NewRedisPresenceManager(n, centrifuge.RedisPresenceManagerConfig{
		Shards:      redisShards,
		Prefix:      viper.GetString("redis_prefix"),
		PresenceTTL: GetDuration("global_presence_ttl", true),
	})
	if err != nil {
		return nil, nil, "", err
	}

	return broker, presenceManager, mode, nil
}

func getTarantoolShardConfigs() ([]tntengine.ShardConfig, string, error) {
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
			return nil, "", fmt.Errorf("unknown Tarantool mode: %s", viper.GetString("tarantool_mode"))
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
	return shardConfigs, string(mode), nil
}

func getTarantoolShards() ([]*tntengine.Shard, string, error) {
	tarantoolShardConfigs, mode, err := getTarantoolShardConfigs()
	if err != nil {
		return nil, mode, err
	}
	tarantoolShards := make([]*tntengine.Shard, 0, len(tarantoolShardConfigs))

	for _, tarantoolConf := range tarantoolShardConfigs {
		tarantoolShard, err := tntengine.NewShard(tarantoolConf)
		if err != nil {
			return nil, mode, err
		}
		tarantoolShards = append(tarantoolShards, tarantoolShard)
	}

	if len(tarantoolShards) > 1 {
		mode += "_sharded"
	}

	return tarantoolShards, mode, nil
}

func tarantoolEngine(n *centrifuge.Node) (centrifuge.Broker, centrifuge.PresenceManager, string, error) {
	tarantoolShards, mode, err := getTarantoolShards()
	if err != nil {
		return nil, nil, "", err
	}
	broker, err := tntengine.NewBroker(n, tntengine.BrokerConfig{
		Shards: tarantoolShards,
	})
	if err != nil {
		return nil, nil, "", err
	}
	presenceManager, err := tntengine.NewPresenceManager(n, tntengine.PresenceManagerConfig{
		Shards:      tarantoolShards,
		PresenceTTL: GetDuration("global_presence_ttl", true),
	})
	if err != nil {
		return nil, nil, "", err
	}
	return broker, presenceManager, mode, nil
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
		case centrifuge.LogLevelWarn:
			l = log.Warn()
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
	// HandlerWebtransport enables Webtransport handler (requires HTTP/3)
	HandlerWebtransport
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
	// HandlerUniHTTPStream enables unidirectional HTTP stream endpoint.
	HandlerUniHTTPStream
	// HandlerSSE enables bidirectional SSE endpoint (with emulation layer).
	HandlerSSE
	// HandlerHTTPStream enables bidirectional HTTP stream endpoint (with emulation layer).
	HandlerHTTPStream
	// HandlerEmulation handles client-to-server requests in an emulation layer.
	HandlerEmulation
	// HandlerSwagger handles swagger UI.
	HandlerSwagger
)

var handlerText = map[HandlerFlag]string{
	HandlerWebsocket:     "websocket",
	HandlerSockJS:        "sockjs",
	HandlerWebtransport:  "webtransport",
	HandlerAPI:           "api",
	HandlerAdmin:         "admin",
	HandlerDebug:         "debug",
	HandlerPrometheus:    "prometheus",
	HandlerHealth:        "health",
	HandlerUniWebsocket:  "uni_websocket",
	HandlerUniSSE:        "uni_sse",
	HandlerUniHTTPStream: "uni_http_stream",
	HandlerSSE:           "sse",
	HandlerHTTPStream:    "http_stream",
	HandlerEmulation:     "emulation",
	HandlerSwagger:       "swagger",
}

func (flags HandlerFlag) String() string {
	flagsOrdered := []HandlerFlag{HandlerWebsocket, HandlerSockJS, HandlerWebtransport, HandlerHTTPStream, HandlerSSE, HandlerEmulation, HandlerAPI, HandlerAdmin, HandlerPrometheus, HandlerDebug, HandlerHealth, HandlerUniWebsocket, HandlerUniSSE, HandlerUniHTTPStream, HandlerSwagger}
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
func Mux(n *centrifuge.Node, ruleContainer *rule.Container, apiExecutor *api.Executor, flags HandlerFlag, keepHeadersInContext bool, wtServer *webtransport.Server) *http.ServeMux {
	mux := http.NewServeMux()
	v := viper.GetViper()

	var commonMiddlewares []alice.Constructor

	useLoggingMW := zerolog.GlobalLevel() <= zerolog.DebugLevel
	if useLoggingMW {
		commonMiddlewares = append(commonMiddlewares, middleware.LogRequest)
	}

	basicMiddlewares := append([]alice.Constructor{}, commonMiddlewares...)
	basicChain := alice.New(basicMiddlewares...)

	if flags&HandlerDebug != 0 {
		mux.Handle("/debug/pprof/", basicChain.Then(http.HandlerFunc(pprof.Index)))
		mux.Handle("/debug/pprof/cmdline", basicChain.Then(http.HandlerFunc(pprof.Cmdline)))
		mux.Handle("/debug/pprof/profile", basicChain.Then(http.HandlerFunc(pprof.Profile)))
		mux.Handle("/debug/pprof/symbol", basicChain.Then(http.HandlerFunc(pprof.Symbol)))
		mux.Handle("/debug/pprof/trace", basicChain.Then(http.HandlerFunc(pprof.Trace)))
	}

	if flags&HandlerEmulation != 0 {
		// register bidirectional SSE connection endpoint.
		emulationMiddlewares := append([]alice.Constructor{}, commonMiddlewares...)
		emulationMiddlewares = append(emulationMiddlewares, middleware.NewCORS(getCheckOrigin()).Middleware)
		emulationChain := alice.New(emulationMiddlewares...)

		emulationPrefix := strings.TrimRight(v.GetString("emulation_handler_prefix"), "/")
		if emulationPrefix == "" {
			emulationPrefix = "/"
		}
		mux.Handle(emulationPrefix, emulationChain.Then(centrifuge.NewEmulationHandler(n, emulationHandlerConfig())))
	}

	connMiddlewares := append([]alice.Constructor{}, commonMiddlewares...)
	connLimit := ruleContainer.Config().ClientConnectionLimit
	if connLimit > 0 {
		connLimitMW := middleware.NewConnLimit(n, ruleContainer)
		connMiddlewares = append(connMiddlewares, connLimitMW.Middleware)
	}
	userIDHTTPHeader := v.GetString("client_user_id_http_header")
	if userIDHTTPHeader != "" {
		connMiddlewares = append(connMiddlewares, middleware.UserHeaderAuth(userIDHTTPHeader))
	}
	if keepHeadersInContext {
		connMiddlewares = append(connMiddlewares, middleware.HeadersToContext)
	}
	connMiddlewares = append(connMiddlewares, middleware.NewCORS(getCheckOrigin()).Middleware)
	connChain := alice.New(connMiddlewares...)

	if flags&HandlerWebsocket != 0 {
		// register WebSocket connection endpoint.
		wsPrefix := strings.TrimRight(v.GetString("websocket_handler_prefix"), "/")
		if wsPrefix == "" {
			wsPrefix = "/"
		}
		mux.Handle(wsPrefix, connChain.Then(centrifuge.NewWebsocketHandler(n, websocketHandlerConfig())))
	}

	if flags&HandlerWebtransport != 0 {
		// register WebTransport connection endpoint.
		wtPrefix := strings.TrimRight(v.GetString("webtransport_handler_prefix"), "/")
		if wtPrefix == "" {
			wtPrefix = "/"
		}
		mux.Handle(wtPrefix, connChain.Then(wt.NewHandler(n, wtServer, webTransportHandlerConfig())))
	}

	if flags&HandlerHTTPStream != 0 {
		// register bidirectional HTTP stream connection endpoint.
		streamPrefix := strings.TrimRight(v.GetString("http_stream_handler_prefix"), "/")
		if streamPrefix == "" {
			streamPrefix = "/"
		}
		mux.Handle(streamPrefix, connChain.Then(centrifuge.NewHTTPStreamHandler(n, httpStreamHandlerConfig())))
	}
	if flags&HandlerSSE != 0 {
		// register bidirectional SSE connection endpoint.
		ssePrefix := strings.TrimRight(v.GetString("sse_handler_prefix"), "/")
		if ssePrefix == "" {
			ssePrefix = "/"
		}
		mux.Handle(ssePrefix, connChain.Then(centrifuge.NewSSEHandler(n, sseHandlerConfig())))
	}

	if flags&HandlerSockJS != 0 {
		// register SockJS connection endpoints.
		sockjsConfig := sockjsHandlerConfig()
		sockjsPrefix := strings.TrimRight(v.GetString("sockjs_handler_prefix"), "/")
		sockjsConfig.HandlerPrefix = sockjsPrefix
		mux.Handle(sockjsPrefix+"/", connChain.Then(centrifuge.NewSockjsHandler(n, sockjsConfig)))
	}

	if flags&HandlerUniWebsocket != 0 {
		// register unidirectional WebSocket connection endpoint.
		wsPrefix := strings.TrimRight(v.GetString("uni_websocket_handler_prefix"), "/")
		if wsPrefix == "" {
			wsPrefix = "/"
		}
		mux.Handle(wsPrefix, connChain.Then(uniws.NewHandler(n, uniWebsocketHandlerConfig())))
	}

	if flags&HandlerUniSSE != 0 {
		// register unidirectional SSE connection endpoint.
		ssePrefix := strings.TrimRight(v.GetString("uni_sse_handler_prefix"), "/")
		if ssePrefix == "" {
			ssePrefix = "/"
		}
		mux.Handle(ssePrefix, connChain.Then(unisse.NewHandler(n, uniSSEHandlerConfig())))
	}

	if flags&HandlerUniHTTPStream != 0 {
		// register unidirectional HTTP stream connection endpoint.
		streamPrefix := strings.TrimRight(v.GetString("uni_http_stream_handler_prefix"), "/")
		if streamPrefix == "" {
			streamPrefix = "/"
		}
		mux.Handle(streamPrefix, connChain.Then(unihttpstream.NewHandler(n, uniStreamHandlerConfig())))
	}

	if flags&HandlerAPI != 0 {
		// register HTTP API endpoints.
		httpErrorMode, err := tools.OptionalStringChoice(viper.GetViper(), "api_error_mode", []string{transportErrorMode})
		if err != nil {
			log.Fatal().Msgf("error in config: %v", err)
		}
		useOpenTelemetry := viper.GetBool("opentelemetry") && viper.GetBool("opentelemetry_api")
		apiHandler := api.NewHandler(n, apiExecutor, api.Config{
			UseOpenTelemetry:      useOpenTelemetry,
			UseTransportErrorMode: httpErrorMode == transportErrorMode,
		})
		apiPrefix := strings.TrimRight(v.GetString("api_handler_prefix"), "/")
		if apiPrefix == "" {
			apiPrefix = "/"
		}

		apiChain := func(op string) alice.Chain {
			apiMiddlewares := append([]alice.Constructor{}, commonMiddlewares...)
			otelHandler := middleware.NewOpenTelemetryHandler(op, nil)
			if useOpenTelemetry {
				apiMiddlewares = append(apiMiddlewares, otelHandler.Middleware)
			}
			apiMiddlewares = append(apiMiddlewares, middleware.Post)
			if !viper.GetBool("api_insecure") {
				apiMiddlewares = append(apiMiddlewares, middleware.NewAPIKeyAuth(viper.GetString("api_key")).Middleware)
			}
			apiChain := alice.New(apiMiddlewares...)
			return apiChain
		}

		mux.Handle(apiPrefix, apiChain(apiPrefix).Then(apiHandler.OldRoute()))
		if apiPrefix != "/" {
			for path, handler := range apiHandler.Routes() {
				handlePath := apiPrefix + path
				mux.Handle(handlePath, apiChain(handlePath).Then(handler))
			}
		} else {
			for path, handler := range apiHandler.Routes() {
				mux.Handle(path, apiChain(path).Then(handler))
			}
		}
	}

	if flags&HandlerSwagger != 0 {
		// register Swagger UI endpoint.
		swaggerPrefix := strings.TrimRight(v.GetString("swagger_handler_prefix"), "/") + "/"
		if swaggerPrefix == "" {
			swaggerPrefix = "/"
		}
		mux.Handle(swaggerPrefix, basicChain.Then(http.StripPrefix(swaggerPrefix, http.FileServer(swaggerui.FS))))
	}

	if flags&HandlerPrometheus != 0 {
		// register Prometheus metrics export endpoint.
		prometheusPrefix := strings.TrimRight(v.GetString("prometheus_handler_prefix"), "/")
		if prometheusPrefix == "" {
			prometheusPrefix = "/"
		}
		mux.Handle(prometheusPrefix, basicChain.Then(promhttp.Handler()))
	}

	if flags&HandlerAdmin != 0 {
		// register admin web interface API endpoints.
		adminPrefix := strings.TrimRight(v.GetString("admin_handler_prefix"), "/")
		mux.Handle(adminPrefix+"/", basicChain.Then(admin.NewHandler(n, apiExecutor, adminHandlerConfig())))
	}

	if flags&HandlerHealth != 0 {
		healthPrefix := strings.TrimRight(v.GetString("health_handler_prefix"), "/")
		if healthPrefix == "" {
			healthPrefix = "/"
		}
		mux.Handle(healthPrefix, basicChain.Then(health.NewHandler(n, health.Config{})))
	}

	return mux
}
