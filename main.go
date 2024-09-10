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
	"github.com/centrifugal/centrifugo/v5/internal/config"
	"github.com/centrifugal/centrifugo/v5/internal/configtypes"
	"github.com/centrifugal/centrifugo/v5/internal/consuming"
	"github.com/centrifugal/centrifugo/v5/internal/health"
	"github.com/centrifugal/centrifugo/v5/internal/jwtutils"
	"github.com/centrifugal/centrifugo/v5/internal/jwtverify"
	"github.com/centrifugal/centrifugo/v5/internal/logging"
	"github.com/centrifugal/centrifugo/v5/internal/metrics/graphite"
	"github.com/centrifugal/centrifugo/v5/internal/middleware"
	"github.com/centrifugal/centrifugo/v5/internal/natsbroker"
	"github.com/centrifugal/centrifugo/v5/internal/notify"
	"github.com/centrifugal/centrifugo/v5/internal/origin"
	"github.com/centrifugal/centrifugo/v5/internal/proxy"
	"github.com/centrifugal/centrifugo/v5/internal/redisnatsbroker"
	"github.com/centrifugal/centrifugo/v5/internal/service"
	"github.com/centrifugal/centrifugo/v5/internal/survey"
	"github.com/centrifugal/centrifugo/v5/internal/swaggerui"
	"github.com/centrifugal/centrifugo/v5/internal/telemetry"
	"github.com/centrifugal/centrifugo/v5/internal/tools"
	"github.com/centrifugal/centrifugo/v5/internal/unigrpc"
	"github.com/centrifugal/centrifugo/v5/internal/unihttpstream"
	"github.com/centrifugal/centrifugo/v5/internal/unisse"
	"github.com/centrifugal/centrifugo/v5/internal/uniws"
	"github.com/centrifugal/centrifugo/v5/internal/usage"
	"github.com/centrifugal/centrifugo/v5/internal/webui"
	"github.com/centrifugal/centrifugo/v5/internal/wt"

	"github.com/centrifugal/centrifuge"
	"github.com/justinas/alice"
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
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	_ "google.golang.org/grpc/encoding/gzip"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/reflection"
)

//go:generate go run internal/gen/api/main.go

const edition = "oss"

const transportErrorMode = "transport"

func main() {
	var configFile string

	var rootCmd = &cobra.Command{
		Use:   "",
		Short: "Centrifugo",
		Long:  "Centrifugo – scalable real-time messaging server in language-agnostic way",
		Run: func(cmd *cobra.Command, args []string) {
			cfg, cfgMeta, err := config.GetConfig(cmd, configFile)
			if err != nil {
				log.Fatal().Msgf("error reading config: %v", err)
			}
			logFileHandler := logging.Setup(cfg)
			if logFileHandler != nil {
				defer func() { _ = logFileHandler.Close() }()
			}
			if cfgMeta.FileNotFound {
				log.Warn().Msg("config file not found, continue using environment and flag options")
			} else {
				absConfPath, _ := filepath.Abs(configFile)
				log.Info().Str("path", absConfPath).Msg("using config file")
			}
			err = tools.WritePidFile(cfg.PidFile)
			if err != nil {
				log.Fatal().Msgf("error writing PID: %v", err)
			}
			if os.Getenv("GOMAXPROCS") == "" {
				_, _ = maxprocs.Set()
			}

			engineName := cfg.Engine

			log.Info().
				Str("version", build.Version).
				Str("runtime", runtime.Version()).
				Int("pid", os.Getpid()).
				Str("engine", engineName).
				Int("gomaxprocs", runtime.GOMAXPROCS(0)).Msg("starting Centrifugo")

			err = cfg.Validate()
			if err != nil {
				log.Fatal().Msgf("error validating config: %v", err)
			}
			cfgContainer, err := config.NewContainer(cfg)
			if err != nil {
				log.Fatal().Msgf("error creating config: %v", err)
			}
			cfgContainer.ChannelOptionsCacheTTL = 200 * time.Millisecond

			var proxyMap *client.ProxyMap
			var keepHeadersInContext bool
			if cfg.GranularProxyMode {
				proxyMap, keepHeadersInContext = granularProxyMapConfig(cfg)
				log.Info().Msg("using granular proxy configuration")
			} else {
				proxyMap, keepHeadersInContext = proxyMapConfig(cfg)
			}

			nodeCfg := centrifugeConfig(build.Version, cfg)

			node, err := centrifuge.New(nodeCfg)
			if err != nil {
				log.Fatal().Msgf("error creating Centrifuge Node: %v", err)
			}

			if cfg.OpenTelemetry.Enabled {
				_, err := telemetry.SetupTracing(context.Background())
				if err != nil {
					log.Fatal().Msgf("error setting up opentelemetry tracing: %v", err)
				}
			}

			brokerName := cfg.Broker
			if brokerName != "" && brokerName != "nats" {
				log.Fatal().Msgf("unknown broker: %s", brokerName)
			}

			var broker centrifuge.Broker
			var presenceManager centrifuge.PresenceManager

			var engineMode string

			if engineName == "memory" {
				broker, presenceManager, engineMode, err = memoryEngine(node)
			} else if engineName == "redis" {
				broker, presenceManager, engineMode, err = redisEngine(node, cfg)
			} else if engineName == "redisnats" {
				if !cfg.EnableUnreleasedFeatures {
					log.Fatal().Msg("redisnats engine requires enable_unreleased_features on")
				}
				log.Warn().Msg("redisnats engine is not released, it may be changed or removed at any point")
				var natsBroker *natsbroker.NatsBroker
				var redisBroker *centrifuge.RedisBroker
				redisBroker, presenceManager, engineMode, err = redisEngine(node, cfg)
				if err != nil {
					log.Fatal().Msgf("error creating redis engine: %v", err)
				}
				natsBroker, err = initNatsBroker(node, cfg)
				if err != nil {
					log.Fatal().Msgf("error creating nats broker: %v", err)
				}
				broker, err = redisnatsbroker.New(natsBroker, redisBroker)
			} else {
				log.Fatal().Msgf("unknown engine: %s", engineName)
			}
			if err != nil {
				log.Fatal().Msgf("error creating %s engine: %v", engineName, err)
			}
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

			if brokerName == "nats" {
				broker, err = initNatsBroker(node, cfg)
				if err != nil {
					log.Fatal().Msgf("error creating broker: %v", err)
				}
				node.SetBroker(broker)
			}

			verifierConfig, err := jwtVerifierConfig(cfg)
			if err != nil {
				log.Fatal().Msgf("error creating JWT verifier config: %v", err)
			}

			tokenVerifier, err := jwtverify.NewTokenVerifierJWT(verifierConfig, cfgContainer)
			if err != nil {
				log.Fatal().Msgf("error creating token verifier: %v", err)
			}

			var subTokenVerifier *jwtverify.VerifierJWT
			if cfg.Client.SubscriptionToken.Enabled {
				log.Info().Msg("initializing separate verifier for subscription tokens")
				subVerifier, err := subJWTVerifierConfig(cfg)
				if err != nil {
					log.Fatal().Msgf("error creating subscription JWT verifier config: %v", err)
				}
				subTokenVerifier, err = jwtverify.NewTokenVerifierJWT(subVerifier, cfgContainer)
				if err != nil {
					log.Fatal().Msgf("error creating token verifier: %v", err)
				}
			}

			clientHandler := client.NewHandler(node, cfgContainer, tokenVerifier, subTokenVerifier, proxyMap, cfg.GranularProxyMode)
			err = clientHandler.Setup()
			if err != nil {
				log.Fatal().Msgf("error setting up client handler: %v", err)
			}
			if cfg.RPC.Ping {
				log.Info().Str("method", cfg.RPC.PingMethod).Msg("RPC ping extension enabled")
				clientHandler.SetRPCExtension(cfg.RPC.PingMethod, func(c client.Client, e centrifuge.RPCEvent) (centrifuge.RPCReply, error) {
					return centrifuge.RPCReply{}, nil
				})
			}

			surveyCaller := survey.NewCaller(node)

			useAPIOpentelemetry := cfg.OpenTelemetry.Enabled && cfg.OpenTelemetry.API
			useConsumingOpentelemetry := cfg.OpenTelemetry.Enabled && cfg.OpenTelemetry.Consuming

			httpAPIExecutor := api.NewExecutor(node, cfgContainer, surveyCaller, api.ExecutorConfig{
				Protocol:         "http",
				UseOpenTelemetry: useAPIOpentelemetry,
			})
			grpcAPIExecutor := api.NewExecutor(node, cfgContainer, surveyCaller, api.ExecutorConfig{
				Protocol:         "grpc",
				UseOpenTelemetry: useAPIOpentelemetry,
			})
			consumingAPIExecutor := api.NewExecutor(node, cfgContainer, surveyCaller, api.ExecutorConfig{
				Protocol:         "consuming",
				UseOpenTelemetry: useConsumingOpentelemetry,
			})

			var services []service.Service

			consumingHandler := api.NewConsumingHandler(node, consumingAPIExecutor, api.ConsumingHandlerConfig{
				UseOpenTelemetry: useConsumingOpentelemetry,
			})

			consumingServices, err := consuming.New(node.ID(), node, consumingHandler, cfg.Consumers)
			if err != nil {
				log.Fatal().Msgf("error initializing consumers: %v", err)
			}

			services = append(services, consumingServices...)

			if err = node.Run(); err != nil {
				log.Fatal().Msgf("error running node: %v", err)
			}

			if cfg.Client.Insecure {
				log.Warn().Msg("INSECURE client mode enabled, make sure you understand risks")
			}
			if cfg.HttpAPI.Insecure {
				log.Warn().Msg("INSECURE HTTP API mode enabled, make sure you understand risks")
			}
			if cfg.Admin.Insecure {
				log.Warn().Msg("INSECURE admin mode enabled, make sure you understand risks")
			}
			if cfg.Debug.Enabled {
				log.Warn().Msg("DEBUG mode enabled, see on " + cfg.Debug.HandlerPrefix)
			}

			var grpcAPIServer *grpc.Server
			var grpcAPIAddr string
			if cfg.GrpcAPI.Enabled {
				grpcAPIAddr = net.JoinHostPort(cfg.GrpcAPI.Address, strconv.Itoa(cfg.GrpcAPI.Port))
				grpcAPIConn, err := net.Listen("tcp", grpcAPIAddr)
				if err != nil {
					log.Fatal().Msgf("cannot listen to address %s", grpcAPIAddr)
				}
				var grpcOpts []grpc.ServerOption

				if cfg.GrpcAPI.Key != "" {
					grpcOpts = append(grpcOpts, api.GRPCKeyAuth(cfg.GrpcAPI.Key))
				}
				if cfg.GrpcAPI.MaxReceiveMessageSize > 0 {
					grpcOpts = append(grpcOpts, grpc.MaxRecvMsgSize(cfg.GrpcAPI.MaxReceiveMessageSize))
				}
				var grpcAPITLSConfig *tls.Config
				if cfg.GrpcAPI.TLS.Enabled {
					grpcAPITLSConfig, err = cfg.GrpcAPI.TLS.ToGoTLSConfig()
					if err != nil {
						log.Fatal().Msgf("error getting TLS config for GRPC API: %v", err)
					}
				}
				if grpcAPITLSConfig != nil {
					grpcOpts = append(grpcOpts, grpc.Creds(credentials.NewTLS(grpcAPITLSConfig)))
				}
				if useAPIOpentelemetry {
					grpcOpts = append(grpcOpts, grpc.StatsHandler(otelgrpc.NewServerHandler()))
				}
				grpcErrorMode, err := tools.OptionalStringChoice(cfg.GrpcAPI.ErrorMode, []string{transportErrorMode})
				if err != nil {
					log.Fatal().Msgf("can't extract grpc error mode: %v", err)
				}
				grpcAPIServer = grpc.NewServer(grpcOpts...)
				_ = api.RegisterGRPCServerAPI(node, grpcAPIExecutor, grpcAPIServer, api.GRPCAPIServiceConfig{
					UseOpenTelemetry:      useAPIOpentelemetry,
					UseTransportErrorMode: grpcErrorMode == transportErrorMode,
				})
				if cfg.GrpcAPI.Reflection {
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
			if cfg.UniGRPC.Enabled {
				grpcUniAddr = net.JoinHostPort(cfg.UniGRPC.Address, strconv.Itoa(cfg.UniGRPC.Port))
				grpcUniConn, err := net.Listen("tcp", grpcUniAddr)
				if err != nil {
					log.Fatal().Msgf("cannot listen to address %s", grpcUniAddr)
				}
				var grpcOpts []grpc.ServerOption
				//nolint:staticcheck
				//goland:noinspection GoDeprecation
				grpcOpts = append(grpcOpts, grpc.CustomCodec(&unigrpc.RawCodec{}), grpc.MaxRecvMsgSize(cfg.UniGRPC.MaxReceiveMessageSize))

				var uniGrpcTLSConfig *tls.Config
				if cfg.GrpcAPI.TLS.Enabled {
					uniGrpcTLSConfig, err = cfg.GrpcAPI.TLS.ToGoTLSConfig()
					if err != nil {
						log.Fatal().Msgf("error getting TLS config for uni GRPC: %v", err)
					}
				}
				if uniGrpcTLSConfig != nil {
					grpcOpts = append(grpcOpts, grpc.Creds(credentials.NewTLS(uniGrpcTLSConfig)))
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
				_ = unigrpc.RegisterService(grpcUniServer, unigrpc.NewService(node, cfg.UniGRPC))
				go func() {
					if err := grpcUniServer.Serve(grpcUniConn); err != nil {
						log.Fatal().Msgf("serve uni GRPC: %v", err)
					}
				}()
			}

			if grpcUniServer != nil {
				log.Info().Msgf("serving unidirectional GRPC on %s", grpcUniAddr)
			}

			httpServers, err := runHTTPServers(node, cfgContainer, httpAPIExecutor, keepHeadersInContext)
			if err != nil {
				log.Fatal().Msgf("error running HTTP server: %v", err)
			}

			var exporter *graphite.Exporter
			if cfg.Graphite.Enabled {
				exporter = graphite.New(graphite.Config{
					Address:  net.JoinHostPort(cfg.Graphite.Host, strconv.Itoa(cfg.Graphite.Port)),
					Gatherer: prometheus.DefaultGatherer,
					Prefix:   strings.TrimSuffix(cfg.Graphite.Prefix, ".") + "." + graphite.PreparePathComponent(nodeCfg.Name),
					Interval: cfg.Graphite.Interval,
					Tags:     cfg.Graphite.Tags,
				})
				services = append(services, exporter)
			}

			var statsSender *usage.Sender
			if !cfg.UsageStats.Disabled {
				statsSender = usage.NewSender(node, cfgContainer, usage.Features{
					Edition:    edition,
					Version:    build.Version,
					Engine:     engineName,
					EngineMode: engineMode,
					Broker:     brokerName,
					BrokerMode: "",

					Websocket:     !cfg.WebSocket.Disabled,
					HTTPStream:    cfg.HTTPStream.Enabled,
					SSE:           cfg.SSE.Enabled,
					UniWebsocket:  cfg.UniWS.Enabled,
					UniHTTPStream: cfg.UniHTTPStream.Enabled,
					UniSSE:        cfg.UniSSE.Enabled,
					UniGRPC:       cfg.UniGRPC.Enabled,

					EnabledConsumers: usage.GetEnabledConsumers(cfg.Consumers),

					GrpcAPI:             cfg.GrpcAPI.Enabled,
					SubscribeToPersonal: cfg.UserSubscribeToPersonal.Enabled,
					Admin:               cfg.Admin.Enabled,

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
				services = append(services, statsSender)
			}

			notify.RegisterHandlers(node, statsSender)

			var serviceGroup *errgroup.Group
			serviceCancel := func() {}
			if len(services) > 0 {
				var serviceCtx context.Context
				serviceCtx, serviceCancel = context.WithCancel(context.Background())
				serviceGroup, serviceCtx = errgroup.WithContext(serviceCtx)
				for _, s := range services {
					s := s
					serviceGroup.Go(func() error {
						return s.Run(serviceCtx)
					})
				}
			}

			for _, key := range cfgMeta.UnknownKeys {
				log.Warn().Str("key", key).Msg("unknown key in configuration file")
			}
			for _, key := range cfgMeta.UnknownEnvs {
				log.Warn().Str("var", key).Msg("unknown var in environment")
			}

			handleSignals(
				cmd, configFile, node, cfgContainer, tokenVerifier, subTokenVerifier,
				httpServers, grpcAPIServer, grpcUniServer,
				serviceGroup, serviceCancel,
			)
		},
	}
	rootCmd.Flags().StringVarP(&configFile, "config", "c", "config.json", "path to config file")
	config.DefineFlags(rootCmd)

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
			cfg, cfgMeta, err := config.GetConfig(cmd, configFile)
			if err != nil {
				fmt.Printf("error reading config: %v\n", err)
				os.Exit(1)
			}
			if cfgMeta.FileNotFound {
				fmt.Println("config file not found")
				os.Exit(1)
			}
			err = cfg.Validate()
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
			cfg, _, err := config.GetConfig(cmd, outputConfigFile)
			if err != nil {
				_ = os.Remove(outputConfigFile)
				fmt.Printf("error: %v\n", err)
				os.Exit(1)
			}
			err = cfg.Validate()
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
			cfg, _, err := config.GetConfig(cmd, genTokenConfigFile)
			if err != nil {
				fmt.Printf("error reading config: %v\n", err)
				os.Exit(1)
			}
			verifierConfig, err := jwtVerifierConfig(cfg)
			if err != nil {
				fmt.Printf("error: %v\n", err)
				os.Exit(1)
			}
			token, err := cli.GenerateToken(verifierConfig, genTokenUser, genTokenTTL)
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
			cfg, _, err := config.GetConfig(cmd, genSubTokenConfigFile)
			if err != nil {
				fmt.Printf("error reading config: %v\n", err)
				os.Exit(1)
			}
			if genSubTokenChannel == "" {
				fmt.Println("channel is required")
				os.Exit(1)
			}
			verifierConfig, err := jwtVerifierConfig(cfg)
			if err != nil {
				fmt.Printf("error: %v\n", err)
				os.Exit(1)
			}
			if cfg.Client.SubscriptionToken.Enabled {
				verifierConfig, err = subJWTVerifierConfig(cfg)
				if err != nil {
					fmt.Printf("error: %v\n", err)
					os.Exit(1)
				}
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
			if genSubTokenTTL >= 0 {
				exp = fmt.Sprintf("with expiration TTL %s", time.Duration(genSubTokenTTL)*time.Second)
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
			cfg, _, err := config.GetConfig(cmd, checkTokenConfigFile)
			if err != nil {
				fmt.Printf("error reading config: %v\n", err)
				os.Exit(1)
			}
			if err != nil {
				fmt.Printf("error: %v\n", err)
				os.Exit(1)
			}
			verifierConfig, err := jwtVerifierConfig(cfg)
			if err != nil {
				fmt.Printf("error: %v\n", err)
				os.Exit(1)
			}
			if len(args) != 1 {
				fmt.Printf("error: provide token to check [centrifugo checktoken <TOKEN>]\n")
				os.Exit(1)
			}
			subject, claims, err := cli.CheckToken(verifierConfig, cfg, args[0])
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
			cfg, _, err := config.GetConfig(cmd, checkSubTokenConfigFile)
			if err != nil {
				fmt.Printf("error reading config: %v\n", err)
				os.Exit(1)
			}
			verifierConfig, err := jwtVerifierConfig(cfg)
			if err != nil {
				fmt.Printf("error: %v\n", err)
				os.Exit(1)
			}
			if cfg.Client.SubscriptionToken.Enabled {
				verifierConfig, err = subJWTVerifierConfig(cfg)
				if err != nil {
					fmt.Printf("error: %v\n", err)
					os.Exit(1)
				}
			}
			if len(args) != 1 {
				fmt.Printf("error: provide token to check [centrifugo checksubtoken <TOKEN>]\n")
				os.Exit(1)
			}
			subject, channel, claims, err := cli.CheckSubToken(verifierConfig, cfg, args[0])
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

func handleSignals(
	cmd *cobra.Command, configFile string, n *centrifuge.Node, cfgContainer *config.Container, tokenVerifier *jwtverify.VerifierJWT,
	subTokenVerifier *jwtverify.VerifierJWT, httpServers []*http.Server, grpcAPIServer *grpc.Server, grpcUniServer *grpc.Server,
	serviceGroup *errgroup.Group, serviceCancel context.CancelFunc,
) {
	cfg := cfgContainer.Config()
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGHUP, syscall.SIGINT, os.Interrupt, syscall.SIGTERM)
	for {
		sig := <-sigCh
		log.Info().Msgf("signal received: %v", sig)
		switch sig {
		case syscall.SIGHUP:
			// Reload application configuration on SIGHUP.
			// Note that Centrifugo can't reload config for everything – just best effort to reload what's possible.
			// We can now reload channel options and token verifiers.
			log.Info().Msg("reloading configuration")
			newCfg, _, err := config.GetConfig(cmd, configFile)
			if err != nil {
				log.Error().Msg(tools.ErrorMessageFromConfigError(err, configFile))
				continue
			}
			if err = newCfg.Validate(); err != nil {
				log.Error().Msgf("error validating config: %v", err)
				continue
			}
			verifierConfig, err := jwtVerifierConfig(newCfg)
			if err != nil {
				log.Error().Msgf("error reloading: %v", err)
				continue
			}
			if err = tokenVerifier.Reload(verifierConfig); err != nil {
				log.Error().Msgf("error reloading: %v", err)
				continue
			}
			if subTokenVerifier != nil {
				subVerifierConfig, err := subJWTVerifierConfig(newCfg)
				if err != nil {
					log.Error().Msgf("error reloading: %v", err)
					continue
				}
				if err := subTokenVerifier.Reload(subVerifierConfig); err != nil {
					log.Error().Msgf("error reloading: %v", err)
					continue
				}
			}
			if err = cfgContainer.Reload(newCfg); err != nil {
				log.Error().Msgf("error reloading: %v", err)
				continue
			}
			log.Info().Msg("configuration successfully reloaded")
		case syscall.SIGINT, os.Interrupt, syscall.SIGTERM:
			log.Info().Msg("shutting down ...")
			pidFile := cfg.PidFile
			shutdownTimeout := cfg.Shutdown.Timeout
			go time.AfterFunc(shutdownTimeout, func() {
				if pidFile != "" {
					_ = os.Remove(pidFile)
				}
				os.Exit(1)
			})

			var wg sync.WaitGroup

			if serviceGroup != nil {
				serviceCancel()
				wg.Add(1)
				go func() {
					defer wg.Done()
					_ = serviceGroup.Wait()
				}()
			}

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
			os.Exit(0)
		}
	}
}

var startHTTPChallengeServerOnce sync.Once

func getTLSConfig(cfg config.Config) (*tls.Config, error) {
	tlsEnabled := cfg.TLS.Enabled
	tlsAutocertEnabled := cfg.TLSAutocert.Enabled
	tlsAutocertHostWhitelist := cfg.TLSAutocert.HostWhitelist
	tlsAutocertCacheDir := cfg.TLSAutocert.CacheDir
	tlsAutocertEmail := cfg.TLSAutocert.Email
	tlsAutocertServerName := cfg.TLSAutocert.ServerName
	tlsAutocertHTTP := cfg.TLSAutocert.HTTP
	tlsAutocertHTTPAddr := cfg.TLSAutocert.HTTPAddr

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
		return cfg.TLS.ToGoTLSConfig()
	}

	return nil, nil
}

type httpErrorLogWriter struct {
	zerolog.Logger
}

func (w *httpErrorLogWriter) Write(data []byte) (int, error) {
	w.Logger.Warn().Msg(strings.TrimSpace(string(data)))
	return len(data), nil
}

func runHTTPServers(n *centrifuge.Node, cfgContainer *config.Container, apiExecutor *api.Executor, keepHeadersInContext bool) ([]*http.Server, error) {
	cfg := cfgContainer.Config()

	debug := cfg.Debug.Enabled
	useAdmin := cfg.Admin.Enabled
	usePrometheus := cfg.Prometheus.Enabled
	useHealth := cfg.Health.Enabled
	useSwagger := cfg.Swagger.Enabled

	adminExternal := cfg.Admin.External
	apiExternal := cfg.HttpAPI.External

	apiDisabled := cfg.HttpAPI.Disabled

	httpAddress := cfg.Address
	httpPort := strconv.Itoa(cfg.Port)
	httpInternalAddress := cfg.InternalAddress
	httpInternalPort := cfg.InternalPort

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
	if !cfg.WebSocket.Disabled {
		portFlags |= HandlerWebsocket
	}
	if cfg.WebTransport.Enabled {
		if !cfg.HTTP3 {
			log.Fatal().Msg("can not enable webtransport without experimental HTTP/3")
		}
		portFlags |= HandlerWebtransport
	}
	if cfg.SSE.Enabled {
		portFlags |= HandlerSSE
	}
	if cfg.HTTPStream.Enabled {
		portFlags |= HandlerHTTPStream
	}
	if cfg.SSE.Enabled || cfg.HTTPStream.Enabled {
		portFlags |= HandlerEmulation
	}
	if useAdmin && adminExternal {
		portFlags |= HandlerAdmin
	}
	if !apiDisabled && apiExternal {
		portFlags |= HandlerAPI
	}
	if cfg.UniWS.Enabled {
		portFlags |= HandlerUniWebsocket
	}
	if cfg.UniSSE.Enabled {
		portFlags |= HandlerUniSSE
	}
	if cfg.UniHTTPStream.Enabled {
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

	tlsConfig, err := getTLSConfig(cfg)
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
		if !cfg.TLSExternal || addr == externalAddr {
			addrTLSConfig = tlsConfig
		}

		useHTTP3 := cfg.HTTP3 && addr == externalAddr

		var wtServer *webtransport.Server
		if useHTTP3 {
			wtServer = &webtransport.Server{
				CheckOrigin: getCheckOrigin(cfg),
			}
		}

		mux := Mux(n, cfgContainer, apiExecutor, handlerFlags, keepHeadersInContext, wtServer)

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
				_ = wtServer.H3.SetQUICHeaders(w.Header())
				mux.ServeHTTP(w, r)
			})
		}

		servers = append(servers, server)

		go func() {
			if useHTTP3 {
				if addrTLSConfig == nil {
					log.Fatal().Msgf("HTTP/3 requires TLS configured")
				}
				if cfg.TLSAutocert.Enabled {
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
					if !errors.Is(err, http.ErrServerClosed) {
						log.Fatal().Msgf("ListenAndServe: %v", err)
					}
				case err := <-qErr:
					// Cannot close the HTTP server or wait for requests to complete properly.
					log.Fatal().Msgf("ListenAndServe HTTP/3: %v", err)
				}
			} else {
				if addrTLSConfig != nil {
					if err := server.ListenAndServeTLS("", ""); err != nil {
						if !errors.Is(err, http.ErrServerClosed) {
							log.Fatal().Msgf("ListenAndServe: %v", err)
						}
					}
				} else {
					if err := server.ListenAndServe(); err != nil {
						if !errors.Is(err, http.ErrServerClosed) {
							log.Fatal().Msgf("ListenAndServe: %v", err)
						}
					}
				}
			}
		}()
	}

	return servers, nil
}

// Now Centrifugo uses https://github.com/tidwall/gjson to extract custom claims from JWT. So technically
// we could support extracting from nested objects using dot syntax, like "centrifugo.user". But for now
// not using this feature to keep things simple until necessary.
var customClaimRe = regexp.MustCompile("^[a-zA-Z_]+$")

func makeVerifierConfig(tokenConf configtypes.Token) (jwtverify.VerifierConfig, error) {
	cfg := jwtverify.VerifierConfig{}

	cfg.HMACSecretKey = tokenConf.HMACSecretKey

	rsaPublicKey := tokenConf.RSAPublicKey
	if rsaPublicKey != "" {
		pubKey, err := jwtutils.ParseRSAPublicKeyFromPEM([]byte(rsaPublicKey))
		if err != nil {
			return jwtverify.VerifierConfig{}, fmt.Errorf("error parsing RSA public key: %w", err)
		}
		cfg.RSAPublicKey = pubKey
	}

	ecdsaPublicKey := tokenConf.ECDSAPublicKey
	if ecdsaPublicKey != "" {
		pubKey, err := jwtutils.ParseECDSAPublicKeyFromPEM([]byte(ecdsaPublicKey))
		if err != nil {
			return jwtverify.VerifierConfig{}, fmt.Errorf("error parsing ECDSA public key: %w", err)
		}
		cfg.ECDSAPublicKey = pubKey
	}

	cfg.JWKSPublicEndpoint = tokenConf.JWKSPublicEndpoint
	cfg.Audience = tokenConf.Audience
	cfg.AudienceRegex = tokenConf.AudienceRegex
	cfg.Issuer = tokenConf.Issuer
	cfg.IssuerRegex = tokenConf.IssuerRegex

	if tokenConf.UserIDClaim != "" {
		customUserIDClaim := tokenConf.UserIDClaim
		if !customClaimRe.MatchString(customUserIDClaim) {
			return jwtverify.VerifierConfig{}, fmt.Errorf("invalid user ID claim: %s, must match %s regular expression", customUserIDClaim, customClaimRe.String())
		}
		cfg.UserIDClaim = customUserIDClaim
	}

	return cfg, nil
}

func jwtVerifierConfig(cfg config.Config) (jwtverify.VerifierConfig, error) {
	return makeVerifierConfig(cfg.Client.Token)
}

func subJWTVerifierConfig(cfg config.Config) (jwtverify.VerifierConfig, error) {
	return makeVerifierConfig(cfg.Client.SubscriptionToken.Token)
}

func proxyMapConfig(cfg config.Config) (*client.ProxyMap, bool) {
	proxyMap := &client.ProxyMap{
		SubscribeProxies:       map[string]proxy.SubscribeProxy{},
		PublishProxies:         map[string]proxy.PublishProxy{},
		RpcProxies:             map[string]proxy.RPCProxy{},
		SubRefreshProxies:      map[string]proxy.SubRefreshProxy{},
		SubscribeStreamProxies: map[string]*proxy.SubscribeStreamProxy{},
	}
	proxyConfig := proxy.Config{
		ProxyCommon: cfg.Proxy.ProxyCommon,
	}

	connectEndpoint := cfg.Proxy.ConnectEndpoint
	connectTimeout := cfg.Proxy.ConnectTimeout
	refreshEndpoint := cfg.Proxy.RefreshEndpoint
	refreshTimeout := cfg.Proxy.RefreshTimeout
	rpcEndpoint := cfg.Proxy.RPCEndpoint
	rpcTimeout := cfg.Proxy.RPCTimeout
	subscribeEndpoint := cfg.Proxy.SubscribeEndpoint
	subscribeTimeout := cfg.Proxy.SubscribeTimeout
	publishEndpoint := cfg.Proxy.PublishEndpoint
	publishTimeout := cfg.Proxy.PublishTimeout
	subRefreshEndpoint := cfg.Proxy.SubRefreshEndpoint
	subRefreshTimeout := cfg.Proxy.SubRefreshTimeout
	proxyStreamSubscribeEndpoint := cfg.Proxy.StreamSubscribeEndpoint
	if strings.HasPrefix(proxyStreamSubscribeEndpoint, "http") {
		log.Fatal().Msg("error creating subscribe stream proxy: only GRPC endpoints supported")
	}
	proxyStreamSubscribeTimeout := cfg.Proxy.StreamSubscribeTimeout

	if connectEndpoint != "" {
		proxyConfig.Endpoint = connectEndpoint
		proxyConfig.Timeout = connectTimeout
		var err error
		proxyMap.ConnectProxy, err = proxy.GetConnectProxy(proxyConfig)
		if err != nil {
			log.Fatal().Msgf("error creating connect proxy: %v", err)
		}
		log.Info().Str("endpoint", connectEndpoint).Msg("connect proxy enabled")
	}

	if refreshEndpoint != "" {
		proxyConfig.Endpoint = refreshEndpoint
		proxyConfig.Timeout = refreshTimeout
		var err error
		proxyMap.RefreshProxy, err = proxy.GetRefreshProxy(proxyConfig)
		if err != nil {
			log.Fatal().Msgf("error creating refresh proxy: %v", err)
		}
		log.Info().Str("endpoint", refreshEndpoint).Msg("refresh proxy enabled")
	}

	if subscribeEndpoint != "" {
		proxyConfig.Endpoint = subscribeEndpoint
		proxyConfig.Timeout = subscribeTimeout
		sp, err := proxy.GetSubscribeProxy(proxyConfig)
		if err != nil {
			log.Fatal().Msgf("error creating subscribe proxy: %v", err)
		}
		proxyMap.SubscribeProxies[""] = sp
		log.Info().Str("endpoint", subscribeEndpoint).Msg("subscribe proxy enabled")
	}

	if publishEndpoint != "" {
		proxyConfig.Endpoint = publishEndpoint
		proxyConfig.Timeout = publishTimeout
		pp, err := proxy.GetPublishProxy(proxyConfig)
		if err != nil {
			log.Fatal().Msgf("error creating publish proxy: %v", err)
		}
		proxyMap.PublishProxies[""] = pp
		log.Info().Str("endpoint", publishEndpoint).Msg("publish proxy enabled")
	}

	if rpcEndpoint != "" {
		proxyConfig.Endpoint = rpcEndpoint
		proxyConfig.Timeout = rpcTimeout
		rp, err := proxy.GetRpcProxy(proxyConfig)
		if err != nil {
			log.Fatal().Msgf("error creating rpc proxy: %v", err)
		}
		proxyMap.RpcProxies[""] = rp
		log.Info().Str("endpoint", rpcEndpoint).Msg("RPC proxy enabled")
	}

	if subRefreshEndpoint != "" {
		proxyConfig.Endpoint = subRefreshEndpoint
		proxyConfig.Timeout = subRefreshTimeout
		srp, err := proxy.GetSubRefreshProxy(proxyConfig)
		if err != nil {
			log.Fatal().Msgf("error creating sub refresh proxy: %v", err)
		}
		proxyMap.SubRefreshProxies[""] = srp
		log.Info().Str("endpoint", subRefreshEndpoint).Msg("sub refresh proxy enabled")
	}

	if proxyStreamSubscribeEndpoint != "" {
		proxyConfig.Endpoint = proxyStreamSubscribeEndpoint
		proxyConfig.Timeout = proxyStreamSubscribeTimeout
		streamProxy, err := proxy.NewSubscribeStreamProxy(proxyConfig)
		if err != nil {
			log.Fatal().Msgf("error creating subscribe stream proxy: %v", err)
		}
		proxyMap.SubscribeStreamProxies[""] = streamProxy
		log.Info().Str("endpoint", proxyStreamSubscribeEndpoint).Msg("subscribe stream proxy enabled")
	}

	keepHeadersInContext := connectEndpoint != "" || refreshEndpoint != "" ||
		rpcEndpoint != "" || subscribeEndpoint != "" || publishEndpoint != "" ||
		subRefreshEndpoint != "" || proxyStreamSubscribeEndpoint != ""

	return proxyMap, keepHeadersInContext
}

func granularProxyMapConfig(cfg config.Config) (*client.ProxyMap, bool) {
	proxyMap := &client.ProxyMap{
		RpcProxies:             map[string]proxy.RPCProxy{},
		PublishProxies:         map[string]proxy.PublishProxy{},
		SubscribeProxies:       map[string]proxy.SubscribeProxy{},
		SubRefreshProxies:      map[string]proxy.SubRefreshProxy{},
		SubscribeStreamProxies: map[string]*proxy.SubscribeStreamProxy{},
		CacheEmptyProxies:      map[string]proxy.CacheEmptyProxy{},
	}
	proxyList := cfg.Proxies
	proxies := make(map[string]proxy.Config)
	for _, p := range proxyList {
		for i, header := range p.HttpHeaders {
			p.HttpHeaders[i] = strings.ToLower(header)
		}
		proxies[p.Name] = p
	}

	var keepHeadersInContext bool

	connectProxyName := cfg.ConnectProxyName
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
		keepHeadersInContext = true
	}
	refreshProxyName := cfg.RefreshProxyName
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
		keepHeadersInContext = true
	}
	subscribeProxyName := cfg.Channel.SubscribeProxyName
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
		keepHeadersInContext = true
	}

	publishProxyName := cfg.Channel.PublishProxyName
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
		keepHeadersInContext = true
	}

	subRefreshProxyName := cfg.Channel.SubRefreshProxyName
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
		keepHeadersInContext = true
	}

	subscribeStreamProxyName := cfg.Channel.SubscribeStreamProxyName
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
		keepHeadersInContext = true
	}

	for _, ns := range cfg.Channel.Namespaces {
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
			keepHeadersInContext = true
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
			keepHeadersInContext = true
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
			keepHeadersInContext = true
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
			keepHeadersInContext = true
		}
	}

	rpcProxyName := cfg.RPC.RpcProxyName
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
		keepHeadersInContext = true
	}

	for _, ns := range cfg.RPC.Namespaces {
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
			keepHeadersInContext = true
		}
	}

	return proxyMap, keepHeadersInContext
}

func centrifugeConfig(version string, appCfg config.Config) centrifuge.Config {
	cfg := centrifuge.Config{}
	cfg.Version = version
	cfg.MetricsNamespace = "centrifugo"
	cfg.Name = applicationName(appCfg)
	cfg.ChannelMaxLength = appCfg.Channel.MaxLength
	cfg.ClientPresenceUpdateInterval = appCfg.Client.PresenceUpdateInterval
	cfg.ClientExpiredCloseDelay = appCfg.Client.ExpiredCloseDelay
	cfg.ClientExpiredSubCloseDelay = appCfg.Client.ExpiredSubCloseDelay
	cfg.ClientStaleCloseDelay = appCfg.Client.StaleCloseDelay
	cfg.ClientQueueMaxSize = appCfg.Client.QueueMaxSize
	cfg.ClientChannelLimit = appCfg.Client.ChannelLimit
	cfg.ClientChannelPositionCheckDelay = appCfg.Client.ChannelPositionCheckDelay
	cfg.ClientChannelPositionMaxTimeLag = appCfg.Client.ChannelPositionMaxTimeLag
	cfg.UserConnectionLimit = appCfg.Client.UserConnectionLimit
	cfg.NodeInfoMetricsAggregateInterval = appCfg.NodeInfoMetricsAggregateInterval
	cfg.HistoryMaxPublicationLimit = appCfg.Client.HistoryMaxPublicationLimit
	cfg.RecoveryMaxPublicationLimit = appCfg.Client.RecoveryMaxPublicationLimit
	cfg.HistoryMetaTTL = appCfg.GlobalHistoryMetaTTL // TODO: v6. GetDuration("global_history_meta_ttl", true)
	cfg.ClientConnectIncludeServerTime = appCfg.Client.ConnectIncludeServerTime
	cfg.LogLevel = logging.CentrifugeLogLevel(strings.ToLower(appCfg.LogLevel))
	cfg.LogHandler = logging.NewCentrifugeLogHandler().Handle
	return cfg
}

// applicationName returns a name for this centrifuge. If no name provided
// in configuration then it constructs node name based on hostname and port
func applicationName(cfg config.Config) string {
	name := cfg.Name
	if name != "" {
		return name
	}
	port := strconv.Itoa(cfg.Port)
	var hostname string
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "?"
	}
	return hostname + "_" + port
}

func getPingPongConfig(cfg config.Config) centrifuge.PingPongConfig {
	pingInterval := cfg.Client.PingInterval
	pongTimeout := cfg.Client.PongTimeout
	if pingInterval <= pongTimeout {
		log.Fatal().Msgf("ping_interval (%s) must be greater than pong_timeout (%s)", pingInterval, pongTimeout)
	}
	return centrifuge.PingPongConfig{
		PingInterval: pingInterval,
		PongTimeout:  pongTimeout,
	}
}

func websocketHandlerConfig(appCfg config.Config) centrifuge.WebsocketConfig {
	cfg := centrifuge.WebsocketConfig{}
	cfg.Compression = appCfg.WebSocket.Compression
	cfg.CompressionLevel = appCfg.WebSocket.CompressionLevel
	cfg.CompressionMinSize = appCfg.WebSocket.CompressionMinSize
	cfg.ReadBufferSize = appCfg.WebSocket.ReadBufferSize
	cfg.WriteBufferSize = appCfg.WebSocket.WriteBufferSize
	cfg.UseWriteBufferPool = appCfg.WebSocket.UseWriteBufferPool
	cfg.WriteTimeout = appCfg.WebSocket.WriteTimeout
	cfg.MessageSizeLimit = appCfg.WebSocket.MessageSizeLimit
	cfg.CheckOrigin = getCheckOrigin(appCfg)
	cfg.PingPongConfig = getPingPongConfig(appCfg)
	return cfg
}

func httpStreamHandlerConfig(appCfg config.Config) centrifuge.HTTPStreamConfig {
	return centrifuge.HTTPStreamConfig{
		MaxRequestBodySize: appCfg.HTTPStream.MaxRequestBodySize,
		PingPongConfig:     getPingPongConfig(appCfg),
	}
}

func sseHandlerConfig(appCfg config.Config) centrifuge.SSEConfig {
	return centrifuge.SSEConfig{
		MaxRequestBodySize: appCfg.SSE.MaxRequestBodySize,
		PingPongConfig:     getPingPongConfig(appCfg),
	}
}

func emulationHandlerConfig(cfg config.Config) centrifuge.EmulationConfig {
	return centrifuge.EmulationConfig{
		MaxRequestBodySize: cfg.Emulation.MaxRequestBodySize,
	}
}

var warnAllowedOriginsOnce sync.Once

func getCheckOrigin(cfg config.Config) func(r *http.Request) bool {
	allowedOrigins := cfg.Client.AllowedOrigins
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

func addRedisShardCommonSettings(shardConf *centrifuge.RedisShardConfig, redisConf configtypes.Redis) {
	shardConf.DB = redisConf.DB
	shardConf.User = redisConf.User
	shardConf.Password = redisConf.Password
	shardConf.ClientName = redisConf.ClientName

	if redisConf.TLS.Enabled {
		tlsConfig, err := redisConf.TLS.ToGoTLSConfig()
		if err != nil {
			log.Fatal().Msgf("error creating Redis TLS config: %v", err)
		}
		shardConf.TLSConfig = tlsConfig
	}
	shardConf.ConnectTimeout = redisConf.ConnectTimeout
	shardConf.IOTimeout = redisConf.IOTimeout
	shardConf.ForceRESP2 = redisConf.ForceResp2
}

func getRedisShardConfigs(redisConf configtypes.Redis) ([]centrifuge.RedisShardConfig, string, error) {
	var shardConfigs []centrifuge.RedisShardConfig

	clusterShards := redisConf.ClusterAddress
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
			addRedisShardCommonSettings(conf, redisConf)
			shardConfigs = append(shardConfigs, *conf)
		}
		return shardConfigs, "cluster", nil
	}

	sentinelShards := redisConf.SentinelAddress
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
			addRedisShardCommonSettings(conf, redisConf)
			conf.SentinelUser = redisConf.SentinelUser
			conf.SentinelPassword = redisConf.SentinelPassword
			conf.SentinelMasterName = redisConf.SentinelMasterName
			if conf.SentinelMasterName == "" {
				return nil, "", fmt.Errorf("master name must be set when using Redis Sentinel")
			}
			conf.SentinelClientName = redisConf.SentinelClientName
			if redisConf.SentinelTLS.Enabled {
				tlsConfig, err := redisConf.TLS.ToGoTLSConfig()
				if err != nil {
					log.Fatal().Msgf("error creating Redis Sentinel TLS config: %v", err)
				}
				conf.SentinelTLSConfig = tlsConfig
			}
			shardConfigs = append(shardConfigs, *conf)
		}
		return shardConfigs, "sentinel", nil
	}

	redisAddresses := redisConf.Address
	if len(redisAddresses) == 0 {
		redisAddresses = []string{"127.0.0.1:6379"}
	}
	for _, redisAddress := range redisAddresses {
		conf := &centrifuge.RedisShardConfig{
			Address: redisAddress,
		}
		addRedisShardCommonSettings(conf, redisConf)
		shardConfigs = append(shardConfigs, *conf)
	}

	return shardConfigs, "standalone", nil
}

func getRedisShards(n *centrifuge.Node, redisConf configtypes.Redis) ([]*centrifuge.RedisShard, string, error) {
	redisShardConfigs, mode, err := getRedisShardConfigs(redisConf)
	if err != nil {
		return nil, "", err
	}
	redisShards := make([]*centrifuge.RedisShard, 0, len(redisShardConfigs))

	for _, shardConf := range redisShardConfigs {
		redisShard, err := centrifuge.NewRedisShard(n, shardConf)
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

func initNatsBroker(node *centrifuge.Node, cfg config.Config) (*natsbroker.NatsBroker, error) {
	return natsbroker.New(node, cfg.Nats)
}

func redisEngine(n *centrifuge.Node, cfg config.Config) (*centrifuge.RedisBroker, centrifuge.PresenceManager, string, error) {
	redisShards, mode, err := getRedisShards(n, cfg.Redis.Redis)
	if err != nil {
		return nil, nil, "", err
	}

	broker, err := centrifuge.NewRedisBroker(n, centrifuge.RedisBrokerConfig{
		Shards:     redisShards,
		Prefix:     cfg.Redis.Redis.Prefix,
		UseLists:   cfg.Redis.UseLists,
		SkipPubSub: cfg.Broker == "redisnats",
	})
	if err != nil {
		return nil, nil, "", err
	}

	presenceManagerConfig := centrifuge.RedisPresenceManagerConfig{
		Shards:          redisShards,
		Prefix:          cfg.Redis.Prefix,
		PresenceTTL:     cfg.Redis.PresenceTTL,
		UseHashFieldTTL: cfg.Redis.PresenceHashFieldTTL,
	}
	if cfg.Redis.PresenceUserMapping {
		presenceManagerConfig.EnableUserMapping = func(_ string) bool {
			return true
		}
	}

	presenceManager, err := centrifuge.NewRedisPresenceManager(n, presenceManagerConfig)
	if err != nil {
		return nil, nil, "", err
	}

	return broker, presenceManager, mode, nil
}

// HandlerFlag is a bit mask of handlers that must be enabled in mux.
type HandlerFlag int

const (
	// HandlerWebsocket enables Raw Websocket handler.
	HandlerWebsocket HandlerFlag = 1 << iota
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
	flagsOrdered := []HandlerFlag{HandlerWebsocket, HandlerWebtransport, HandlerHTTPStream, HandlerSSE, HandlerEmulation, HandlerAPI, HandlerAdmin, HandlerPrometheus, HandlerDebug, HandlerHealth, HandlerUniWebsocket, HandlerUniSSE, HandlerUniHTTPStream, HandlerSwagger}
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
func Mux(n *centrifuge.Node, cfgContainer *config.Container, apiExecutor *api.Executor, flags HandlerFlag, keepHeadersInContext bool, wtServer *webtransport.Server) *http.ServeMux {
	mux := http.NewServeMux()
	cfg := cfgContainer.Config()

	var commonMiddlewares []alice.Constructor

	useLoggingMW := zerolog.GlobalLevel() <= zerolog.DebugLevel
	if useLoggingMW {
		commonMiddlewares = append(commonMiddlewares, middleware.LogRequest)
	}

	basicMiddlewares := append([]alice.Constructor{}, commonMiddlewares...)
	basicChain := alice.New(basicMiddlewares...)

	if flags&HandlerDebug != 0 {
		mux.Handle(cfg.Debug.HandlerPrefix+"/", basicChain.Then(http.HandlerFunc(pprof.Index)))
		mux.Handle(cfg.Debug.HandlerPrefix+"/cmdline", basicChain.Then(http.HandlerFunc(pprof.Cmdline)))
		mux.Handle(cfg.Debug.HandlerPrefix+"/profile", basicChain.Then(http.HandlerFunc(pprof.Profile)))
		mux.Handle(cfg.Debug.HandlerPrefix+"/symbol", basicChain.Then(http.HandlerFunc(pprof.Symbol)))
		mux.Handle(cfg.Debug.HandlerPrefix+"/trace", basicChain.Then(http.HandlerFunc(pprof.Trace)))
	}

	if flags&HandlerEmulation != 0 {
		// register bidirectional SSE connection endpoint.
		emulationMiddlewares := append([]alice.Constructor{}, commonMiddlewares...)
		emulationMiddlewares = append(emulationMiddlewares, middleware.NewCORS(getCheckOrigin(cfg)).Middleware)
		emulationChain := alice.New(emulationMiddlewares...)

		emulationPrefix := strings.TrimRight(cfg.Emulation.HandlerPrefix, "/")
		if emulationPrefix == "" {
			emulationPrefix = "/"
		}
		mux.Handle(emulationPrefix, emulationChain.Then(centrifuge.NewEmulationHandler(n, emulationHandlerConfig(cfg))))
	}

	connMiddlewares := append([]alice.Constructor{}, commonMiddlewares...)
	connLimit := cfg.Client.ConnectionLimit
	if connLimit > 0 {
		connLimitMW := middleware.NewConnLimit(n, cfgContainer)
		connMiddlewares = append(connMiddlewares, connLimitMW.Middleware)
	}
	userIDHTTPHeader := cfg.Client.UserIDHTTPHeader
	if userIDHTTPHeader != "" {
		connMiddlewares = append(connMiddlewares, middleware.UserHeaderAuth(userIDHTTPHeader))
	}
	if keepHeadersInContext {
		connMiddlewares = append(connMiddlewares, middleware.HeadersToContext)
	}
	connMiddlewares = append(connMiddlewares, middleware.NewCORS(getCheckOrigin(cfg)).Middleware)
	connChain := alice.New(connMiddlewares...)

	if flags&HandlerWebsocket != 0 {
		// register WebSocket connection endpoint.
		wsPrefix := strings.TrimRight(cfg.WebSocket.HandlerPrefix, "/")
		if wsPrefix == "" {
			wsPrefix = "/"
		}
		mux.Handle(wsPrefix, connChain.Then(centrifuge.NewWebsocketHandler(n, websocketHandlerConfig(cfg))))
	}

	if flags&HandlerWebtransport != 0 {
		// register WebTransport connection endpoint.
		wtPrefix := strings.TrimRight(cfg.WebTransport.HandlerPrefix, "/")
		if wtPrefix == "" {
			wtPrefix = "/"
		}
		mux.Handle(wtPrefix, connChain.Then(wt.NewHandler(n, wtServer, cfg.WebTransport, getPingPongConfig(cfg))))
	}

	if flags&HandlerHTTPStream != 0 {
		// register bidirectional HTTP stream connection endpoint.
		streamPrefix := strings.TrimRight(cfg.HTTPStream.HandlerPrefix, "/")
		if streamPrefix == "" {
			streamPrefix = "/"
		}
		mux.Handle(streamPrefix, connChain.Then(centrifuge.NewHTTPStreamHandler(n, httpStreamHandlerConfig(cfg))))
	}
	if flags&HandlerSSE != 0 {
		// register bidirectional SSE connection endpoint.
		ssePrefix := strings.TrimRight(cfg.SSE.HandlerPrefix, "/")
		if ssePrefix == "" {
			ssePrefix = "/"
		}
		mux.Handle(ssePrefix, connChain.Then(centrifuge.NewSSEHandler(n, sseHandlerConfig(cfg))))
	}

	if flags&HandlerUniWebsocket != 0 {
		// register unidirectional WebSocket connection endpoint.
		wsPrefix := strings.TrimRight(cfg.UniWS.HandlerPrefix, "/")
		if wsPrefix == "" {
			wsPrefix = "/"
		}
		mux.Handle(wsPrefix, connChain.Then(
			uniws.NewHandler(n, cfg.UniWS, getCheckOrigin(cfg), getPingPongConfig(cfg))))
	}

	if flags&HandlerUniSSE != 0 {
		// register unidirectional SSE connection endpoint.
		ssePrefix := strings.TrimRight(cfg.UniSSE.HandlerPrefix, "/")
		if ssePrefix == "" {
			ssePrefix = "/"
		}
		mux.Handle(ssePrefix, connChain.Then(unisse.NewHandler(n, cfg.UniSSE, getPingPongConfig(cfg))))
	}

	if flags&HandlerUniHTTPStream != 0 {
		// register unidirectional HTTP stream connection endpoint.
		streamPrefix := strings.TrimRight(cfg.UniHTTPStream.HandlerPrefix, "/")
		if streamPrefix == "" {
			streamPrefix = "/"
		}
		mux.Handle(streamPrefix, connChain.Then(unihttpstream.NewHandler(n, cfg.UniHTTPStream, getPingPongConfig(cfg))))
	}

	if flags&HandlerAPI != 0 {
		// register HTTP API endpoints.
		httpErrorMode, err := tools.OptionalStringChoice(cfg.HttpAPI.ErrorMode, []string{transportErrorMode})
		if err != nil {
			log.Fatal().Msgf("error in config: %v", err)
		}
		useOpenTelemetry := cfg.OpenTelemetry.Enabled && cfg.OpenTelemetry.API
		apiHandler := api.NewHandler(n, apiExecutor, api.Config{
			UseOpenTelemetry:      useOpenTelemetry,
			UseTransportErrorMode: httpErrorMode == transportErrorMode,
		})
		apiPrefix := strings.TrimRight(cfg.HttpAPI.HandlerPrefix, "/")
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
			if !cfg.HttpAPI.Insecure {
				apiMiddlewares = append(apiMiddlewares, middleware.NewAPIKeyAuth(cfg.HttpAPI.Key).Middleware)
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
		swaggerPrefix := strings.TrimRight(cfg.Swagger.HandlerPrefix, "/") + "/"
		if swaggerPrefix == "" {
			swaggerPrefix = "/"
		}
		mux.Handle(swaggerPrefix, basicChain.Then(http.StripPrefix(swaggerPrefix, http.FileServer(swaggerui.FS))))
	}

	if flags&HandlerPrometheus != 0 {
		// register Prometheus metrics export endpoint.
		prometheusPrefix := strings.TrimRight(cfg.Prometheus.HandlerPrefix, "/")
		if prometheusPrefix == "" {
			prometheusPrefix = "/"
		}
		mux.Handle(prometheusPrefix, basicChain.Then(promhttp.Handler()))
	}

	if flags&HandlerAdmin != 0 {
		// register admin web interface API endpoints.
		cfg.Admin.WebFS = webui.FS
		adminPrefix := strings.TrimRight(cfg.Admin.HandlerPrefix, "/")
		mux.Handle(adminPrefix+"/", basicChain.Then(admin.NewHandler(n, apiExecutor, cfg.Admin)))
	}

	if flags&HandlerHealth != 0 {
		healthPrefix := strings.TrimRight(cfg.Health.HandlerPrefix, "/")
		if healthPrefix == "" {
			healthPrefix = "/"
		}
		mux.Handle(healthPrefix, basicChain.Then(health.NewHandler(n, health.Config{})))
	}

	return mux
}
