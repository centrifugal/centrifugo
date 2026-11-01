package app

import (
	"context"
	"crypto/fips140"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/centrifugal/centrifugo/v6/internal/api"
	"github.com/centrifugal/centrifugo/v6/internal/build"
	"github.com/centrifugal/centrifugo/v6/internal/client"
	"github.com/centrifugal/centrifugo/v6/internal/config"
	"github.com/centrifugal/centrifugo/v6/internal/confighelpers"
	"github.com/centrifugal/centrifugo/v6/internal/consuming"
	"github.com/centrifugal/centrifugo/v6/internal/jwtverify"
	"github.com/centrifugal/centrifugo/v6/internal/logging"
	"github.com/centrifugal/centrifugo/v6/internal/metrics"
	"github.com/centrifugal/centrifugo/v6/internal/notify"
	"github.com/centrifugal/centrifugo/v6/internal/service"
	"github.com/centrifugal/centrifugo/v6/internal/survey"
	"github.com/centrifugal/centrifugo/v6/internal/telemetry"
	"github.com/centrifugal/centrifugo/v6/internal/tools"
	"github.com/centrifugal/centrifugo/v6/internal/usage"

	"github.com/centrifugal/centrifuge"
	"github.com/joho/godotenv"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"go.uber.org/automaxprocs/maxprocs"

	_ "google.golang.org/grpc/encoding/gzip"
)

const edition = "oss"

// Options configure the App.
type Options struct {
	// Extensions are applied in order.
	Extensions []Extension
}

// App is a Centrifugo server: New builds it, Start runs it, Wait waits for a
// reason to stop, Shutdown stops it and Close releases what remains.
type App struct {
	cmd        *cobra.Command
	configFile string
	extensions []Extension
	lifecycle  *Lifecycle

	cfg        config.Config
	cfgMeta    config.Meta
	logHandler centrifuge.LogHandler
	logClosers []func()

	container            *config.Container
	proxyMap             *client.ProxyMap
	keepHeadersInContext bool
	nodeCfg              centrifuge.Config
	node                 *centrifuge.Node
	tokenVerifier        *jwtverify.VerifierJWT
	subTokenVerifier     *jwtverify.VerifierJWT
	clientHandler        *client.Handler
	survey               *survey.Caller
	httpAPIExecutor      *api.Executor
	grpcAPIExecutor      *api.Executor
	consumingAPIExecutor *api.Executor
	components           *Components

	// Registered services will be run after node.Run() but before HTTP/GRPC servers start.
	// Registered services will be stopped after node's shutdown and HTTP/GRPC servers shutdown.
	services      *service.Manager
	serviceCtx    context.Context
	serviceCancel context.CancelFunc
	serviceDone   chan struct{}
	// Ingest services (async consumers) have a separate lifecycle from the rest of
	// services – see the shutdown sequence in Shutdown.
	ingest       *service.Manager
	ingestCtx    context.Context
	ingestCancel context.CancelFunc
	ingestDone   chan struct{}
	consumers    []consuming.Consumer

	// metricsRegistry holds the metrics of this App: /metrics, the Graphite
	// exporter and the node gather from it.
	metricsRegistry *prometheus.Registry

	muxCtx *MuxContext

	// startMu is held by Start: Shutdown waits for a Start in progress.
	startMu       sync.Mutex
	nodeRunning   bool
	httpServers   []*httpServer
	acmeServer    *acmeServer
	serveWG       *sync.WaitGroup
	grpcAPIServer *grpcServer
	grpcUniServer *grpcServer
	sigCh         chan os.Signal
	ownSignals    bool // sigCh was created by Start
	mu            sync.Mutex
	addrs         []net.Addr

	shutdownOnce   sync.Once
	shutdownStep   atomic.Value // name of the shutdown step running
	shutdownReason ExitReason
	shutdownCode   int
	shutdownErr    error
	closeOnce      sync.Once
	timedOut       atomic.Bool // a shutdown step didn't finish in time
	stopRequested  atomic.Bool
	// stopWatchDone ends the watching of a restart's shutdown for SIGINT and
	// SIGTERM; stopWatchExited is closed once it ended.
	stopWatchDone   chan struct{}
	stopWatchExited chan struct{}
	stopWatchEnded  bool
	stopWatchOnce   sync.Once
}

// processInit guards initialisation done once per process: maxprocs, which
// doesn't depend on the config. A second App in the process reuses it.
var processInit struct {
	maxprocs sync.Once
}

// loggedError is an error New or Start already logged.
type loggedError struct {
	err error
}

func (e *loggedError) Error() string { return e.err.Error() }

func (e *loggedError) Unwrap() error { return e.err }

// fatal logs err with msg at fatal level, without exiting, and returns it
// marked as logged. An error already logged is returned as is.
func (a *App) fatal(err error, msg string) error {
	var logged *loggedError
	var exitErr *ExitError
	if errors.As(err, &logged) || errors.As(err, &exitErr) {
		return err
	}
	return logFatal(err, msg)
}

// logFatal logs err with msg at fatal level, without exiting, and returns it
// marked as logged.
func logFatal(err error, msg string) error {
	event := log.WithLevel(zerolog.FatalLevel)
	if err != nil {
		event = event.Err(err)
	}
	event.Msg(msg)
	if err == nil {
		return &loggedError{err: errors.New(msg)}
	}
	return &loggedError{err: fmt.Errorf("%s: %w", msg, err)}
}

// serveError handles an error of a server after Start began serving: it is
// logged with msg at fatal level and the App is asked to exit with code 1.
// Once shutdown began it is only logged.
func (a *App) serveError(err error, msg string) {
	if a.lifecycle.isStopping() {
		log.Error().Err(err).Msg(msg)
		return
	}
	_ = logFatal(err, msg)
	a.lifecycle.Exit(1, err)
}

// exitRequested logs an exit asked for with Lifecycle.Exit during New or Start.
func (a *App) exitRequested(exitErr *ExitError) error {
	logExitRequested(exitErr)
	return exitErr
}

func logExitRequested(exitErr *ExitError) {
	event := log.Info()
	if exitErr.Err != nil {
		event = log.Error().Err(exitErr.Err)
	}
	event.Int("code", exitErr.Code).Msg("exit requested")
}

// eachExtension calls f for every extension implementing T, in order, and
// stops at the first error.
func eachExtension[T any](extensions []Extension, f func(name string, e T) error) error {
	for _, ext := range extensions {
		if e, ok := ext.(T); ok {
			if err := f(ext.Name(), e); err != nil {
				return err
			}
		}
	}
	return nil
}

func wrapExtensionError(name string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("extension %s: %w", name, err)
}

// New builds an App from the config file and command flags. An error is
// logged before New returns it, and what New created is cleaned up.
func New(cmd *cobra.Command, configFile string, opts Options) (*App, error) {
	a := &App{
		cmd:        cmd,
		configFile: configFile,
		extensions: opts.Extensions,
		lifecycle:  newLifecycle(),
	}
	a.serviceCtx, a.serviceCancel = context.WithCancel(context.Background())
	a.ingestCtx, a.ingestCancel = context.WithCancel(context.Background())

	phases := []func() error{
		a.setupConfigAndLogging, // 1, 2
		a.setupContainer,        // 3
		a.setupProxies,          // 4
		a.setupNode,             // 5
		a.setupEngines,          // 6
		a.setupHandlers,         // 7
		a.setupServices,         // 8
	}
	for _, phase := range phases {
		err := phase()
		if err == nil {
			if exitErr := a.lifecycle.exited(); exitErr != nil {
				err = a.exitRequested(exitErr)
			}
		}
		if err != nil {
			a.cleanupFailedNew()
			return nil, err
		}
	}
	return a, nil
}

// cleanupFailedNew runs the stop steps registered so far, then the close
// tier. The error of New is already logged.
func (a *App) cleanupFailedNew() {
	a.lifecycle.quiesce()
	a.serviceCancel()
	a.ingestCancel()
	s := newShutdownRun(a.shutdownTimeout(), &a.shutdownStep)
	defer s.cancel()
	_ = s.step("producers", a.lifecycle.runningProducers, func(context.Context) error {
		a.lifecycle.wg.Wait()
		return nil
	})
	var errs []error
	steps := a.lifecycle.takeStopSteps()
	for i := len(steps) - 1; i >= 0; i-- {
		if err := s.step(steps[i].name, nil, steps[i].stop); err != nil {
			errs = append(errs, err)
		}
	}
	if err := errors.Join(errs...); err != nil {
		log.Error().Err(err).Msg("error shutting down")
	}
	a.closeTier()
}

// loadDotEnv loads .env from the working directory when it exists. Variables
// already set are not overridden, so loading it twice changes nothing. It
// reports whether the file was loaded.
func loadDotEnv() (bool, error) {
	if !tools.FileExists(".env") {
		return false, nil
	}
	if err := godotenv.Load(); err != nil {
		return false, err
	}
	return true, nil
}

// setupConfigAndLogging loads .env and config (phase 1), then sets up logging,
// the pid file and maxprocs and logs the banner (phase 2).
func (a *App) setupConfigAndLogging() error {
	dotEnvUsed, err := loadDotEnv()
	if err != nil {
		return a.fatal(err, "error loading .env file")
	}
	cfg, cfgMeta, err := config.GetConfig(a.cmd, a.configFile)
	if err != nil {
		return a.fatal(err, "error getting config")
	}
	a.cfg = cfg
	a.cfgMeta = cfgMeta

	var writers []io.Writer
	err = eachExtension(a.extensions, func(name string, e LogWriterProvider) error {
		w, closer, err := e.LogWriters(cfg)
		if err != nil {
			return wrapExtensionError(name, err)
		}
		writers = append(writers, w...)
		if closer != nil {
			a.logClosers = append(a.logClosers, closer)
		}
		return nil
	})
	if err != nil {
		return a.fatal(err, "error setting up log writers")
	}

	logHandler, logCloseFn, err := logging.Setup(cfg, logging.Options{Writers: writers})
	if err != nil {
		return a.fatal(err, "error opening log file")
	}
	a.logHandler = logHandler
	if logCloseFn != nil {
		a.logClosers = append([]func(){logCloseFn}, a.logClosers...)
	}
	if cfgMeta.FileNotFound {
		log.Warn().Msg("config file not found, continue using environment and flag options")
	} else {
		absConfPath, _ := filepath.Abs(a.configFile)
		log.Info().Str("path", absConfPath).Msg("using config file")
		if dotEnvUsed {
			log.Info().Msg("environment variables have been loaded from .env file")
		}
	}
	err = tools.WritePidFile(cfg.PidFile)
	if err != nil {
		return a.fatal(err, "error writing PID")
	}
	if cfg.PidFile != "" {
		a.lifecycle.OnStop("pid file", func(context.Context) error {
			a.mu.Lock()
			restart := a.shutdownReason == ExitRestart
			a.mu.Unlock()
			if !restart { // Kept across a restart: the PID stays the same.
				_ = os.Remove(cfg.PidFile)
			}
			return nil
		})
	}
	processInit.maxprocs.Do(func() {
		_, _ = maxprocs.Set(maxprocs.Logger(func(s string, i ...interface{}) {
			log.Info().Msgf(strings.ToLower(s), i...)
		}))
	})

	a.services = service.NewManager()

	entry := log.Info().
		Str("version", build.Version).
		Str("runtime", runtime.Version()).
		Int("pid", os.Getpid()).
		Int("gomaxprocs", runtime.GOMAXPROCS(0))

	if fips140.Enabled() { // See https://go.dev/doc/security/fips140.
		entry = entry.Bool("fips", true)
	}

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
	return nil
}

// setupContainer validates config and creates the config container and
// metrics (phase 3).
func (a *App) setupContainer() error {
	cfg := a.cfg
	err := cfg.Validate()
	if err != nil {
		return a.fatal(err, "error validating config")
	}
	a.container, err = config.NewContainer(cfg)
	if err != nil {
		return a.fatal(err, "error creating config")
	}

	// Each App has its own metrics registry, so that its metrics follow its
	// config.
	a.metricsRegistry = newMetricsRegistry()
	metricsConfig := metrics.Config{
		Namespace:        "",  // Use default "centrifugo" namespace.
		ConstLabels:      nil, // Can be populated from config in the future.
		Registerer:       a.metricsRegistry,
		NativeHistograms: cfg.Prometheus.NativeHistograms,
	}
	if err := metrics.Init(metricsConfig); err != nil {
		return a.fatal(err, "error initializing metrics")
	}
	return nil
}

// newMetricsRegistry returns a registry with the Go runtime and process
// collectors, as the default Prometheus registry has.
func newMetricsRegistry() *prometheus.Registry {
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		collectors.NewGoCollector(),
	)
	return reg
}

// setupProxies builds the proxy map (phase 4).
func (a *App) setupProxies() error {
	var err error
	a.proxyMap, a.keepHeadersInContext, err = buildProxyMap(a.cfg)
	if err != nil {
		return a.fatal(err, "error building proxy map")
	}
	proxyMap := a.proxyMap
	a.lifecycle.OnStop("proxies", func(context.Context) error {
		return proxyMap.Close()
	})
	return nil
}

// setupNode creates the node and sets up tracing (phase 5).
func (a *App) setupNode() error {
	cfg := a.cfg
	a.nodeCfg = a.centrifugeNodeConfig()
	var err error
	a.node, err = centrifuge.New(a.nodeCfg)
	if err != nil {
		return a.fatal(err, "error creating Centrifuge Node")
	}

	if cfg.OpenTelemetry.Enabled {
		provider, err := telemetry.SetupTracing(context.Background(), cfg.OpenTelemetry, a.node.ID())
		if err != nil {
			return a.fatal(err, "error setting up opentelemetry tracing")
		}
		a.lifecycle.OnStop("tracing", func(ctx context.Context) error {
			return telemetry.ShutdownTracing(ctx, provider)
		})
	}
	return nil
}

// setupEngines creates engines and sets them on the node (phase 6).
func (a *App) setupEngines() error {
	err := a.configureEngines()
	if err != nil {
		return a.fatal(err, "configure engines error")
	}
	err = a.configureMapBroker()
	if err != nil {
		return a.fatal(err, "configure map broker error")
	}
	return nil
}

// setupHandlers creates token verifiers, the client handler, the survey
// caller and API executors, calls Setupper extensions and registers the
// client handler on the node (phase 7).
func (a *App) setupHandlers() error {
	cfg := a.cfg
	node := a.node
	cfgContainer := a.container

	verifierConfig, err := confighelpers.MakeVerifierConfig(cfg.Client.Token)
	if err != nil {
		return a.fatal(err, "error creating JWT verifier config")
	}

	a.tokenVerifier, err = jwtverify.NewTokenVerifierJWT(verifierConfig, cfgContainer)
	if err != nil {
		return a.fatal(err, "error creating token verifier")
	}
	tokenVerifier := a.tokenVerifier
	a.lifecycle.OnStop("token verifier", func(context.Context) error {
		return tokenVerifier.Close()
	})

	if cfg.Client.SubscriptionToken.Enabled {
		log.Info().Msg("initializing separate verifier for subscription tokens")
		subVerifier, err := confighelpers.MakeVerifierConfig(cfg.Client.SubscriptionToken.Token)
		if err != nil {
			return a.fatal(err, "error creating subscription JWT verifier config")
		}
		a.subTokenVerifier, err = jwtverify.NewTokenVerifierJWT(subVerifier, cfgContainer)
		if err != nil {
			return a.fatal(err, "error creating token verifier")
		}
		subTokenVerifier := a.subTokenVerifier
		a.lifecycle.OnStop("subscription token verifier", func(context.Context) error {
			return subTokenVerifier.Close()
		})
	}

	a.clientHandler = client.NewHandler(node, cfgContainer, a.tokenVerifier, a.subTokenVerifier, a.proxyMap)

	a.survey = survey.NewCaller(node)

	useAPIOpentelemetry := cfg.OpenTelemetry.Enabled && cfg.OpenTelemetry.API
	useConsumingOpentelemetry := cfg.OpenTelemetry.Enabled && cfg.OpenTelemetry.Consuming

	a.httpAPIExecutor = api.NewExecutor(node, cfgContainer, a.survey, api.ExecutorConfig{
		Protocol:         "http",
		UseOpenTelemetry: useAPIOpentelemetry,
	})
	a.grpcAPIExecutor = api.NewExecutor(node, cfgContainer, a.survey, api.ExecutorConfig{
		Protocol:         "grpc",
		UseOpenTelemetry: useAPIOpentelemetry,
	})
	a.consumingAPIExecutor = api.NewExecutor(node, cfgContainer, a.survey, api.ExecutorConfig{
		Protocol:         "consuming",
		UseOpenTelemetry: useConsumingOpentelemetry,
	})

	a.components = &Components{
		ConfigFile:        a.configFile,
		Container:         cfgContainer,
		Node:              node,
		ClientHandler:     a.clientHandler,
		HTTPExecutor:      a.httpAPIExecutor,
		GRPCExecutor:      a.grpcAPIExecutor,
		ConsumingExecutor: a.consumingAPIExecutor,
		app:               a,
	}
	err = eachExtension(a.extensions, func(name string, e Setupper) error {
		return wrapExtensionError(name, e.Setup(a.components))
	})
	if err != nil {
		return a.fatal(err, "error setting up extensions")
	}

	err = a.clientHandler.Setup()
	if err != nil {
		return a.fatal(err, "error setting up client handler")
	}
	if cfg.RPC.Ping.Enabled {
		log.Info().Str("method", cfg.RPC.Ping.Method).Msg("RPC ping extension enabled")
		a.clientHandler.SetRPCExtension(cfg.RPC.Ping.Method, func(c client.Client, e centrifuge.RPCEvent) (centrifuge.RPCReply, error) {
			return centrifuge.RPCReply{}, nil
		})
	}
	return nil
}

// setupServices creates consumers, graphite exporter and usage sender, and
// registers notification handlers (phase 8).
func (a *App) setupServices() error {
	cfg := a.cfg
	node := a.node
	proxyMap := a.proxyMap

	useConsumingOpentelemetry := cfg.OpenTelemetry.Enabled && cfg.OpenTelemetry.Consuming
	consumingHandler := api.NewConsumingHandler(node, a.consumingAPIExecutor, api.ConsumingHandlerConfig{
		UseOpenTelemetry: useConsumingOpentelemetry,
	})

	consumers, err := consuming.New(node.ID(), consumingHandler, cfg.Consumers)
	if err != nil {
		return a.fatal(err, "error initializing consumers")
	}
	a.consumers = consumers
	// Consumers are drained in Shutdown; this step closes them when Start
	// never ran.
	a.lifecycle.OnStop("consumers", func(ctx context.Context) error {
		var errs []error
		for _, c := range a.consumers {
			if err := c.Close(ctx); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
				errs = append(errs, err)
			}
		}
		return errors.Join(errs...)
	})

	// Consumers are registered in a separate manager: unlike other services they
	// must be stopped before Node shutdown, so that no consumed message is
	// acknowledged after this node lost the ability to deliver it.
	a.ingest = service.NewManager()
	for _, c := range a.consumers {
		a.ingest.Register(c)
	}

	if cfg.Graphite.Enabled {
		exporter := graphiteExporter(cfg, a.nodeCfg, a.metricsRegistry)
		a.lifecycle.OnStop("graphite", func(context.Context) error {
			return exporter.Close()
		})
		a.services.Register(exporter)
	}

	var statsSender *usage.Sender
	if !cfg.UsageStats.Disabled {
		statsSender = usage.NewSender(node, a.container, usage.Features{
			Edition:                edition,
			Version:                build.Version,
			EngineEnabled:          !cfg.Broker.Enabled || !cfg.PresenceManager.Enabled,
			EngineType:             cfg.Engine.Type,
			BrokerEnabled:          cfg.Broker.Enabled,
			BrokerType:             cfg.Broker.Type,
			PresenceManagerEnabled: cfg.PresenceManager.Enabled,
			PresenceManagerType:    cfg.PresenceManager.Type,

			Websocket:     !cfg.WebSocket.Disabled,
			HTTPStream:    cfg.HTTPStream.Enabled,
			SSE:           cfg.SSE.Enabled,
			UniWebsocket:  cfg.UniWS.Enabled,
			UniHTTPStream: cfg.UniHTTPStream.Enabled,
			UniSSE:        cfg.UniSSE.Enabled,
			UniGRPC:       cfg.UniGRPC.Enabled,
			WebTransport:  cfg.WebTransport.Enabled,

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
			Swagger:              cfg.Swagger.Enabled,
		})
		a.services.Register(statsSender)
	}

	notify.RegisterHandlers(node, statsSender)
	return nil
}
