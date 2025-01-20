package app

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/api"
	"github.com/centrifugal/centrifugo/v6/internal/build"
	"github.com/centrifugal/centrifugo/v6/internal/client"
	"github.com/centrifugal/centrifugo/v6/internal/config"
	"github.com/centrifugal/centrifugo/v6/internal/confighelpers"
	"github.com/centrifugal/centrifugo/v6/internal/consuming"
	"github.com/centrifugal/centrifugo/v6/internal/jwtverify"
	"github.com/centrifugal/centrifugo/v6/internal/logging"
	"github.com/centrifugal/centrifugo/v6/internal/notify"
	"github.com/centrifugal/centrifugo/v6/internal/service"
	"github.com/centrifugal/centrifugo/v6/internal/survey"
	"github.com/centrifugal/centrifugo/v6/internal/telemetry"
	"github.com/centrifugal/centrifugo/v6/internal/tools"
	"github.com/centrifugal/centrifugo/v6/internal/usage"

	"github.com/centrifugal/centrifuge"
	"github.com/joho/godotenv"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"go.uber.org/automaxprocs/maxprocs"
	"google.golang.org/grpc"

	_ "google.golang.org/grpc/encoding/gzip"
)

const edition = "oss"

func Run(cmd *cobra.Command, configFile string) {
	dotEnvUsed := false
	if tools.FileExists(".env") {
		err := godotenv.Load()
		if err != nil {
			log.Fatal().Err(err).Msg("error loading .env file")
		}
		dotEnvUsed = true
	}
	cfg, cfgMeta, err := config.GetConfig(cmd, configFile)
	if err != nil {
		log.Fatal().Err(err).Msg("error getting config")
	}

	ctx, serviceCancel := context.WithCancel(context.Background())
	defer serviceCancel()

	centrifugeLogHandler, logCloseFn := logging.Setup(cfg)
	if logCloseFn != nil {
		defer logCloseFn()
	}
	if cfgMeta.FileNotFound {
		log.Warn().Msg("config file not found, continue using environment and flag options")
	} else {
		absConfPath, _ := filepath.Abs(configFile)
		log.Info().Str("path", absConfPath).Msg("using config file")
		if dotEnvUsed {
			log.Info().Msg("environment variables have been loaded from .env file")
		}
	}
	err = tools.WritePidFile(cfg.PidFile)
	if err != nil {
		log.Fatal().Err(err).Msg("error writing PID")
	}
	_, _ = maxprocs.Set(maxprocs.Logger(func(s string, i ...interface{}) {
		log.Info().Msgf(strings.ToLower(s), i...)
	}))

	// Registered services will be run after node.Run() but before HTTP/GRPC servers start.
	// Registered services will be stopped after node's shutdown and HTTP/GRPC servers shutdown.
	serviceManager := service.NewManager()

	entry := log.Info().
		Str("version", build.Version).
		Str("runtime", runtime.Version()).
		Int("pid", os.Getpid()).
		Int("gomaxprocs", runtime.GOMAXPROCS(0))

	if cfg.Broker.Enabled {
		entry = entry.Str("broker", cfg.Broker.Type)
	}
	if cfg.PresenceManager.Enabled {
		entry = entry.Str("presence_manager", cfg.PresenceManager.Type)
	}
	if !cfg.Broker.Enabled || !cfg.PresenceManager.Enabled {
		entry = entry.Str("engine", cfg.Engine.Type)
	}
	entry.Msg("starting Centrifugo")

	if build.Version == "0.0.0" {
		log.Warn().Msg("running a development build of Centrifugo (version 0.0.0), ensure to use release build in production")
	}

	err = cfg.Validate()
	if err != nil {
		log.Fatal().Err(err).Msg("error validating config")
	}
	cfgContainer, err := config.NewContainer(cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("error creating config")
	}
	cfgContainer.ChannelOptionsCacheTTL = 200 * time.Millisecond

	proxyMap, keepHeadersInContext, err := buildProxyMap(cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("error building proxy map")
	}

	nodeCfg := centrifugeNodeConfig(build.Version, edition, cfgContainer, centrifugeLogHandler)
	node, err := centrifuge.New(nodeCfg)
	if err != nil {
		log.Fatal().Err(err).Msg("error creating Centrifuge Node")
	}

	if cfg.OpenTelemetry.Enabled {
		_, err := telemetry.SetupTracing(context.Background())
		if err != nil {
			log.Fatal().Err(err).Msg("error setting up opentelemetry tracing")
		}
	}

	modes, err := configureEngines(node, cfgContainer)
	if err != nil {
		log.Fatal().Err(err).Msg("configure engines error")
	}

	verifierConfig, err := confighelpers.MakeVerifierConfig(cfg.Client.Token)
	if err != nil {
		log.Fatal().Err(err).Msg("error creating JWT verifier config")
	}

	tokenVerifier, err := jwtverify.NewTokenVerifierJWT(verifierConfig, cfgContainer)
	if err != nil {
		log.Fatal().Err(err).Msg("error creating token verifier")
	}

	var subTokenVerifier *jwtverify.VerifierJWT
	if cfg.Client.SubscriptionToken.Enabled {
		log.Info().Msg("initializing separate verifier for subscription tokens")
		subVerifier, err := confighelpers.MakeVerifierConfig(cfg.Client.SubscriptionToken.Token)
		if err != nil {
			log.Fatal().Err(err).Msg("error creating subscription JWT verifier config")
		}
		subTokenVerifier, err = jwtverify.NewTokenVerifierJWT(subVerifier, cfgContainer)
		if err != nil {
			log.Fatal().Err(err).Msg("error creating token verifier")
		}
	}

	clientHandler := client.NewHandler(node, cfgContainer, tokenVerifier, subTokenVerifier, proxyMap)
	err = clientHandler.Setup()
	if err != nil {
		log.Fatal().Err(err).Msg("error setting up client handler")
	}
	if cfg.RPC.Ping.Enabled {
		log.Info().Str("method", cfg.RPC.Ping.Method).Msg("RPC ping extension enabled")
		clientHandler.SetRPCExtension(cfg.RPC.Ping.Method, func(c client.Client, e centrifuge.RPCEvent) (centrifuge.RPCReply, error) {
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

	consumingHandler := api.NewConsumingHandler(node, consumingAPIExecutor, api.ConsumingHandlerConfig{
		UseOpenTelemetry: useConsumingOpentelemetry,
	})

	consumingServices, err := consuming.New(node.ID(), consumingHandler, cfg.Consumers)
	if err != nil {
		log.Fatal().Err(err).Msg("error initializing consumers")
	}

	serviceManager.Register(consumingServices...)

	if cfg.Graphite.Enabled {
		serviceManager.Register(graphiteExporter(cfg, nodeCfg))
	}

	var statsSender *usage.Sender
	if !cfg.UsageStats.Disabled {
		statsSender = usage.NewSender(node, cfgContainer, usage.Features{
			Edition:                edition,
			Version:                build.Version,
			EngineEnabled:          !cfg.Broker.Enabled || !cfg.PresenceManager.Enabled,
			EngineType:             cfg.Engine.Type,
			EngineMode:             modes.engineMode,
			BrokerEnabled:          cfg.Broker.Enabled,
			BrokerType:             cfg.Broker.Type,
			BrokerMode:             modes.brokerMode,
			PresenceManagerEnabled: cfg.PresenceManager.Enabled,
			PresenceManagerType:    cfg.PresenceManager.Type,
			PresenceManagerMode:    modes.presenceManagerMode,

			Websocket:     !cfg.WebSocket.Disabled,
			HTTPStream:    cfg.HTTPStream.Enabled,
			SSE:           cfg.SSE.Enabled,
			UniWebsocket:  cfg.UniWS.Enabled,
			UniHTTPStream: cfg.UniHTTPStream.Enabled,
			UniSSE:        cfg.UniSSE.Enabled,
			UniGRPC:       cfg.UniGRPC.Enabled,

			EnabledConsumers: usage.GetEnabledConsumers(cfg.Consumers),

			GrpcAPI:             cfg.GrpcAPI.Enabled,
			SubscribeToPersonal: cfg.Client.SubscribeToUserPersonalChannel.Enabled,
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
		serviceManager.Register(statsSender)
	}

	notify.RegisterHandlers(node, statsSender)

	if err = node.Run(); err != nil {
		log.Fatal().Err(err).Msg("error running node")
	}

	serviceManager.Run(ctx)

	var grpcAPIServer *grpc.Server
	if cfg.GrpcAPI.Enabled {
		var err error
		grpcAPIServer, err = runGRPCAPIServer(cfg, node, useAPIOpentelemetry, grpcAPIExecutor)
		if err != nil {
			log.Fatal().Err(err).Msg("error creating GRPC API server")
		}
	}

	var grpcUniServer *grpc.Server
	if cfg.UniGRPC.Enabled {
		var err error
		grpcAPIServer, err = runGRPCUniServer(cfg, node)
		if err != nil {
			log.Fatal().Err(err).Msg("error creating GRPC API server")
		}
	}

	httpServers, err := runHTTPServers(node, cfgContainer, httpAPIExecutor, keepHeadersInContext)
	if err != nil {
		log.Fatal().Err(err).Msg("error running HTTP server")
	}

	logStartWarnings(cfg, cfgMeta)

	handleSignals(
		cmd, configFile, node, cfgContainer, tokenVerifier, subTokenVerifier,
		httpServers, grpcAPIServer, grpcUniServer,
		serviceManager, serviceCancel,
	)
}

func handleSignals(
	cmd *cobra.Command, configFile string, n *centrifuge.Node, cfgContainer *config.Container,
	tokenVerifier *jwtverify.VerifierJWT, subTokenVerifier *jwtverify.VerifierJWT, httpServers []*http.Server,
	grpcAPIServer *grpc.Server, grpcUniServer *grpc.Server, serviceManager *service.Manager,
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
			verifierConfig, err := confighelpers.MakeVerifierConfig(newCfg.Client.Token)
			if err != nil {
				log.Error().Msgf("error reloading: %v", err)
				continue
			}
			if err = tokenVerifier.Reload(verifierConfig); err != nil {
				log.Error().Msgf("error reloading: %v", err)
				continue
			}
			if subTokenVerifier != nil {
				subVerifierConfig, err := confighelpers.MakeVerifierConfig(newCfg.Client.SubscriptionToken.Token)
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
			go time.AfterFunc(shutdownTimeout.ToDuration(), func() {
				if pidFile != "" {
					_ = os.Remove(pidFile)
				}
				log.Fatal().Msg("shutdown timeout reached")
			})

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

			for _, srv := range httpServers {
				wg.Add(1)
				go func(srv *http.Server) {
					defer wg.Done()
					_ = srv.Shutdown(context.Background()) // We have a separate timeout goroutine.
				}(srv)
			}

			_ = n.Shutdown(context.Background()) // We have a separate timeout goroutine.
			wg.Wait()

			serviceCancel()
			_ = serviceManager.Wait()

			if pidFile != "" {
				_ = os.Remove(pidFile)
			}
			os.Exit(0)
		}
	}
}
