package server

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/pprof"
	"path"
	"strings"
	"time"

	"github.com/centrifugal/centrifugo/lib/auth"
	"github.com/centrifugal/centrifugo/lib/conns"
	"github.com/centrifugal/centrifugo/lib/logger"
	"github.com/centrifugal/centrifugo/lib/metrics"
	"github.com/centrifugal/centrifugo/lib/proto"
	clientproto "github.com/centrifugal/centrifugo/lib/proto/client"

	"github.com/gorilla/websocket"
	"github.com/igm/sockjs-go/sockjs"
)

// HandlerFlag is a bit mask of handlers that must be enabled in mux.
type HandlerFlag int

const (
	// HandlerWebsocket enables Raw Websocket handler.
	HandlerWebsocket HandlerFlag = 1 << iota
	// HandlerSockJS enables SockJS handler.
	HandlerSockJS
	// HandlerAPI enables API handler.
	HandlerAPI
	// HandlerWeb enables web interface serving.
	HandlerWeb
	// HandlerDebug enables debug handlers.
	HandlerDebug
)

var handlerText = map[HandlerFlag]string{
	HandlerWebsocket: "websocket",
	HandlerSockJS:    "SockJS",
	HandlerAPI:       "API",
	HandlerDebug:     "debug",
	HandlerWeb:       "web",
}

func (flags HandlerFlag) String() string {
	flagsOrdered := []HandlerFlag{HandlerWebsocket, HandlerSockJS, HandlerAPI, HandlerWeb, HandlerDebug}
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
	WebPath       string
	WebFS         http.FileSystem
	SockjsOptions sockjs.Options
	HandlerFlags  HandlerFlag
}

// defaultMuxOptions contain default Mux Options to start Centrifugo server.
func defaultMuxOptions() MuxOptions {
	sockjsOpts := sockjs.DefaultOptions
	sockjsOpts.SockJSURL = "//cdn.jsdelivr.net/sockjs/1.1/sockjs.min.js"
	return MuxOptions{
		HandlerFlags:  HandlerWebsocket | HandlerSockJS | HandlerAPI,
		SockjsOptions: sockjs.DefaultOptions,
	}
}

// ServeMux returns a mux including set of default handlers for Centrifugo server.
func ServeMux(s *HTTPServer, muxOpts MuxOptions) *http.ServeMux {

	mux := http.NewServeMux()

	prefix := muxOpts.Prefix
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

	if flags&HandlerWebsocket != 0 {
		// register raw Websocket endpoint.
		mux.Handle(prefix+"/connection/websocket", s.logged(s.wrapShutdown(http.HandlerFunc(s.websocketHandler))))
	}

	if flags&HandlerSockJS != 0 {
		// register SockJS endpoints.
		sjsh := newSockJSHandler(s, path.Join(prefix, "/connection/sockjs"), muxOpts.SockjsOptions)
		mux.Handle(path.Join(prefix, "/connection/sockjs")+"/", s.logged(s.wrapShutdown(sjsh)))
	}

	if flags&HandlerAPI != 0 {
		// register HTTP API endpoint.
		mux.Handle(prefix+"/api/", s.logged(s.wrapShutdown(http.HandlerFunc(s.apiHandler))))
	}

	if flags&HandlerWeb != 0 {
		// register admin web interface API endpoints.
		mux.Handle(prefix+"/auth/", s.logged(http.HandlerFunc(s.authHandler)))

		// serve web interface single-page application.
		if webPath != "" {
			webPrefix := prefix + "/"
			mux.Handle(webPrefix, http.StripPrefix(webPrefix, http.FileServer(http.Dir(webPath))))
		} else if webFS != nil {
			webPrefix := prefix + "/"
			mux.Handle(webPrefix, http.StripPrefix(webPrefix, http.FileServer(webFS)))
		}
	}

	return mux
}

// newSockJSHandler returns SockJS handler bind to sockjsPrefix url prefix.
// SockJS handler has several handlers inside responsible for various tasks
// according to SockJS protocol.
func newSockJSHandler(s *HTTPServer, sockjsPrefix string, sockjsOpts sockjs.Options) http.Handler {
	return sockjs.NewHandler(sockjsPrefix, sockjsOpts, s.sockJSHandler)
}

// sockJSHandler called when new client connection comes to SockJS endpoint.
func (s *HTTPServer) sockJSHandler(sess sockjs.Session) {

	request := sess.Request()
	ctx := request.Context()
	var connCredentials *conns.Credentials
	if val := ctx.Value(CredentialsKey); val != nil {
		if credentials, ok := val.(*conns.Credentials); ok {
			connCredentials = credentials
		}
	}

	metrics.DefaultRegistry.Counters.Inc("http_sockjs_num_requests")

	// Separate goroutine for better GC of caller's data.
	go func() {
		session := newSockjsSession(sess)
		c := conns.New(request.Context(), s.node, session, clientproto.EncodingJSON, connCredentials)
		defer c.Close(nil)

		if logger.DEBUG.Enabled() {
			logger.DEBUG.Printf("New SockJS session established with uid %s\n", c.UID())
			defer func() {
				logger.DEBUG.Printf("SockJS session with uid %s completed", c.UID())
			}()
		}

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
	}()
}

func (s *HTTPServer) websocketHandler(w http.ResponseWriter, r *http.Request) {
	metrics.DefaultRegistry.Counters.Inc("http_raw_ws_num_requests")

	s.RLock()
	wsCompression := s.config.WebsocketCompression
	wsCompressionLevel := s.config.WebsocketCompressionLevel
	wsCompressionMinSize := s.config.WebsocketCompressionMinSize
	wsReadBufferSize := s.config.WebsocketReadBufferSize
	wsWriteBufferSize := s.config.WebsocketWriteBufferSize
	s.RUnlock()

	upgrader := websocket.Upgrader{
		ReadBufferSize:    wsReadBufferSize,
		WriteBufferSize:   wsWriteBufferSize,
		EnableCompression: wsCompression,
		CheckOrigin: func(r *http.Request) bool {
			// Allow all connections.
			return true
		},
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.DEBUG.Printf("Websocket connection upgrade error: %#v", err.Error())
		return
	}

	if wsCompression {
		err := conn.SetCompressionLevel(wsCompressionLevel)
		if err != nil {
			logger.ERROR.Printf("Error setting websocket compression level: %v", err)
		}
	}

	config := s.node.Config()
	pingInterval := config.PingInterval
	writeTimeout := config.ClientMessageWriteTimeout
	maxRequestSize := config.ClientRequestMaxSize

	if maxRequestSize > 0 {
		conn.SetReadLimit(int64(maxRequestSize))
	}
	if pingInterval > 0 {
		pongWait := pingInterval * 10 / 9
		conn.SetReadDeadline(time.Now().Add(pongWait))
		conn.SetPongHandler(func(string) error { conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })
	}

	ctx := r.Context()
	var connCredentials *conns.Credentials
	if val := ctx.Value(CredentialsKey); val != nil {
		if credentials, ok := val.(*conns.Credentials); ok {
			connCredentials = credentials
		}
	}

	var enc = clientproto.EncodingJSON
	format := r.URL.Query().Get("format")
	if format == "protobuf" {
		enc = clientproto.EncodingProtobuf
	}

	// Separate goroutine for better GC of caller's data.
	go func() {
		opts := &wsSessionOptions{
			pingInterval:       pingInterval,
			writeTimeout:       writeTimeout,
			compressionMinSize: wsCompressionMinSize,
		}
		session := newWSSession(conn, opts)
		c := conns.New(r.Context(), s.node, session, enc, connCredentials)
		defer c.Close(nil)

		if logger.DEBUG.Enabled() {
			logger.DEBUG.Printf("New raw websocket session established with uid %s\n", c.UID())
			defer func() {
				logger.DEBUG.Printf("Raw websocket session with uid %s completed", c.UID())
			}()
		}

		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				return
			}
			err = c.Handle(message)
			if err != nil {
				return
			}
		}
	}()
}

// apiHandler is responsible for receiving API commands over HTTP.
func (s *HTTPServer) apiHandler(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	defer func() {
		metrics.DefaultRegistry.HDRHistograms.RecordMicroseconds("http_api", time.Now().Sub(started))
	}()
	metrics.DefaultRegistry.Counters.Inc("http_api_num_requests")

	var data []byte
	var err error

	data, err = ioutil.ReadAll(r.Body)
	if err != nil {
		logger.ERROR.Printf("error reading body: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if len(data) == 0 {
		logger.ERROR.Println("no data found in API request")
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	contentType := r.Header.Get("Content-Type")
	var resp []byte
	if strings.HasPrefix(strings.ToLower(contentType), "application/json") {
		resp, err = s.jsonAPIHandler.Handle(data)
	} else {
		resp, err = s.protobufAPIHandler.Handle(data)
	}
	if err != nil {
		logger.ERROR.Printf("error handling request: %v", err)
		if err == proto.ErrInvalidData {
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(resp)
}

const insecureWebToken = "insecure"

// authHandler allows to get admin web interface token.
func (s *HTTPServer) authHandler(w http.ResponseWriter, r *http.Request) {
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

// wrapShutdown will return http Handler.
// If Application in shutdown it will return http.StatusServiceUnavailable.
func (s *HTTPServer) wrapShutdown(h http.Handler) http.Handler {
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

// logged middleware logs request.
func (s *HTTPServer) logged(h http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		var start time.Time
		if logger.DEBUG.Enabled() {
			start = time.Now()
		}
		h.ServeHTTP(w, r)
		if logger.DEBUG.Enabled() {
			addr := r.Header.Get("X-Real-IP")
			if addr == "" {
				addr = r.Header.Get("X-Forwarded-For")
				if addr == "" {
					addr = r.RemoteAddr
				}
			}
			logger.DEBUG.Printf("%s %s from %s completed in %s\n", r.Method, r.URL.Path, addr, time.Since(start))
		}
		return
	}
	return http.HandlerFunc(fn)
}
