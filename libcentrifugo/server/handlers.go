package server

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/pprof"
	"path"
	"strings"
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

// ServeMux returns a mux including set of default handlers for Centrifugo server.
func ServeMux(s *HTTPServer, muxOpts MuxOptions) *http.ServeMux {

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
		mux.Handle(prefix+"/connection/websocket", s.logged(s.wrapShutdown(http.HandlerFunc(s.rawWebsocketHandler))))
	}

	if flags&HandlerSockJS != 0 {
		// register SockJS endpoints.
		sjsh := newSockJSHandler(s, path.Join(prefix, "/connection"), muxOpts.SockjsOptions)
		mux.Handle(path.Join(prefix, "/connection/")+"/", s.logged(s.wrapShutdown(sjsh)))
	}

	if flags&HandlerAPI != 0 {
		// register HTTP API endpoint.
		mux.Handle(prefix+"/api/", s.logged(s.wrapShutdown(http.HandlerFunc(s.apiHandler))))
	}

	if (admin || web) && flags&HandlerAdmin != 0 {
		// register admin websocket endpoint.
		mux.Handle(prefix+"/socket", s.logged(http.HandlerFunc(s.adminWebsocketHandler)))

		// optionally serve admin web interface.
		if web {
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

// rawWebsocketHandler called when new client connection comes to raw Websocket endpoint.
func (s *HTTPServer) rawWebsocketHandler(w http.ResponseWriter, r *http.Request) {

	plugin.Metrics.Counters.Inc("http_raw_ws_num_requests")

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

// apiHandler is responsible for receiving API commands over HTTP.
func (s *HTTPServer) apiHandler(w http.ResponseWriter, r *http.Request) {
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

// wrapShutdown will return an http Handler.
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

// adminWebsocketHandler handles admin websocket connections.
func (s *HTTPServer) adminWebsocketHandler(w http.ResponseWriter, r *http.Request) {

	config := s.node.Config()

	ws, err := websocket.Upgrade(w, r, nil, 0, 0)
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
