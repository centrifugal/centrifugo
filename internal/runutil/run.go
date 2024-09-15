package runutil

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/centrifugal/centrifugo/v5/internal/api"
	"github.com/centrifugal/centrifugo/v5/internal/build"
	"github.com/centrifugal/centrifugo/v5/internal/client"
	"github.com/centrifugal/centrifugo/v5/internal/config"
	"github.com/centrifugal/centrifugo/v5/internal/consuming"
	"github.com/centrifugal/centrifugo/v5/internal/jwtverify"
	"github.com/centrifugal/centrifugo/v5/internal/logging"
	"github.com/centrifugal/centrifugo/v5/internal/natsbroker"
	"github.com/centrifugal/centrifugo/v5/internal/notify"
	"github.com/centrifugal/centrifugo/v5/internal/redisnatsbroker"
	"github.com/centrifugal/centrifugo/v5/internal/service"
	"github.com/centrifugal/centrifugo/v5/internal/survey"
	"github.com/centrifugal/centrifugo/v5/internal/telemetry"
	"github.com/centrifugal/centrifugo/v5/internal/tools"
	"github.com/centrifugal/centrifugo/v5/internal/usage"

	"github.com/centrifugal/centrifuge"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"go.uber.org/automaxprocs/maxprocs"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"

	_ "google.golang.org/grpc/encoding/gzip"
)

const edition = "oss"

func Run(cmd *cobra.Command, configFile string) {
	cfg, cfgMeta, err := config.GetConfig(cmd, configFile)
	if err != nil {
		log.Fatal().Msgf("error getting config: %v", err)
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

	nodeCfg := centrifugeNodeConfig(build.Version, cfg)

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
		natsBroker, err = NatsBroker(node, cfg)
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
		broker, err = NatsBroker(node, cfg)
		if err != nil {
			log.Fatal().Msgf("error creating broker: %v", err)
		}
		node.SetBroker(broker)
	}

	verifierConfig, err := JWTVerifierConfig(cfg)
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
		subVerifier, err := SubJWTVerifierConfig(cfg)
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

	var grpcAPIServer *grpc.Server
	if cfg.GrpcAPI.Enabled {
		var err error
		grpcAPIServer, err = runGRPCAPIServer(cfg, node, useAPIOpentelemetry, grpcAPIExecutor)
		if err != nil {
			log.Fatal().Msgf("error creating GRPC API server: %v", err)
		}
	}

	var grpcUniServer *grpc.Server
	if cfg.UniGRPC.Enabled {
		var err error
		grpcAPIServer, err = runGRPCUniServer(cfg, node)
		if err != nil {
			log.Fatal().Msgf("error creating GRPC API server: %v", err)
		}
	}

	httpServers, err := runHTTPServers(node, cfgContainer, httpAPIExecutor, keepHeadersInContext)
	if err != nil {
		log.Fatal().Msgf("error running HTTP server: %v", err)
	}

	if cfg.Graphite.Enabled {
		services = append(services, graphiteExporter(cfg, nodeCfg))
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

	logStartWarnings(cfg, cfgMeta)
	handleSignals(
		cmd, configFile, node, cfgContainer, tokenVerifier, subTokenVerifier,
		httpServers, grpcAPIServer, grpcUniServer,
		serviceGroup, serviceCancel,
	)
}

func handleSignals(
	cmd *cobra.Command, configFile string, n *centrifuge.Node, cfgContainer *config.Container,
	tokenVerifier *jwtverify.VerifierJWT, subTokenVerifier *jwtverify.VerifierJWT, httpServers []*http.Server,
	grpcAPIServer *grpc.Server, grpcUniServer *grpc.Server, serviceGroup *errgroup.Group,
	serviceCancel context.CancelFunc,
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
				log.Err(err).Msg("error reading config")
				continue
			}
			if err = newCfg.Validate(); err != nil {
				log.Error().Msgf("error validating config: %v", err)
				continue
			}
			verifierConfig, err := JWTVerifierConfig(newCfg)
			if err != nil {
				log.Error().Msgf("error reloading: %v", err)
				continue
			}
			if err = tokenVerifier.Reload(verifierConfig); err != nil {
				log.Error().Msgf("error reloading: %v", err)
				continue
			}
			if subTokenVerifier != nil {
				subVerifierConfig, err := SubJWTVerifierConfig(newCfg)
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
