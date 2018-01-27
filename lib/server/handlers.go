package server

import (
	"context"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/pprof"
	"path"
	"strings"
	"time"

	"github.com/centrifugal/centrifugo/lib/auth"
	"github.com/centrifugal/centrifugo/lib/client"
	"github.com/centrifugal/centrifugo/lib/logger"
	"github.com/centrifugal/centrifugo/lib/metrics"
	"github.com/centrifugal/centrifugo/lib/proto"
	apiproto "github.com/centrifugal/centrifugo/lib/proto/api"

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
	// HandlerAdmin enables admin web interface.
	HandlerAdmin
	// HandlerDebug enables debug handlers.
	HandlerDebug
)

var handlerText = map[HandlerFlag]string{
	HandlerWebsocket: "websocket",
	HandlerSockJS:    "SockJS",
	HandlerAPI:       "API",
	HandlerAdmin:     "admin",
	HandlerDebug:     "debug",
}

func (flags HandlerFlag) String() string {
	flagsOrdered := []HandlerFlag{HandlerWebsocket, HandlerSockJS, HandlerAPI, HandlerAdmin, HandlerDebug}
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
		mux.Handle(prefix+"/connection/websocket", s.log(s.wrapShutdown(http.HandlerFunc(s.websocketHandler))))
	}

	if flags&HandlerSockJS != 0 {
		// register SockJS endpoints.
		sjsh := newSockJSHandler(s, path.Join(prefix, "/connection/sockjs"), muxOpts.SockjsOptions)
		mux.Handle(path.Join(prefix, "/connection/sockjs")+"/", s.log(s.wrapShutdown(sjsh)))
	}

	if flags&HandlerAPI != 0 {
		// register HTTP API endpoint.
		mux.Handle(prefix+"/api/", s.log(s.apiAuth(s.wrapShutdown(http.HandlerFunc(s.apiHandler)))))
	}

	if flags&HandlerAdmin != 0 {
		// register admin web interface API endpoints.
		mux.Handle(prefix+"/admin/auth/", s.log(http.HandlerFunc(s.authHandler)))
		mux.Handle(prefix+"/admin/api/", s.log(http.HandlerFunc(s.apiHandler)))
		// serve admin single-page web application.
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

	metrics.DefaultRegistry.Counters.Inc("http_sockjs_num_requests")

	// Separate goroutine for better GC of caller's data.
	go func() {
		session := newSockjsTransport(sess)
		c := client.New(sess.Request().Context(), s.node, session, client.Config{Encoding: proto.EncodingJSON})
		defer c.Close(nil)

		if logger.DEBUG.Enabled() {
			logger.DEBUG.Printf("New SockJS session established with client ID %s\n", c.ID())
			defer func() {
				logger.DEBUG.Printf("SockJS session with client ID %s completed", c.ID())
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

	var enc = proto.EncodingJSON
	format := r.URL.Query().Get("format")
	if format == "protobuf" {
		enc = proto.EncodingProtobuf
	}

	// Separate goroutine for better GC of caller's data.
	go func() {
		opts := &websocketTransportOptions{
			pingInterval:       pingInterval,
			writeTimeout:       writeTimeout,
			compressionMinSize: wsCompressionMinSize,
		}
		session := newWebsocketTransport(conn, opts)
		c := client.New(r.Context(), s.node, session, client.Config{Encoding: enc})
		defer c.Close(nil)

		if logger.DEBUG.Enabled() {
			logger.DEBUG.Printf("New raw websocket session established with client ID %s\n", c.ID())
			defer func() {
				logger.DEBUG.Printf("Raw websocket session with client ID %s completed", c.ID())
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

func (s *HTTPServer) handleCommand(ctx context.Context, enc apiproto.Encoding, cmd *apiproto.Command) (*apiproto.Reply, error) {
	var err error

	method := cmd.Method
	params := cmd.Params

	rep := &apiproto.Reply{
		ID: cmd.ID,
	}

	var replyRes proto.Raw

	if method == "" {
		logger.ERROR.Println("method required in API command")
		rep.Error = apiproto.ErrBadRequest
		return rep, nil
	}

	decoder := apiproto.GetDecoder(enc)
	defer apiproto.PutDecoder(enc, decoder)

	encoder := apiproto.GetEncoder(enc)
	defer apiproto.PutEncoder(enc, encoder)

	switch method {
	case "publish":
		cmd, err := decoder.DecodePublish(params)
		if err != nil {
			logger.ERROR.Printf("error decoding publish params: %v", err)
			rep.Error = apiproto.ErrBadRequest
			return rep, nil
		}
		resp := s.api.Publish(ctx, cmd)
		if resp.Error != nil {
			rep.Error = resp.Error
		} else {
			if resp.Result != nil {
				replyRes, err = encoder.EncodePublish(resp.Result)
				if err != nil {
					return nil, err
				}
			}
		}
	case "broadcast":
		cmd, err := decoder.DecodeBroadcast(params)
		if err != nil {
			logger.ERROR.Printf("error decoding broadcast params: %v", err)
			rep.Error = apiproto.ErrBadRequest
			return rep, nil
		}
		resp := s.api.Broadcast(ctx, cmd)
		if resp.Error != nil {
			rep.Error = resp.Error
		} else {
			if resp.Result != nil {
				replyRes, err = encoder.EncodeBroadcast(resp.Result)
				if err != nil {
					return nil, err
				}
			}
		}
	case "unsubscribe":
		cmd, err := decoder.DecodeUnsubscribe(params)
		if err != nil {
			logger.ERROR.Printf("error decoding unsubscribe params: %v", err)
			rep.Error = apiproto.ErrBadRequest
			return rep, nil
		}
		resp := s.api.Unsubscribe(ctx, cmd)
		if resp.Error != nil {
			rep.Error = resp.Error
		} else {
			if resp.Result != nil {
				replyRes, err = encoder.EncodeUnsubscribe(resp.Result)
				if err != nil {
					return nil, err
				}
			}
		}
	case "disconnect":
		cmd, err := decoder.DecodeDisconnect(params)
		if err != nil {
			logger.ERROR.Printf("error decoding disconnect params: %v", err)
			rep.Error = apiproto.ErrBadRequest
			return rep, nil
		}
		resp := s.api.Disconnect(ctx, cmd)
		if resp.Error != nil {
			rep.Error = resp.Error
		} else {
			if resp.Result != nil {
				replyRes, err = encoder.EncodeDisconnect(resp.Result)
				if err != nil {
					return nil, err
				}
			}
		}
	case "presence":
		cmd, err := decoder.DecodePresence(params)
		if err != nil {
			logger.ERROR.Printf("error decoding presence params: %v", err)
			rep.Error = apiproto.ErrBadRequest
			return rep, nil
		}
		resp := s.api.Presence(ctx, cmd)
		if resp.Error != nil {
			rep.Error = resp.Error
		} else {
			if resp.Result != nil {
				replyRes, err = encoder.EncodePresence(resp.Result)
				if err != nil {
					return nil, err
				}
			}
		}
	case "presence_stats":
		cmd, err := decoder.DecodePresenceStats(params)
		if err != nil {
			logger.ERROR.Printf("error decoding presence_stats params: %v", err)
			rep.Error = apiproto.ErrBadRequest
			return rep, nil
		}
		resp := s.api.PresenceStats(ctx, cmd)
		if resp.Error != nil {
			rep.Error = resp.Error
		} else {
			if resp.Result != nil {
				replyRes, err = encoder.EncodePresenceStats(resp.Result)
				if err != nil {
					return nil, err
				}
			}
		}
	case "history":
		cmd, err := decoder.DecodeHistory(params)
		if err != nil {
			logger.ERROR.Printf("error decoding history params: %v", err)
			rep.Error = apiproto.ErrBadRequest
			return rep, nil
		}
		resp := s.api.History(ctx, cmd)
		if resp.Error != nil {
			rep.Error = resp.Error
		} else {
			if resp.Result != nil {
				replyRes, err = encoder.EncodeHistory(resp.Result)
				if err != nil {
					return nil, err
				}
			}
		}
	case "channels":
		resp := s.api.Channels(ctx, &apiproto.ChannelsRequest{})
		if resp.Error != nil {
			rep.Error = resp.Error
		} else {
			if resp.Result != nil {
				replyRes, err = encoder.EncodeChannels(resp.Result)
				if err != nil {
					return nil, err
				}
			}
		}
	case "info":
		resp := s.api.Info(ctx, &apiproto.InfoRequest{})
		if resp.Error != nil {
			rep.Error = resp.Error
		} else {
			if resp.Result != nil {
				replyRes, err = encoder.EncodeInfo(resp.Result)
				if err != nil {
					return nil, err
				}
			}
		}
	default:
		rep.Error = apiproto.ErrMethodNotFound
	}

	if replyRes != nil {
		rep.Result = replyRes
	}

	return rep, nil
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

	var enc apiproto.Encoding

	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(strings.ToLower(contentType), "application/octet-stream") {
		enc = apiproto.EncodingProtobuf
	} else {
		enc = apiproto.EncodingJSON
	}

	encoder := apiproto.GetReplyEncoder(enc)
	defer apiproto.PutReplyEncoder(enc, encoder)

	decoder := apiproto.GetCommandDecoder(enc, data)
	defer apiproto.PutCommandDecoder(enc, decoder)

	for {
		command, err := decoder.Decode()
		if err != nil {
			if err == io.EOF {
				break
			}
			logger.ERROR.Printf("error decoding API data: %v", err)
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}
		rep, err := s.handleCommand(r.Context(), enc, command)
		if err != nil {
			logger.ERROR.Printf("error handling API command: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		err = encoder.Encode(rep)
		if err != nil {
			logger.ERROR.Printf("error encoding API reply: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
	}
	resp := encoder.Finish()
	w.Header().Set("Content-Type", contentType)
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
