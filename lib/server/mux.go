package server

import (
	"net/http"
	"net/http/pprof"
	"strings"

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
	// HandlerPrometheus enables Prometheus handler.
	HandlerPrometheus
)

var handlerText = map[HandlerFlag]string{
	HandlerWebsocket:  "websocket",
	HandlerSockJS:     "SockJS",
	HandlerAPI:        "API",
	HandlerAdmin:      "admin",
	HandlerDebug:      "debug",
	HandlerPrometheus: "prometheus",
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

// Mux returns a mux including set of default handlers for Centrifugo server.
func Mux(s *HTTPServer, muxOpts MuxOptions) *http.ServeMux {

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

	// if flags&HandlerWebsocket != 0 {
	// 	// register Websocket connection endpoint.
	// 	mux.Handle(prefix+"/connection/websocket", s.LogRequest(s.WrapShutdown(http.HandlerFunc(s.websocketHandler))))
	// }

	// if flags&HandlerSockJS != 0 {
	// 	// register SockJS connection endpoints.
	// 	sjsh := newSockJSHandler(s, path.Join(prefix, "/connection/sockjs"), muxOpts.SockjsOptions)
	// 	mux.Handle(path.Join(prefix, "/connection/sockjs")+"/", s.log(s.wrapShutdown(sjsh)))
	// }

	// if flags&HandlerAPI != 0 {
	// 	// register HTTP API endpoint.
	// 	mux.Handle(prefix+"/api", s.log(s.apiKeyAuth(s.wrapShutdown(http.HandlerFunc(s.apiHandler)))))
	// }

	// if flags&HandlerPrometheus != 0 {
	// 	// register Prometheus metrics export endpoint.
	// 	mux.Handle(prefix+"/metrics", s.log(s.wrapShutdown(promhttp.Handler())))
	// }

	// if flags&HandlerAdmin != 0 {
	// 	// register admin web interface API endpoints.
	// 	mux.Handle(prefix+"/admin/auth", s.log(http.HandlerFunc(s.authHandler)))
	// 	mux.Handle(prefix+"/admin/api", s.log(s.adminSecureTokenAuth(s.wrapShutdown(http.HandlerFunc(s.apiHandler)))))
	// 	// serve admin single-page web application.
	// 	if webPath != "" {
	// 		webPrefix := prefix + "/"
	// 		mux.Handle(webPrefix, http.StripPrefix(webPrefix, http.FileServer(http.Dir(webPath))))
	// 	} else if webFS != nil {
	// 		webPrefix := prefix + "/"
	// 		mux.Handle(webPrefix, http.StripPrefix(webPrefix, http.FileServer(webFS)))
	// 	}
	// }

	return mux
}
