package libcentrifugo

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/pprof"
	"strings"
	"time"

	"github.com/centrifugal/centrifugo/Godeps/_workspace/src/github.com/FZambia/go-logger"
	"github.com/centrifugal/centrifugo/Godeps/_workspace/src/github.com/gorilla/websocket"
	"github.com/centrifugal/centrifugo/Godeps/_workspace/src/gopkg.in/igm/sockjs-go.v2/sockjs"
	"github.com/centrifugal/centrifugo/libcentrifugo/auth"
)

type HandlerFlag int

const (
	HandlerRawWS HandlerFlag = 1 << iota
	HandlerSockJS
	HandlerAPI
	HandlerAdmin
	HandlerDebug
)

var handlerText map[HandlerFlag]string = map[HandlerFlag]string{
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

// DefaultMux returns a mux including set of default handlers for Centrifugo server.
func DefaultMux(app *Application, muxOpts MuxOptions) *http.ServeMux {

	mux := http.NewServeMux()

	prefix := muxOpts.Prefix
	web := muxOpts.Web
	webPath := muxOpts.WebPath
	webFS := muxOpts.WebFS
	flags := muxOpts.HandlerFlags

	if flags&HandlerDebug != 0 {
		mux.Handle(prefix+"/debug/pprof/", http.HandlerFunc(pprof.Index))
		mux.Handle(prefix+"/debug/pprof/cmdline", http.HandlerFunc(pprof.Cmdline))
		mux.Handle(prefix+"/debug/pprof/profile", http.HandlerFunc(pprof.Profile))
		mux.Handle(prefix+"/debug/pprof/symbol", http.HandlerFunc(pprof.Symbol))
	}

	if flags&HandlerRawWS != 0 {
		// register raw Websocket endpoint
		mux.Handle(prefix+"/connection/websocket", app.Logged(app.WrapShutdown(http.HandlerFunc(app.RawWebsocketHandler))))
	}

	if flags&HandlerSockJS != 0 {
		// register SockJS endpoints
		sjsh := NewSockJSHandler(app, prefix+"/connection", muxOpts.SockjsOptions)
		mux.Handle(prefix+"/connection/", app.Logged(app.WrapShutdown(sjsh)))
	}

	if flags&HandlerAPI != 0 {
		// register HTTP API endpoint
		mux.Handle(prefix+"/api/", app.Logged(app.WrapShutdown(http.HandlerFunc(app.APIHandler))))
	}

	if flags&HandlerAdmin != 0 {
		// register admin websocket endpoint
		mux.Handle(prefix+"/socket", app.Logged(http.HandlerFunc(app.AdminWebsocketHandler)))

		// optionally serve admin web interface
		if web {
			// register admin web interface API endpoints
			mux.Handle(prefix+"/auth/", app.Logged(http.HandlerFunc(app.AuthHandler)))
			mux.Handle(prefix+"/info/", app.Logged(app.Authenticated(http.HandlerFunc(app.InfoHandler))))
			mux.Handle(prefix+"/action/", app.Logged(app.Authenticated(http.HandlerFunc(app.ActionHandler))))

			// serve web interface single-page application
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
func NewSockJSHandler(app *Application, sockjsPrefix string, sockjsOpts sockjs.Options) http.Handler {
	return sockjs.NewHandler(sockjsPrefix, sockjsOpts, app.sockJSHandler)
}

type sockjsConn struct {
	sess    sockjs.Session
	closeCh chan struct{}
}

func newSockjsConn(sess sockjs.Session) *sockjsConn {
	return &sockjsConn{
		sess:    sess,
		closeCh: make(chan struct{}),
	}
}

func (conn *sockjsConn) Send(msg []byte) error {
	select {
	case <-conn.closeCh:
		return nil
	default:
		return conn.sess.Send(string(msg))
	}
}

func (conn *sockjsConn) Close(status uint32, reason string) error {
	return conn.sess.Close(status, reason)
}

// sockJSHandler called when new client connection comes to SockJS endpoint.
func (app *Application) sockJSHandler(s sockjs.Session) {

	conn := newSockjsConn(s)
	defer close(conn.closeCh)

	c, err := newClient(app, conn)
	if err != nil {
		logger.ERROR.Println(err)
		return
	}
	defer c.clean()
	logger.INFO.Printf("New SockJS session established with uid %s\n", c.uid())

	for {
		if msg, err := s.Recv(); err == nil {
			err = c.message([]byte(msg))
			if err != nil {
				logger.ERROR.Println(err)
				s.Close(CloseStatus, "error handling message")
				break
			}
			continue
		}
		break
	}
}

// wsConn is a struct to fit SockJS session interface so client will accept
// it as its sess
type wsConn struct {
	ws           *websocket.Conn
	closeCh      chan struct{}
	pingInterval time.Duration
	pingTimer    *time.Timer
}

func newWSConn(ws *websocket.Conn, pingInterval time.Duration) *wsConn {
	conn := &wsConn{
		ws:           ws,
		closeCh:      make(chan struct{}),
		pingInterval: pingInterval,
	}
	conn.pingTimer = time.AfterFunc(conn.pingInterval, conn.ping)
	return conn
}

func (conn *wsConn) ping() {
	select {
	case <-conn.closeCh:
		conn.pingTimer.Stop()
		return
	default:
		err := conn.ws.WriteControl(websocket.PingMessage, []byte("ping"), time.Now().Add(conn.pingInterval/2))
		if err != nil {
			conn.ws.Close()
			conn.pingTimer.Stop()
			return
		}
		conn.pingTimer = time.AfterFunc(conn.pingInterval, conn.ping)
	}
}

func (conn *wsConn) Send(message []byte) error {
	select {
	case <-conn.closeCh:
		return nil
	default:
		return conn.ws.WriteMessage(websocket.TextMessage, message)
	}
}

func (conn *wsConn) Close(status uint32, reason string) error {
	return conn.ws.Close()
}

// RawWebsocketHandler called when new client connection comes to raw Websocket endpoint.
func (app *Application) RawWebsocketHandler(w http.ResponseWriter, r *http.Request) {

	ws, err := websocket.Upgrade(w, r, nil, sockjs.WebSocketReadBufSize, sockjs.WebSocketWriteBufSize)
	if _, ok := err.(websocket.HandshakeError); ok {
		http.Error(w, `Can "Upgrade" only to "WebSocket".`, http.StatusBadRequest)
		return
	} else if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer ws.Close()

	app.RLock()
	pingInterval := app.config.PingInterval
	app.RUnlock()
	pongWait := pingInterval * 10 / 9 // https://github.com/gorilla/websocket/blob/master/examples/chat/conn.go#L22

	conn := newWSConn(ws, pingInterval)
	defer close(conn.closeCh)

	c, err := newClient(app, conn)
	if err != nil {
		return
	}
	logger.INFO.Printf("New raw Websocket session established with uid %s\n", c.uid())
	defer c.clean()

	ws.SetReadDeadline(time.Now().Add(pongWait))
	ws.SetPongHandler(func(string) error { ws.SetReadDeadline(time.Now().Add(pongWait)); return nil })

	for {
		_, message, err := conn.ws.ReadMessage()
		if err != nil {
			break
		}
		err = c.message(message)
		if err != nil {
			logger.ERROR.Println(err)
			conn.Close(CloseStatus, "error handling message")
			break
		}
	}
}

var (
	arrayJsonPrefix  byte = '['
	objectJsonPrefix byte = '{'
)

func cmdFromAPIMsg(msg []byte) ([]apiCommand, error) {
	var commands []apiCommand

	firstByte := msg[0]

	switch firstByte {
	case objectJsonPrefix:
		// single command request
		var command apiCommand
		err := json.Unmarshal(msg, &command)
		if err != nil {
			return nil, err
		}
		commands = append(commands, command)
	case arrayJsonPrefix:
		// array of commands received
		err := json.Unmarshal(msg, &commands)
		if err != nil {
			return nil, err
		}
	default:
		return nil, ErrInvalidMessage
	}
	return commands, nil
}

// APIHandler is responsible for receiving API commands over HTTP.
func (app *Application) APIHandler(w http.ResponseWriter, r *http.Request) {

	app.metrics.numAPIRequests.Inc(1)
	defer app.metrics.timeAPI.UpdateSince(time.Now())

	contentType := r.Header.Get("Content-Type")

	var sign string
	var data []byte
	var err error

	if strings.HasPrefix(strings.ToLower(contentType), "application/json") {
		// json request
		sign = r.Header.Get("X-API-Sign")
		defer r.Body.Close()
		data, err = ioutil.ReadAll(r.Body)
		if err != nil {
			logger.ERROR.Println(err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
	} else {
		// application/x-www-form-urlencoded request
		sign = r.FormValue("sign")
		data = []byte(r.FormValue("data"))
	}

	app.RLock()
	secret := app.config.Secret
	insecure := app.config.InsecureAPI
	app.RUnlock()

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

	commands, err := cmdFromAPIMsg(data)
	if err != nil {
		logger.ERROR.Println(err)
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	var mr multiResponse

	for _, command := range commands {
		resp, err := app.apiCmd(command)
		if err != nil {
			logger.ERROR.Println(err)
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}
		mr = append(mr, resp)
	}
	jsonResp, err := json.Marshal(mr)
	if err != nil {
		logger.ERROR.Println(err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(jsonResp)
}

// AuthHandler allows to get admin web interface token.
func (app *Application) AuthHandler(w http.ResponseWriter, r *http.Request) {
	password := r.FormValue("password")

	app.RLock()
	insecure := app.config.InsecureWeb
	webPassword := app.config.WebPassword
	webSecret := app.config.WebSecret
	app.RUnlock()

	if insecure {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]string{"token": "insecure"}
		json.NewEncoder(w).Encode(resp)
		return
	}

	if webPassword == "" || webSecret == "" {
		logger.ERROR.Println("web_password and web_secret must be set in configuration")
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
	if password == webPassword {
		w.Header().Set("Content-Type", "application/json")
		token, err := app.adminAuthToken()
		if err != nil {
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

// Authenticated middleware checks that request contains valid auth token in headers.
func (app *Application) Authenticated(h http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader != "" {
			token := strings.TrimPrefix(authHeader, "Token ")
			err := app.checkAdminAuthToken(token)
			if err == nil {
				h.ServeHTTP(w, r)
				return
			}
		}
		w.WriteHeader(401)
	}
	return http.HandlerFunc(fn)
}

// WrapShutdown will return an http Handler.
// If Application in shutdown it will return http.StatusServiceUnavailable.
func (app *Application) WrapShutdown(h http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		app.RLock()
		shutdown := app.shutdown
		app.RUnlock()
		if shutdown {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		h.ServeHTTP(w, r)
	}
	return http.HandlerFunc(fn)
}

// Logged middleware logs request.
func (app *Application) Logged(h http.Handler) http.Handler {
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
		logger.INFO.Printf("%s %s from %s completed in %s\n", r.Method, r.URL.Path, addr, time.Since(start))
		return
	}
	return http.HandlerFunc(fn)
}

// InfoHahdler allows to get actual information about Centrifugo nodes running.
func (app *Application) InfoHandler(w http.ResponseWriter, r *http.Request) {
	app.nodesMu.Lock()
	defer app.nodesMu.Unlock()
	app.RLock()
	defer app.RUnlock()
	info := map[string]interface{}{
		"version":             app.config.Version,
		"secret":              app.config.Secret,
		"connection_lifetime": app.config.ConnLifetime,
		"channel_options":     app.config.ChannelOptions,
		"namespaces":          app.config.Namespaces,
		"engine":              app.engine.name(),
		"node_name":           app.config.Name,
		"nodes":               app.nodes,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

// ActionHandler allows to call API commands via submitting a form.
func (app *Application) ActionHandler(w http.ResponseWriter, r *http.Request) {

	app.metrics.numAPIRequests.Inc(1)
	defer app.metrics.timeAPI.UpdateSince(time.Now())

	method := r.FormValue("method")

	var resp *response
	var err error

	switch method {
	case "publish":
		channel := Channel(r.FormValue("channel"))
		data := r.FormValue("data")
		if data == "" {
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}
		cmd := &publishAPICommand{
			Channel: channel,
			Data:    []byte(data),
		}
		resp, err = app.publishCmd(cmd)
	case "unsubscribe":
		channel := Channel(r.FormValue("channel"))
		user := UserID(r.FormValue("user"))
		cmd := &unsubscribeAPICommand{
			Channel: channel,
			User:    user,
		}
		resp, err = app.unsubcribeCmd(cmd)
	case "disconnect":
		user := UserID(r.FormValue("user"))
		cmd := &disconnectAPICommand{
			User: user,
		}
		resp, err = app.disconnectCmd(cmd)
	case "presence":
		channel := Channel(r.FormValue("channel"))
		cmd := &presenceAPICommand{
			Channel: channel,
		}
		resp, err = app.presenceCmd(cmd)
	case "history":
		channel := Channel(r.FormValue("channel"))
		cmd := &historyAPICommand{
			Channel: channel,
		}
		resp, err = app.historyCmd(cmd)
	case "channels":
		resp, err = app.channelsCmd()
	case "stats":
		resp, err = app.statsCmd()
	default:
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	if err != nil {
		logger.ERROR.Println(err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

var upgrader = &websocket.Upgrader{ReadBufferSize: 1024, WriteBufferSize: 1024}

// AdminWebsocketHandler handles admin websocket connections.
func (app *Application) AdminWebsocketHandler(w http.ResponseWriter, r *http.Request) {
	app.RLock()
	web := app.config.Web
	app.RUnlock()
	if !web {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer ws.Close()
	c, err := newAdminClient(app, ws)
	if err != nil {
		return
	}
	logger.INFO.Printf("New admin session established with uid %s\n", c.uid())
	defer func() {
		close(c.closeChan)
		err := app.removeAdminConn(c)
		if err != nil {
			logger.ERROR.Println(err)
		}
	}()

	go c.writer()

	for {
		_, message, err := ws.ReadMessage()
		if err != nil {
			break
		}
		resp, err := c.handleMessage(message)
		if err != nil {
			break
		}
		msgBytes, err := json.Marshal(resp)
		if err != nil {
			break
		} else {
			err := c.send(msgBytes)
			if err != nil {
				break
			}
		}
	}
}
