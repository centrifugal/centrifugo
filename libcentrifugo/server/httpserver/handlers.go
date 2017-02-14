package httpserver

import (
	"crypto/tls"
	"encoding/json"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/centrifugal/centrifugo/libcentrifugo/api/v1"
	"github.com/centrifugal/centrifugo/libcentrifugo/auth"
	"github.com/centrifugal/centrifugo/libcentrifugo/conns"
	"github.com/centrifugal/centrifugo/libcentrifugo/conns/adminconn"
	"github.com/centrifugal/centrifugo/libcentrifugo/conns/clientconn"
	"github.com/centrifugal/centrifugo/libcentrifugo/logger"
	"github.com/centrifugal/centrifugo/libcentrifugo/plugin"
	"github.com/centrifugal/centrifugo/libcentrifugo/proto"
	"github.com/gorilla/websocket"
	"github.com/igm/sockjs-go/sockjs"
	"github.com/rakyll/statik/fs"
	"golang.org/x/crypto/acme/autocert"
	"golang.org/x/net/http2"
)

// HandlerFlag is a bit mask of handlers that must be enabled in mux.
type HandlerFlag int

const (
	// HandlerRawWS enables Raw Websocket handler.
	HandlerRawWS HandlerFlag = 1 << iota
	// HandlerSockJS enables SockJS handler.
	HandlerSockJS
	// HandlerAPI enables API handler.
	HandlerAPI
	// HandlerAdmin enables admin handlers - admin websocket, web interface endpoints.
	HandlerAdmin
	// HandlerDebug enables debug handlers.
	HandlerDebug
)

var handlerText = map[HandlerFlag]string{
	HandlerRawWS:  "raw websocket",
	HandlerSockJS: "SockJS",
	HandlerAPI:    "API",
	HandlerAdmin:  "admin",
	HandlerDebug:  "debug",
}

func (flags HandlerFlag) String() string {
	flagsOrdered := []HandlerFlag{HandlerRawWS, HandlerSockJS, HandlerAPI, HandlerAdmin, HandlerDebug}
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

// MuxOptions contain various options for DefaultMux.
type MuxOptions struct {
	Prefix        string
	Admin         bool
	Web           bool
	WebPath       string
	WebFS         http.FileSystem
	SockjsOptions sockjs.Options
	HandlerFlags  HandlerFlag
}

// DefaultMuxOptions contain default SockJS options.
var DefaultMuxOptions = MuxOptions{
	HandlerFlags:  HandlerRawWS | HandlerSockJS | HandlerAPI | HandlerAdmin,
	SockjsOptions: sockjs.DefaultOptions,
}

func (s *HTTPServer) runHTTPServer() error {

	nodeConfig := s.node.Config()

	debug := nodeConfig.Debug
	adminEnabled := nodeConfig.Admin

	s.RLock()
	sockjsURL := s.config.SockjsURL
	sockjsHeartbeatDelay := s.config.SockjsHeartbeatDelay
	webEnabled := s.config.Web
	webPath := s.config.WebPath
	sslEnabled := s.config.SSL
	sslCert := s.config.SSLCert
	sslKey := s.config.SSLKey
	sslAutocertEnabled := s.config.SSLAutocert
	sslAutocertHostWhitelist := s.config.SSLAutocertHostWhitelist
	sslAutocertCacheDir := s.config.SSLAutocertCacheDir
	sslAutocertEmail := s.config.SSLAutocertEmail
	sslAutocertForceRSA := s.config.SSLAutocertForceRSA
	sslAutocertServerName := s.config.SSLAutocertServerName
	address := s.config.HTTPAddress
	clientPort := s.config.HTTPPort
	adminPort := s.config.HTTPAdminPort
	apiPort := s.config.HTTPAPIPort
	httpPrefix := s.config.HTTPPrefix
	wsReadBufferSize := s.config.WebsocketReadBufferSize
	wsWriteBufferSize := s.config.WebsocketWriteBufferSize
	s.RUnlock()

	if wsReadBufferSize > 0 {
		sockjs.WebSocketReadBufSize = wsReadBufferSize
	}
	if wsWriteBufferSize > 0 {
		sockjs.WebSocketWriteBufSize = wsWriteBufferSize
	}

	sockjsOpts := sockjs.DefaultOptions

	// Override sockjs url. It's important to use the same SockJS library version
	// on client and server sides when using iframe based SockJS transports, otherwise
	// SockJS will raise error about version mismatch.
	if sockjsURL != "" {
		logger.INFO.Println("SockJS url:", sockjsURL)
		sockjsOpts.SockJSURL = sockjsURL
	}

	sockjsOpts.HeartbeatDelay = time.Duration(sockjsHeartbeatDelay) * time.Second

	var webFS http.FileSystem
	if webEnabled {
		webFS, _ = fs.New()
	}

	if webEnabled {
		adminEnabled = true
	}

	if apiPort == "" {
		apiPort = clientPort
	}
	if adminPort == "" {
		adminPort = clientPort
	}

	// portToHandlerFlags contains mapping between ports and handler flags
	// to serve on this port.
	portToHandlerFlags := map[string]HandlerFlag{}

	var portFlags HandlerFlag

	portFlags = portToHandlerFlags[clientPort]
	portFlags |= HandlerRawWS | HandlerSockJS
	portToHandlerFlags[clientPort] = portFlags

	portFlags = portToHandlerFlags[apiPort]
	portFlags |= HandlerAPI
	portToHandlerFlags[apiPort] = portFlags

	portFlags = portToHandlerFlags[adminPort]
	if adminEnabled {
		portFlags |= HandlerAdmin
	}
	if debug {
		portFlags |= HandlerDebug
	}
	portToHandlerFlags[adminPort] = portFlags

	var wg sync.WaitGroup
	// Iterate over port to flags mapping and start HTTP servers
	// on separate ports serving handlers specified in flags.
	for handlerPort, handlerFlags := range portToHandlerFlags {
		muxOpts := MuxOptions{
			Prefix:        httpPrefix,
			Admin:         adminEnabled,
			Web:           webEnabled,
			WebPath:       webPath,
			WebFS:         webFS,
			HandlerFlags:  handlerFlags,
			SockjsOptions: sockjsOpts,
		}
		mux := DefaultMux(s, muxOpts)

		addr := net.JoinHostPort(address, handlerPort)

		logger.INFO.Printf("Start serving %s endpoints on %s\n", handlerFlags, addr)

		wg.Add(1)
		go func() {
			defer wg.Done()

			if sslAutocertEnabled {
				certManager := autocert.Manager{
					Prompt:   autocert.AcceptTOS,
					ForceRSA: sslAutocertForceRSA,
					Email:    sslAutocertEmail,
				}
				if sslAutocertHostWhitelist != nil {
					certManager.HostPolicy = autocert.HostWhitelist(sslAutocertHostWhitelist...)
				}
				if sslAutocertCacheDir != "" {
					certManager.Cache = autocert.DirCache(sslAutocertCacheDir)
				}
				server := &http.Server{
					Addr:    addr,
					Handler: mux,
					TLSConfig: &tls.Config{
						GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
							// See https://github.com/centrifugal/centrifugo/issues/144#issuecomment-279393819
							if sslAutocertServerName != "" && hello.ServerName == "" {
								hello.ServerName = sslAutocertServerName
							}
							return certManager.GetCertificate(hello)
						},
					},
				}

				// TODO: actually we won't need this when building with Go > 1.8
				// See https://github.com/centrifugal/centrifugo/issues/145
				if !strings.Contains(os.Getenv("GODEBUG"), "http2server=0") {
					if err := http2.ConfigureServer(server, nil); err != nil {
						logger.FATAL.Fatalln("Configuring HTTP/2 server error:", err)
					}
				}

				if err := server.ListenAndServeTLS("", ""); err != nil {
					logger.FATAL.Fatalln("ListenAndServe:", err)
				}
			} else if sslEnabled {
				// Autocert disabled - just try to use provided SSL cert and key files.
				server := &http.Server{Addr: addr, Handler: mux}

				// TODO: actually we won't need this when building with Go > 1.8
				// See https://github.com/centrifugal/centrifugo/issues/145
				if !strings.Contains(os.Getenv("GODEBUG"), "http2server=0") {
					if err := http2.ConfigureServer(server, nil); err != nil {
						logger.FATAL.Fatalln("Configuring HTTP/2 server error:", err)
					}
				}

				if err := server.ListenAndServeTLS(sslCert, sslKey); err != nil {
					logger.FATAL.Fatalln("ListenAndServe:", err)
				}
			} else {
				if err := http.ListenAndServe(addr, mux); err != nil {
					logger.FATAL.Fatalln("ListenAndServe:", err)
				}
			}
		}()
	}
	wg.Wait()

	return nil
}

// DefaultMux returns a mux including set of default handlers for Centrifugo server.
func DefaultMux(s *HTTPServer, muxOpts MuxOptions) *http.ServeMux {

	mux := http.NewServeMux()

	prefix := muxOpts.Prefix
	admin := muxOpts.Admin
	web := muxOpts.Web
	webPath := muxOpts.WebPath
	webFS := muxOpts.WebFS
	flags := muxOpts.HandlerFlags

	if flags&HandlerDebug != 0 {
		mux.Handle(prefix+"/debug/pprof/", http.HandlerFunc(pprof.Index))
		mux.Handle(prefix+"/debug/pprof/cmdline", http.HandlerFunc(pprof.Cmdline))
		mux.Handle(prefix+"/debug/pprof/profile", http.HandlerFunc(pprof.Profile))
		mux.Handle(prefix+"/debug/pprof/symbol", http.HandlerFunc(pprof.Symbol))
		mux.Handle(prefix+"/debug/pprof/trace", http.HandlerFunc(pprof.Trace))
	}

	if flags&HandlerRawWS != 0 {
		// register raw Websocket endpoint.
		mux.Handle(prefix+"/connection/websocket", s.Logged(s.WrapShutdown(http.HandlerFunc(s.RawWebsocketHandler))))
	}

	if flags&HandlerSockJS != 0 {
		// register SockJS endpoints.
		sjsh := NewSockJSHandler(s, prefix+"/connection", muxOpts.SockjsOptions)
		mux.Handle(prefix+"/connection/", s.Logged(s.WrapShutdown(sjsh)))
	}

	if flags&HandlerAPI != 0 {
		// register HTTP API endpoint.
		mux.Handle(prefix+"/api/", s.Logged(s.WrapShutdown(http.HandlerFunc(s.APIHandler))))
	}

	if (admin || web) && flags&HandlerAdmin != 0 {
		// register admin websocket endpoint.
		mux.Handle(prefix+"/socket", s.Logged(http.HandlerFunc(s.AdminWebsocketHandler)))

		// optionally serve admin web interface.
		if web {
			// register admin web interface API endpoints.
			mux.Handle(prefix+"/auth/", s.Logged(http.HandlerFunc(s.AuthHandler)))

			// serve web interface single-page application.
			if webPath != "" {
				webPrefix := prefix + "/"
				mux.Handle(webPrefix, http.StripPrefix(webPrefix, http.FileServer(http.Dir(webPath))))
			} else if webFS != nil {
				webPrefix := prefix + "/"
				mux.Handle(webPrefix, http.StripPrefix(webPrefix, http.FileServer(webFS)))
			}
		}
	}

	return mux
}

// NewSockJSHandler returns SockJS handler bind to sockjsPrefix url prefix.
// SockJS handler has several handlers inside responsible for various tasks
// according to SockJS protocol.
func NewSockJSHandler(s *HTTPServer, sockjsPrefix string, sockjsOpts sockjs.Options) http.Handler {
	return sockjs.NewHandler(sockjsPrefix, sockjsOpts, s.sockJSHandler)
}

// sockJSHandler called when new client connection comes to SockJS endpoint.
func (s *HTTPServer) sockJSHandler(sess sockjs.Session) {

	plugin.Metrics.Counters.Inc("http_sockjs_num_requests")

	c, err := clientconn.New(s.node, newSockjsSession(sess))
	if err != nil {
		logger.ERROR.Println(err)
		sess.Close(3000, "Internal Server Error")
		return
	}
	defer c.Close(nil)

	logger.DEBUG.Printf("New SockJS session established with uid %s\n", c.UID())
	defer func() {
		logger.DEBUG.Printf("SockJS session with uid %s completed", c.UID())
	}()

	for {
		if msg, err := sess.Recv(); err == nil {
			err = c.Handle([]byte(msg))
			if err != nil {
				return
			}
			continue
		}
		break
	}
}

// RawWebsocketHandler called when new client connection comes to raw Websocket endpoint.
func (s *HTTPServer) RawWebsocketHandler(w http.ResponseWriter, r *http.Request) {

	plugin.Metrics.Counters.Inc("http_raw_ws_num_requests")

	s.RLock()
	wsCompression := s.config.WebsocketCompression
	wsCompressionLevel := s.config.WebsocketCompressionLevel
	wsCompressionMinSize := s.config.WebsocketCompressionMinSize
	wsReadBufferSize := s.config.WebsocketReadBufferSize
	wsWriteBufferSize := s.config.WebsocketWriteBufferSize
	s.RUnlock()

	if wsReadBufferSize == 0 {
		wsReadBufferSize = sockjs.WebSocketReadBufSize
	}
	if wsWriteBufferSize == 0 {
		wsWriteBufferSize = sockjs.WebSocketWriteBufSize
	}

	upgrader := websocket.Upgrader{
		ReadBufferSize:    wsReadBufferSize,
		WriteBufferSize:   wsWriteBufferSize,
		EnableCompression: wsCompression,
		CheckOrigin: func(r *http.Request) bool {
			// Allow all connections.
			return true
		},
	}

	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.DEBUG.Printf("Websocket connection upgrade error: %#v", err.Error())
		return
	}

	if wsCompression {
		err := ws.SetCompressionLevel(wsCompressionLevel)
		if err != nil {
			logger.ERROR.Printf("Error setting websocket compression level: %v", err)
		}
	}

	config := s.node.Config()
	pingInterval := config.PingInterval
	writeTimeout := config.ClientMessageWriteTimeout

	if pingInterval > 0 {
		pongWait := pingInterval * 10 / 9 // https://github.com/gorilla/websocket/blob/master/examples/chat/conn.go#L22
		ws.SetReadDeadline(time.Now().Add(pongWait))
		ws.SetPongHandler(func(string) error { ws.SetReadDeadline(time.Now().Add(pongWait)); return nil })
	}

	c, err := clientconn.New(s.node, newWSSession(ws, pingInterval, writeTimeout, wsCompressionMinSize))
	if err != nil {
		logger.ERROR.Println(err)
		ws.Close()
		return
	}
	defer c.Close(nil)

	logger.DEBUG.Printf("New raw websocket session established with uid %s\n", c.UID())
	defer func() {
		logger.DEBUG.Printf("Raw websocket session with uid %s completed", c.UID())
	}()

	for {
		_, message, err := ws.ReadMessage()
		if err != nil {
			return
		}
		err = c.Handle(message)
		if err != nil {
			return
		}
	}
}

// APIHandler is responsible for receiving API commands over HTTP.
func (s *HTTPServer) APIHandler(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	defer func() {
		plugin.Metrics.HDRHistograms.RecordMicroseconds("http_api", time.Now().Sub(started))
	}()
	plugin.Metrics.Counters.Inc("http_api_num_requests")

	contentType := r.Header.Get("Content-Type")

	var sign string
	var data []byte
	var err error

	config := s.node.Config()
	secret := config.Secret
	insecure := config.InsecureAPI

	if strings.HasPrefix(strings.ToLower(contentType), "application/json") {
		// json request, this is a prefferred more performant way, as parsing
		// Form Value rather expensive (about 30% speed up).
		if !insecure {
			sign = r.Header.Get("X-API-Sign")
		}
		defer r.Body.Close()
		data, err = ioutil.ReadAll(r.Body)
		if err != nil {
			logger.ERROR.Println(err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
	} else {
		// application/x-www-form-urlencoded request
		if !insecure {
			sign = r.FormValue("sign")
		}
		data = []byte(r.FormValue("data"))
	}

	if sign == "" && !insecure {
		logger.ERROR.Println("no sign found in API request")
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	if len(data) == 0 {
		logger.ERROR.Println("no data found in API request")
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	if !insecure {
		if secret == "" {
			logger.ERROR.Println("no secret set in config")
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}
		isValid := auth.CheckApiSign(secret, data, sign)
		if !isValid {
			logger.ERROR.Println("invalid sign")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	}

	jsonResp, err := apiv1.ProcessAPIData(s.node, data)
	if err != nil {
		if err == proto.ErrInvalidMessage {
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(jsonResp)
}

const insecureWebToken = "insecure"

// AuthHandler allows to get admin web interface token.
func (s *HTTPServer) AuthHandler(w http.ResponseWriter, r *http.Request) {
	password := r.FormValue("password")

	config := s.node.Config()
	insecure := config.InsecureAdmin
	adminPassword := config.AdminPassword
	adminSecret := config.AdminSecret

	if insecure {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]string{"token": insecureWebToken}
		json.NewEncoder(w).Encode(resp)
		return
	}

	if adminPassword == "" || adminSecret == "" {
		logger.ERROR.Println("admin_password and admin_secret must be set in configuration")
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	if password == adminPassword {
		w.Header().Set("Content-Type", "application/json")
		token, err := auth.GenerateAdminToken(adminSecret)
		if err != nil {
			logger.ERROR.Println(err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		resp := map[string]string{
			"token": token,
		}
		json.NewEncoder(w).Encode(resp)
		return
	}
	http.Error(w, "Bad Request", http.StatusBadRequest)
}

// WrapShutdown will return an http Handler.
// If Application in shutdown it will return http.StatusServiceUnavailable.
func (s *HTTPServer) WrapShutdown(h http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		s.RLock()
		shutdown := s.shutdown
		s.RUnlock()
		if shutdown {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		h.ServeHTTP(w, r)
	}
	return http.HandlerFunc(fn)
}

// Logged middleware logs request.
func (s *HTTPServer) Logged(h http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		addr := r.Header.Get("X-Real-IP")
		if addr == "" {
			addr = r.Header.Get("X-Forwarded-For")
			if addr == "" {
				addr = r.RemoteAddr
			}
		}
		h.ServeHTTP(w, r)
		if logger.DEBUG.Enabled() {
			logger.DEBUG.Printf("%s %s from %s completed in %s\n", r.Method, r.URL.Path, addr, time.Since(start))
		}
		return
	}
	return http.HandlerFunc(fn)
}

const (
	// AdminWebsocketReadBufferSize is a size of read buffer for admin websocket connection.
	AdminWebsocketReadBufferSize = 1024
	// AdminWebsocketWriteBufferSize is a size of write buffer for admin websocket connection.
	AdminWebsocketWriteBufferSize = 1024
)

var upgrader = &websocket.Upgrader{ReadBufferSize: AdminWebsocketReadBufferSize, WriteBufferSize: AdminWebsocketWriteBufferSize}

// AdminWebsocketHandler handles admin websocket connections.
func (s *HTTPServer) AdminWebsocketHandler(w http.ResponseWriter, r *http.Request) {

	config := s.node.Config()
	admin := config.Admin

	s.RLock()
	web := s.config.Web
	s.RUnlock()

	if !(admin || web) {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	ws, err := websocket.Upgrade(w, r, nil, sockjs.WebSocketReadBufSize, sockjs.WebSocketWriteBufSize)
	if err != nil {
		logger.DEBUG.Printf("Admin connection upgrade error: %#v", err.Error())
		return
	}

	pingInterval := config.PingInterval
	writeTimeout := config.ClientMessageWriteTimeout

	if pingInterval > 0 {
		pongWait := pingInterval * 10 / 9 // https://github.com/gorilla/websocket/blob/master/examples/chat/conn.go#L22
		ws.SetReadDeadline(time.Now().Add(pongWait))
		ws.SetPongHandler(func(string) error { ws.SetReadDeadline(time.Now().Add(pongWait)); return nil })
	}

	sess := newWSSession(ws, pingInterval, writeTimeout, 0)

	c, err := adminconn.New(s.node, sess)
	if err != nil {
		sess.Close(&conns.DisconnectAdvice{Reason: proto.ErrInternalServerError.Error(), Reconnect: true})
		return
	}
	defer c.Close(nil)

	start := time.Now()
	logger.DEBUG.Printf("New admin session established with uid %s\n", c.UID())
	defer func() {
		logger.DEBUG.Printf("Admin session completed in %s, uid %s", time.Since(start), c.UID())
	}()

	for {
		_, message, err := sess.ws.ReadMessage()
		if err != nil {
			break
		}
		err = c.Handle(message)
		if err != nil {
			c.Close(&conns.DisconnectAdvice{Reason: err.Error(), Reconnect: true})
			break
		}
	}
}
