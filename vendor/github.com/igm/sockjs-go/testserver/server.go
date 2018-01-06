package main

import (
	"log"
	"net/http"
	"strings"

	"github.com/igm/sockjs-go/sockjs"
)

type testHandler struct {
	prefix  string
	handler http.Handler
}

func newSockjsHandler(prefix string, options sockjs.Options, fn func(sockjs.Session)) *testHandler {
	return &testHandler{prefix, sockjs.NewHandler(prefix, options, fn)}
}

type testHandlers []*testHandler

func main() {
	// prepare various options for tests
	echoOptions := sockjs.DefaultOptions
	echoOptions.ResponseLimit = 4096
	echoOptions.RawWebsocket = true

	disabledWebsocketOptions := sockjs.DefaultOptions
	disabledWebsocketOptions.Websocket = false

	cookieNeededOptions := sockjs.DefaultOptions
	cookieNeededOptions.JSessionID = sockjs.DefaultJSessionID

	closeOptions := sockjs.DefaultOptions
	closeOptions.RawWebsocket = true
	// register various test handlers
	var handlers = []*testHandler{
		newSockjsHandler("/echo", echoOptions, echoHandler),
		newSockjsHandler("/cookie_needed_echo", cookieNeededOptions, echoHandler),
		newSockjsHandler("/close", closeOptions, closeHandler),
		newSockjsHandler("/disabled_websocket_echo", disabledWebsocketOptions, echoHandler),
	}
	log.Fatal(http.ListenAndServe(":8081", testHandlers(handlers)))
}

func (t testHandlers) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	for _, handler := range t {
		if strings.HasPrefix(req.URL.Path, handler.prefix) {
			handler.handler.ServeHTTP(rw, req)
			return
		}
	}
	http.NotFound(rw, req)
}

func closeHandler(conn sockjs.Session) { conn.Close(3000, "Go away!") }
func echoHandler(conn sockjs.Session) {
	log.Println("New connection created")
	for {
		if msg, err := conn.Recv(); err != nil {
			break
		} else {
			if err := conn.Send(msg); err != nil {
				break
			}
		}
	}
	log.Println("Sessionection closed")
}
