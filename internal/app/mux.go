package app

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	stdlog "log"
	"net"
	"net/http"
	"net/http/pprof"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/admin"
	"github.com/centrifugal/centrifugo/v6/internal/api"
	"github.com/centrifugal/centrifugo/v6/internal/config"
	"github.com/centrifugal/centrifugo/v6/internal/conninit"
	"github.com/centrifugal/centrifugo/v6/internal/health"
	"github.com/centrifugal/centrifugo/v6/internal/middleware"
	"github.com/centrifugal/centrifugo/v6/internal/swaggerui"
	"github.com/centrifugal/centrifugo/v6/internal/tools"
	"github.com/centrifugal/centrifugo/v6/internal/unihttpstream"
	"github.com/centrifugal/centrifugo/v6/internal/unisse"
	"github.com/centrifugal/centrifugo/v6/internal/uniws"
	"github.com/centrifugal/centrifugo/v6/internal/wt"

	"github.com/centrifugal/centrifuge"
	"github.com/justinas/alice"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/quic-go/quic-go/http3"
	"github.com/quic-go/webtransport-go"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

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
	// HandlerInit handles connection initialization.
	HandlerInit
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
	HandlerInit:          "init",
	HandlerSwagger:       "swagger",
}

func (flags HandlerFlag) String() string {
	flagsOrdered := []HandlerFlag{HandlerWebsocket, HandlerWebtransport, HandlerHTTPStream, HandlerSSE, HandlerEmulation, HandlerAPI, HandlerAdmin, HandlerPrometheus, HandlerDebug, HandlerHealth, HandlerUniWebsocket, HandlerUniSSE, HandlerUniHTTPStream, HandlerSwagger, HandlerInit}
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

// Mux returns a mux including set of default handlers for Centrifugo server
// and the routes given, and the names of the routes it registered.
func (a *App) Mux(flags HandlerFlag, routes []Route, wtServer *webtransport.Server) (*http.ServeMux, []string, error) {
	n := a.node
	cfgContainer := a.container
	apiExecutor := a.httpAPIExecutor
	keepHeadersInContext := a.keepHeadersInContext

	mux := http.NewServeMux()
	cfg := cfgContainer.Config()

	// patterns registered on the mux, to detect conflicting routes.
	patterns := map[string]struct{}{}
	handle := func(pattern string, h http.Handler) {
		mux.Handle(pattern, h)
		patterns[pattern] = struct{}{}
	}

	var commonMiddlewares []alice.Constructor

	useLoggingMW := zerolog.GlobalLevel() <= zerolog.DebugLevel
	if useLoggingMW {
		commonMiddlewares = append(commonMiddlewares, middleware.LogRequest)
	}
	if cfg.Prometheus.Enabled && cfg.Prometheus.InstrumentHTTPHandlers {
		commonMiddlewares = append(commonMiddlewares, middleware.HTTPServerInstrumentation)
	}

	basicMiddlewares := append([]alice.Constructor{}, commonMiddlewares...)
	basicChain := alice.New(basicMiddlewares...)

	if flags&HandlerDebug != 0 {
		handle(cfg.Debug.HandlerPrefix+"/", basicChain.Then(http.HandlerFunc(pprof.Index)))
		handle(cfg.Debug.HandlerPrefix+"/cmdline", basicChain.Then(http.HandlerFunc(pprof.Cmdline)))
		handle(cfg.Debug.HandlerPrefix+"/profile", basicChain.Then(http.HandlerFunc(pprof.Profile)))
		handle(cfg.Debug.HandlerPrefix+"/symbol", basicChain.Then(http.HandlerFunc(pprof.Symbol)))
		handle(cfg.Debug.HandlerPrefix+"/trace", basicChain.Then(http.HandlerFunc(pprof.Trace)))
	}

	if flags&HandlerEmulation != 0 {
		// register bidirectional emulation endpoint.
		checkOrigin, err := getCheckOrigin(cfg)
		if err != nil {
			return nil, nil, err
		}
		emulationMiddlewares := append([]alice.Constructor{}, commonMiddlewares...)
		emulationMiddlewares = append(emulationMiddlewares, middleware.NewCORS(checkOrigin).Middleware)
		emulationChain := alice.New(emulationMiddlewares...)

		emulationPrefix := strings.TrimRight(cfg.Emulation.HandlerPrefix, "/")
		if emulationPrefix == "" {
			emulationPrefix = "/"
		}
		handle(emulationPrefix, emulationChain.Then(centrifuge.NewEmulationHandler(n, emulationHandlerConfig(cfg))))
	}

	connMiddlewares := append([]alice.Constructor{}, commonMiddlewares...)
	// The ConnLimit middleware enforces both the total connection cap and the
	// connection rate limit, so install it when either is configured - otherwise
	// connection_rate_limit is silently ignored when connection_limit is 0.
	if cfg.Client.ConnectionLimit > 0 || cfg.Client.ConnectionRateLimit > 0 {
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
	connCheckOrigin, err := getCheckOrigin(cfg)
	if err != nil {
		return nil, nil, err
	}
	connMiddlewares = append(connMiddlewares, middleware.NewCORS(connCheckOrigin).Middleware)
	connChain := alice.New(connMiddlewares...)

	if flags&HandlerWebsocket != 0 {
		// register WebSocket connection endpoint.
		wsPrefix := strings.TrimRight(cfg.WebSocket.HandlerPrefix, "/")
		if wsPrefix == "" {
			wsPrefix = "/"
		}
		wsConfig, err := websocketHandlerConfig(cfg)
		if err != nil {
			return nil, nil, err
		}
		handle(wsPrefix, connChain.Then(centrifuge.NewWebsocketHandler(n, wsConfig)))
	}

	if flags&HandlerWebtransport != 0 {
		// register WebTransport connection endpoint.
		wtPrefix := strings.TrimRight(cfg.WebTransport.HandlerPrefix, "/")
		if wtPrefix == "" {
			wtPrefix = "/"
		}
		pingPong, err := getPingPongConfig(cfg)
		if err != nil {
			return nil, nil, err
		}
		handle(wtPrefix, connChain.Then(wt.NewHandler(n, wtServer, cfg.WebTransport, pingPong)))
	}

	if flags&HandlerInit != 0 {
		// register connection initialization endpoint.
		connInitPrefix := strings.TrimRight(cfg.Init.HandlerPrefix, "/")
		if connInitPrefix == "" {
			connInitPrefix = "/"
		}
		handle(connInitPrefix, middleware.Method(http.MethodGet, connChain.Then(conninit.NewHandler())))
	}

	if flags&HandlerHTTPStream != 0 {
		// register bidirectional HTTP stream connection endpoint.
		streamPrefix := strings.TrimRight(cfg.HTTPStream.HandlerPrefix, "/")
		if streamPrefix == "" {
			streamPrefix = "/"
		}
		streamConfig, err := httpStreamHandlerConfig(cfg)
		if err != nil {
			return nil, nil, err
		}
		handle(streamPrefix, connChain.Then(centrifuge.NewHTTPStreamHandler(n, streamConfig)))
	}
	if flags&HandlerSSE != 0 {
		// register bidirectional SSE connection endpoint.
		ssePrefix := strings.TrimRight(cfg.SSE.HandlerPrefix, "/")
		if ssePrefix == "" {
			ssePrefix = "/"
		}
		sseConfig, err := sseHandlerConfig(cfg)
		if err != nil {
			return nil, nil, err
		}
		handle(ssePrefix, connChain.Then(centrifuge.NewSSEHandler(n, sseConfig)))
	}

	if flags&HandlerUniWebsocket != 0 {
		// register unidirectional WebSocket connection endpoint.
		wsPrefix := strings.TrimRight(cfg.UniWS.HandlerPrefix, "/")
		if wsPrefix == "" {
			wsPrefix = "/"
		}
		checkOrigin, err := getCheckOrigin(cfg)
		if err != nil {
			return nil, nil, err
		}
		pingPong, err := getPingPongConfig(cfg)
		if err != nil {
			return nil, nil, err
		}
		handle(wsPrefix, connChain.Then(
			uniws.NewHandler(n, cfg.UniWS, checkOrigin, pingPong)))
	}

	if flags&HandlerUniSSE != 0 {
		// register unidirectional SSE connection endpoint.
		ssePrefix := strings.TrimRight(cfg.UniSSE.HandlerPrefix, "/")
		if ssePrefix == "" {
			ssePrefix = "/"
		}
		pingPong, err := getPingPongConfig(cfg)
		if err != nil {
			return nil, nil, err
		}
		handle(ssePrefix, connChain.Then(unisse.NewHandler(n, cfg.UniSSE, pingPong)))
	}

	if flags&HandlerUniHTTPStream != 0 {
		// register unidirectional HTTP stream connection endpoint.
		streamPrefix := strings.TrimRight(cfg.UniHTTPStream.HandlerPrefix, "/")
		if streamPrefix == "" {
			streamPrefix = "/"
		}
		pingPong, err := getPingPongConfig(cfg)
		if err != nil {
			return nil, nil, err
		}
		handle(streamPrefix, connChain.Then(unihttpstream.NewHandler(n, cfg.UniHTTPStream, pingPong)))
	}

	if flags&HandlerAPI != 0 {
		// register HTTP API endpoints.
		httpErrorMode, err := tools.OptionalStringChoice(cfg.HttpAPI.ErrorMode, []string{config.TransportErrorMode})
		if err != nil {
			return nil, nil, logFatal(err, "error in config")
		}
		useOpenTelemetry := cfg.OpenTelemetry.Enabled && cfg.OpenTelemetry.API
		apiHandler := api.NewHandler(n, apiExecutor, api.Config{
			UseOpenTelemetry:      useOpenTelemetry,
			UseTransportErrorMode: httpErrorMode == config.TransportErrorMode,
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

		handle(apiPrefix, apiChain(apiPrefix).Then(apiHandler.OldRoute()))
		if apiPrefix != "/" {
			for path, handler := range apiHandler.Routes() {
				handlePath := apiPrefix + path
				handle(handlePath, apiChain(handlePath).Then(handler))
			}
		} else {
			for path, handler := range apiHandler.Routes() {
				handle(path, apiChain(path).Then(handler))
			}
		}
	}

	if flags&HandlerSwagger != 0 {
		// register Swagger UI endpoint.
		swaggerPrefix := strings.TrimRight(cfg.Swagger.HandlerPrefix, "/") + "/"
		if swaggerPrefix == "" {
			swaggerPrefix = "/"
		}
		handle(swaggerPrefix, basicChain.Then(http.StripPrefix(swaggerPrefix, http.FileServer(swaggerui.FS))))
	}

	if flags&HandlerPrometheus != 0 {
		// register Prometheus metrics export endpoint.
		prometheusPrefix := strings.TrimRight(cfg.Prometheus.HandlerPrefix, "/")
		if prometheusPrefix == "" {
			prometheusPrefix = "/"
		}
		metricsHandler := promhttp.InstrumentMetricHandler(a.metricsRegistry, promhttp.HandlerFor(a.metricsRegistry, promhttp.HandlerOpts{}))
		handle(prometheusPrefix, basicChain.Then(metricsHandler))
	}

	if flags&HandlerAdmin != 0 {
		adminPrefix := strings.TrimRight(cfg.Admin.HandlerPrefix, "/")
		adminHandler, err := admin.NewHandler(n, apiExecutor, cfg.Admin)
		if err != nil {
			return nil, nil, logFatal(err, "error creating admin web reverse proxy")
		}
		handle(adminPrefix+"/", basicChain.Then(adminHandler))
	}

	if flags&HandlerHealth != 0 {
		healthPrefix := strings.TrimRight(cfg.Health.HandlerPrefix, "/")
		if healthPrefix == "" {
			healthPrefix = "/"
		}
		handle(healthPrefix, basicChain.Then(health.NewHandler(n, health.Config{})))
	}

	chainHandler := func(r Route) http.Handler {
		switch r.Chain {
		case ChainNone:
			return r.Handler
		default:
			return basicChain.Then(r.Handler)
		}
	}
	var names []string
	for _, r := range routes {
		if r.IfAbsent {
			continue
		}
		if _, ok := patterns[r.Pattern]; ok {
			return nil, nil, fmt.Errorf("route %s: pattern %q is already registered", r.Name, r.Pattern)
		}
		if err := handleRecover(mux, r.Pattern, chainHandler(r)); err != nil {
			return nil, nil, fmt.Errorf("route %s: %w", r.Name, err)
		}
		patterns[r.Pattern] = struct{}{}
		names = append(names, r.Name)
	}
	for _, r := range routes {
		if !r.IfAbsent {
			continue
		}
		if _, ok := patterns[r.Pattern]; ok {
			continue
		}
		if err := handleRecover(mux, r.Pattern, chainHandler(r)); err != nil {
			return nil, nil, fmt.Errorf("route %s: %w", r.Name, err)
		}
		patterns[r.Pattern] = struct{}{}
		names = append(names, r.Name)
	}

	return mux, names, nil
}

// handleRecover registers h on mux, returning the panic of an invalid or
// conflicting pattern as an error.
func handleRecover(mux *http.ServeMux, pattern string, h http.Handler) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()
	mux.Handle(pattern, h)
	return nil
}

// getPingPongConfig returns the ping/pong config of connection endpoints. An
// error is logged before it is returned.
func getPingPongConfig(cfg config.Config) (centrifuge.PingPongConfig, error) {
	pingInterval := cfg.Client.PingInterval
	pongTimeout := cfg.Client.PongTimeout
	if pingInterval <= pongTimeout {
		const msg = "ping_interval must be greater than pong_timeout"
		log.WithLevel(zerolog.FatalLevel).Str("ping_interval", pingInterval.String()).Str("pong_timeout", pongTimeout.String()).
			Msg(msg)
		return centrifuge.PingPongConfig{}, &loggedError{err: errors.New(msg)}
	}
	return centrifuge.PingPongConfig{
		PingInterval: pingInterval.ToDuration(),
		PongTimeout:  pongTimeout.ToDuration(),
	}, nil
}

func websocketHandlerConfig(appCfg config.Config) (centrifuge.WebsocketConfig, error) {
	cfg := centrifuge.WebsocketConfig{}
	cfg.Compression = appCfg.WebSocket.Compression
	cfg.CompressionLevel = appCfg.WebSocket.CompressionLevel
	cfg.CompressionMinSize = appCfg.WebSocket.CompressionMinSize
	cfg.ReadBufferSize = appCfg.WebSocket.ReadBufferSize
	cfg.WriteBufferSize = appCfg.WebSocket.WriteBufferSize
	cfg.UseWriteBufferPool = appCfg.WebSocket.UseWriteBufferPool
	cfg.WriteTimeout = appCfg.WebSocket.WriteTimeout.ToDuration()
	cfg.MessageSizeLimit = appCfg.WebSocket.MessageSizeLimit
	cfg.DecompressedMessageSizeLimit = appCfg.WebSocket.DecompressedMessageSizeLimit
	checkOrigin, err := getCheckOrigin(appCfg)
	if err != nil {
		return cfg, err
	}
	cfg.CheckOrigin = checkOrigin
	cfg.PingPongConfig, err = getPingPongConfig(appCfg)
	if err != nil {
		return cfg, err
	}
	return cfg, nil
}

func httpStreamHandlerConfig(appCfg config.Config) (centrifuge.HTTPStreamConfig, error) {
	pingPong, err := getPingPongConfig(appCfg)
	if err != nil {
		return centrifuge.HTTPStreamConfig{}, err
	}
	return centrifuge.HTTPStreamConfig{
		MaxRequestBodySize: appCfg.HTTPStream.MaxRequestBodySize,
		PingPongConfig:     pingPong,
	}, nil
}

func sseHandlerConfig(appCfg config.Config) (centrifuge.SSEConfig, error) {
	pingPong, err := getPingPongConfig(appCfg)
	if err != nil {
		return centrifuge.SSEConfig{}, err
	}
	return centrifuge.SSEConfig{
		MaxRequestBodySize: appCfg.SSE.MaxRequestBodySize,
		PingPongConfig:     pingPong,
	}, nil
}

func emulationHandlerConfig(cfg config.Config) centrifuge.EmulationConfig {
	return centrifuge.EmulationConfig{
		MaxRequestBodySize: cfg.Emulation.MaxRequestBodySize,
	}
}

// httpServer is an HTTP server with its bound listeners.
type httpServer struct {
	server       *http.Server
	ln           net.Listener
	tls          bool
	logLine      string // logged when it starts serving
	listenErrMsg string // logged when serving fails
	// HTTP/3 (with WebTransport when wt is set) on udp, HTTP over TLS on tcp.
	h3  *http3.Server
	wt  *webtransport.Server
	udp *net.UDPConn
	tcp *net.TCPListener
}

// buildHTTPServers builds HTTP servers for the external and internal
// addresses serving the handlers enabled for each, plus routes added by
// extensions, and binds their listeners.
func (a *App) buildHTTPServers() error {
	cfg := a.container.Config()
	var routes []Route
	if a.muxCtx != nil {
		routes = a.muxCtx.routes
	}

	debug := cfg.Debug.Enabled
	useAdmin := cfg.Admin.Enabled
	usePrometheus := cfg.Prometheus.Enabled
	useHealth := cfg.Health.Enabled
	useSwagger := cfg.Swagger.Enabled
	useConnInit := cfg.Init.Enabled

	adminExternal := cfg.Admin.External
	apiExternal := cfg.HttpAPI.External

	apiDisabled := cfg.HttpAPI.Disabled

	httpAddress := cfg.HTTP.Address
	httpPort := strconv.Itoa(cfg.HTTP.Port)
	httpInternalAddress := cfg.HTTP.InternalAddress
	httpInternalPort := cfg.HTTP.InternalPort

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
		if !cfg.HTTP.HTTP3.Enabled {
			return logFatal(nil, "can not enable webtransport without experimental HTTP/3")
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
	if useConnInit {
		portFlags |= HandlerInit
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

	// Routes of extensions per address. Routes which are not IfAbsent make a
	// server start on their address.
	addrToRoutes := map[string][]Route{}
	for _, r := range routes {
		addr := externalAddr
		if r.Port == PortInternal {
			addr = internalAddr
		}
		addrToRoutes[addr] = append(addrToRoutes[addr], r)
		if _, ok := addrToHandlerFlags[addr]; !ok {
			addrToHandlerFlags[addr] = 0
		}
	}
	startsServer := func(addr string, flags HandlerFlag) bool {
		if flags != 0 {
			return true
		}
		for _, r := range addrToRoutes[addr] {
			if !r.IfAbsent {
				return true
			}
		}
		return false
	}

	tlsConfig, err := a.getTLSConfig(cfg)
	if err != nil {
		return logFatal(err, "can not get TLS config")
	}
	if err := a.listenACME(); err != nil {
		return err
	}
	var internalTLSConfig *tls.Config
	if cfg.HTTP.InternalTLS.Enabled {
		internalTLSConfig, err = cfg.HTTP.InternalTLS.ToGoTLSConfig("internal_tls")
		if err != nil {
			return logFatal(err, "can not get internal TLS config")
		}
	}

	// Iterate over port-to-flags mapping and start HTTP servers
	// on separate ports serving handlers specified in flags.
	for addr, handlerFlags := range addrToHandlerFlags {
		addr := addr
		if !startsServer(addr, handlerFlags) {
			continue
		}
		var addrTLSConfig *tls.Config
		if !cfg.HTTP.TLSExternal || addr == externalAddr {
			addrTLSConfig = tlsConfig
		}
		if addr != externalAddr && cfg.HTTP.InternalTLS.Enabled {
			addrTLSConfig = internalTLSConfig
		}

		useHTTP3 := cfg.HTTP.HTTP3.Enabled && addr == externalAddr
		useWebtransport := useHTTP3 && handlerFlags&HandlerWebtransport != 0

		var wtServer *webtransport.Server
		if useWebtransport {
			checkOrigin, err := getCheckOrigin(cfg)
			if err != nil {
				return err
			}
			wtServer = &webtransport.Server{
				CheckOrigin: checkOrigin,
			}
		}

		mux, routeNames, err := a.Mux(handlerFlags, addrToRoutes[addr], wtServer)
		if err != nil {
			return err
		}

		var h3Server *http3.Server
		if useHTTP3 {
			h3Server = &http3.Server{
				Addr:      addr,
				TLSConfig: addrTLSConfig,
				Handler:   mux,
			}
			if useWebtransport {
				// webtransport.Server.Serve passes H3.TLSConfig to QUIC as is,
				// while http3.Server.Serve selects the h3 ALPN first. Without it
				// the listener negotiates no application protocol, and clients
				// offering h3 (all browsers) fail the handshake.
				if addrTLSConfig != nil {
					h3Server.TLSConfig = http3.ConfigureTLSConfig(addrTLSConfig)
				}
				webtransport.ConfigureHTTP3Server(h3Server)
				wtServer.H3 = h3Server
			}
		}

		var protoSuffix string
		if useHTTP3 {
			protoSuffix = " with HTTP/3 (experimental)"
		}

		useH2C := cfg.HTTP.H2CExternal && addr == externalAddr && addrTLSConfig == nil
		if useH2C {
			protoSuffix = " with HTTP/2 cleartext"
		}

		endpoints := handlerFlags.String()
		if len(routeNames) > 0 {
			if endpoints != "" {
				endpoints += ", "
			}
			endpoints += strings.Join(routeNames, ", ")
		}
		logLine := fmt.Sprintf("serving %s endpoints on %s%s", endpoints, addr, protoSuffix)

		var handler http.Handler = mux
		if useHTTP3 {
			handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = h3Server.SetQUICHeaders(w.Header())
				mux.ServeHTTP(w, r)
			})
		}

		server := &http.Server{
			Addr:              addr,
			Handler:           handler,
			TLSConfig:         addrTLSConfig,
			ErrorLog:          stdlog.New(&httpErrorLogWriter{Logger: log.Logger}, "", 0),
			ReadHeaderTimeout: 10 * time.Second,
			ReadTimeout:       30 * time.Second,
			WriteTimeout:      10 * time.Second,
		}
		if useH2C {
			protocols := new(http.Protocols)
			protocols.SetHTTP1(true)
			protocols.SetUnencryptedHTTP2(true)
			server.Protocols = protocols
		}

		hs := &httpServer{server: server, logLine: logLine}
		a.httpServers = append(a.httpServers, hs)

		if useHTTP3 {
			if addrTLSConfig == nil {
				return logFatal(nil, "HTTP/3 requires TLS configured")
			}
			if cfg.HTTP.TLSAutocert.Enabled {
				return logFatal(nil, "can not use HTTP/3 with autocert")
			}
			hs.h3 = h3Server
			hs.wt = wtServer
			if err := a.listenHTTP3(hs, addr); err != nil {
				return err
			}
			continue
		}

		// Listen here rather than in ListenAndServe, so that bound addresses
		// are known when Start returns.
		hs.listenErrMsg = "error ListenAndServe"
		if addrTLSConfig != nil {
			hs.listenErrMsg = "error ListenAndServeTLS"
			hs.tls = true
		}
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			return logFatal(err, hs.listenErrMsg)
		}
		a.lifecycle.OnStop("HTTP listener "+addr, func(context.Context) error {
			_ = ln.Close()
			return nil
		})
		a.addAddr(ln.Addr())
		hs.ln = ln
	}

	return nil
}

// listenHTTP3 binds the UDP and TCP listeners of addr for HTTP/3 and HTTP
// over TLS.
func (a *App) listenHTTP3(hs *httpServer, addr string) error {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return logFatal(err, "can not start HTTP/3, resolve UDP")
	}
	udpConn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return logFatal(err, "can not start HTTP/3, listen UDP")
	}
	a.lifecycle.OnStop("HTTP/3 UDP listener "+addr, func(context.Context) error {
		_ = udpConn.Close()
		return nil
	})

	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		return logFatal(err, "can not start HTTP/3, resolve TCP")
	}
	tcpConn, err := net.ListenTCP("tcp", tcpAddr)
	if err != nil {
		return logFatal(err, "can not start HTTP/3, listen TCP")
	}
	a.lifecycle.OnStop("HTTP/3 TCP listener "+addr, func(context.Context) error {
		_ = tcpConn.Close()
		return nil
	})
	a.addAddr(tcpConn.Addr())
	a.addAddr(udpConn.LocalAddr())
	hs.udp = udpConn
	hs.tcp = tcpConn
	return nil
}

// serveHTTP serves hs in goroutines tracked by wg.
func (a *App) serveHTTP(hs *httpServer, wg *sync.WaitGroup) {
	log.Info().Msg(hs.logLine)
	if hs.udp != nil {
		a.serveHTTP3(hs, wg)
		return
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		var err error
		if hs.tls {
			err = hs.server.ServeTLS(hs.ln, "", "")
		} else {
			err = hs.server.Serve(hs.ln)
		}
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			a.serveError(err, hs.listenErrMsg)
		}
	}()
}

// serveHTTP3 serves HTTP/3 (with WebTransport when set) and HTTP over TLS on
// the listeners of hs. When the HTTP server stops, HTTP/3 is closed.
func (a *App) serveHTTP3(hs *httpServer, wg *sync.WaitGroup) {
	tlsConn := tls.NewListener(hs.tcp, hs.server.TLSConfig)
	// Buffered: the goroutine whose result isn't awaited returns too.
	hErr := make(chan error, 1)
	qErr := make(chan error, 1)
	wg.Add(3)
	go func() {
		defer wg.Done()
		hErr <- hs.server.Serve(tlsConn)
	}()
	go func() {
		defer wg.Done()
		if hs.wt != nil {
			qErr <- hs.wt.Serve(hs.udp)
		} else {
			qErr <- hs.h3.Serve(hs.udp)
		}
	}()
	go func() {
		defer wg.Done()
		defer func() { _ = hs.udp.Close() }()
		defer func() { _ = tlsConn.Close() }()

		select {
		case err := <-hErr:
			hs.closeHTTP3()
			if !errors.Is(err, http.ErrServerClosed) {
				a.serveError(err, "error ListenAndServe")
			}
		case err := <-qErr:
			// Cannot close the HTTP server or wait for requests to complete properly.
			a.serveError(err, "error ListenAndServe HTTP/3")
		}
	}()
}

// closeHTTP3 closes the HTTP/3 server of hs, if any.
func (hs *httpServer) closeHTTP3() {
	if hs.wt != nil {
		_ = hs.wt.Close()
	} else if hs.h3 != nil {
		_ = hs.h3.Close()
	}
}
