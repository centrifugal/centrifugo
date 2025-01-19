package app

import (
	"crypto/tls"
	"errors"
	stdlog "log"
	"net"
	"net/http"
	"net/http/pprof"
	"strconv"
	"strings"

	"github.com/centrifugal/centrifugo/v6/internal/admin"
	"github.com/centrifugal/centrifugo/v6/internal/api"
	"github.com/centrifugal/centrifugo/v6/internal/config"
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
func Mux(
	n *centrifuge.Node, cfgContainer *config.Container, apiExecutor *api.Executor, flags HandlerFlag, keepHeadersInContext bool, wtServer *webtransport.Server,
) *http.ServeMux {
	mux := http.NewServeMux()
	cfg := cfgContainer.Config()

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
		httpErrorMode, err := tools.OptionalStringChoice(cfg.HttpAPI.ErrorMode, []string{config.TransportErrorMode})
		if err != nil {
			log.Fatal().Err(err).Msg("error in config")
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

func getPingPongConfig(cfg config.Config) centrifuge.PingPongConfig {
	pingInterval := cfg.Client.PingInterval
	pongTimeout := cfg.Client.PongTimeout
	if pingInterval <= pongTimeout {
		log.Fatal().Str("ping_interval", pingInterval.String()).Str("pong_timeout", pongTimeout.String()).
			Msg("ping_interval must be greater than pong_timeout")
	}
	return centrifuge.PingPongConfig{
		PingInterval: pingInterval.ToDuration(),
		PongTimeout:  pongTimeout.ToDuration(),
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
	cfg.WriteTimeout = appCfg.WebSocket.WriteTimeout.ToDuration()
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

func runHTTPServers(
	n *centrifuge.Node, cfgContainer *config.Container, apiExecutor *api.Executor, keepHeadersInContext bool,
) ([]*http.Server, error) {
	cfg := cfgContainer.Config()

	debug := cfg.Debug.Enabled
	useAdmin := cfg.Admin.Enabled
	usePrometheus := cfg.Prometheus.Enabled
	useHealth := cfg.Health.Enabled
	useSwagger := cfg.Swagger.Enabled

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

	tlsConfig, err := GetTLSConfig(cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("can not get TLS config")
	}
	var internalTLSConfig *tls.Config
	if cfg.HTTP.InternalTLS.Enabled {
		internalTLSConfig, err = cfg.HTTP.InternalTLS.ToGoTLSConfig("internal_tls")
		if err != nil {
			log.Fatal().Err(err).Msg("can not get internal TLS config")
		}
	}

	// Iterate over port-to-flags mapping and start HTTP servers
	// on separate ports serving handlers specified in flags.
	for addr, handlerFlags := range addrToHandlerFlags {
		addr := addr
		if handlerFlags == 0 {
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
			ErrorLog:  stdlog.New(&httpErrorLogWriter{Logger: log.Logger}, "", 0),
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
					log.Fatal().Msg("HTTP/3 requires TLS configured")
				}
				if cfg.HTTP.TLSAutocert.Enabled {
					log.Fatal().Msg("can not use HTTP/3 with autocert")
				}

				udpAddr, err := net.ResolveUDPAddr("udp", addr)
				if err != nil {
					log.Fatal().Err(err).Msg("can not start HTTP/3, resolve UDP")
				}
				udpConn, err := net.ListenUDP("udp", udpAddr)
				if err != nil {
					log.Fatal().Err(err).Msg("can not start HTTP/3, listen UDP")
				}
				defer func() { _ = udpConn.Close() }()

				tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
				if err != nil {
					log.Fatal().Err(err).Msg("can not start HTTP/3, resolve TCP")
				}
				tcpConn, err := net.ListenTCP("tcp", tcpAddr)
				if err != nil {
					log.Fatal().Err(err).Msg("can not start HTTP/3, listen TCP")
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
						log.Fatal().Err(err).Msg("error ListenAndServe")
					}
				case err := <-qErr:
					// Cannot close the HTTP server or wait for requests to complete properly.
					log.Fatal().Err(err).Msg("error ListenAndServe HTTP/3")
				}
			} else {
				if addrTLSConfig != nil {
					if err := server.ListenAndServeTLS("", ""); err != nil {
						if !errors.Is(err, http.ErrServerClosed) {
							log.Fatal().Err(err).Msg("error ListenAndServeTLS")
						}
					}
				} else {
					if err := server.ListenAndServe(); err != nil {
						if !errors.Is(err, http.ErrServerClosed) {
							log.Fatal().Err(err).Msgf("error ListenAndServe")
						}
					}
				}
			}
		}()
	}

	return servers, nil
}
