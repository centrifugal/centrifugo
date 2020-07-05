[![Join the chat at https://t.me/joinchat/ABFVWBE0AhkyyhREoaboXQ](https://img.shields.io/badge/Telegram-Group-blue.svg)](https://t.me/joinchat/ABFVWBE0AhkyyhREoaboXQ)
[![Build Status](https://travis-ci.org/centrifugal/centrifuge.svg?branch=master)](https://travis-ci.org/centrifugal/centrifuge)
[![Coverage Status](https://coveralls.io/repos/github/centrifugal/centrifuge/badge.svg?branch=master)](https://coveralls.io/github/centrifugal/centrifuge?branch=master)
[![GoDoc](https://godoc.org/github.com/centrifugal/centrifuge?status.svg)](https://godoc.org/github.com/centrifugal/centrifuge)

**This library has no v1 release yet, API still evolves. Use with strict versioning.**

Centrifuge library is a real-time core of [Centrifugo](https://github.com/centrifugal/centrifugo) server. It's also supposed to be a general purpose real-time messaging library for Go programming language. The library is based on a strict client-server protocol based on Protobuf schema and solves several problems developer may come across when building complex real-time applications – like scalability (millions of connections), proper connection management, fast reconnect with message recovery, fallback option.

Library highlights:

* Fast and optimized for low-latency communication with thousands of client connections. See [benchmark](https://centrifugal.github.io/centrifugo/misc/benchmark/)
* WebSocket with JSON or binary Protobuf protocol
* SockJS polyfill library support for browsers where WebSocket not available (JSON only)
* Built-in horizontal scalability with Redis PUB/SUB, consistent Redis sharding, Sentinel and Redis Cluster for HA
* Possibility to register custom PUB/SUB broker, history and presence storage implementations
* Native authentication over HTTP middleware or JWT-based
* Bidirectional asynchronous message communication and RPC calls
* Channel (room) concept to broadcast message to all channel subscribers
* Presence information for channels (show all active clients in channel)
* History information for channels (last messages published into channel)
* Join/leave events for channels (aka client goes online/offline)
* Message recovery mechanism for channels to survive short network disconnects or node restart
* Prometheus instrumentation
* Client libraries for main application environments (see below)

Client libraries:

* [centrifuge-js](https://github.com/centrifugal/centrifuge-js) – for browser, NodeJS and React Native
* [centrifuge-go](https://github.com/centrifugal/centrifuge-go) - for Go language
* [centrifuge-mobile](https://github.com/centrifugal/centrifuge-mobile) - for iOS and Android using `centrifuge-go` as basis and `gomobile` project to create bindings
* [centrifuge-dart](https://github.com/centrifugal/centrifuge-dart) - for Dart and Flutter
* [centrifuge-swift](https://github.com/centrifugal/centrifuge-swift) – for native iOS development
* [centrifuge-java](https://github.com/centrifugal/centrifuge-java) – for native Android development and general Java

See [Godoc](https://godoc.org/github.com/centrifugal/centrifuge) and [examples](https://github.com/centrifugal/centrifuge/tree/master/_examples). You can also consider [Centrifugo server documentation](https://centrifugal.github.io/centrifugo/) as extra doc for this package (because it's built on top of Centrifuge library).

### Installation

To install use:

```bash
go get github.com/centrifugal/centrifuge
```

`go mod` is a recommended way of adding this library to your project dependencies.

### Quick example

Let's take a look on how to build the simplest real-time chat ever with Centrifuge library. Clients will be able to connect to server over Websocket, send a message into channel and this message will be instantly delivered to all active channel subscribers. On server side we will accept all connections and will work as simple PUB/SUB proxy without worrying too much about permissions. In this example we will use Centrifuge Javascript client on a frontend.

Create file `main.go` with the following code:

```go
package main

import (
	"log"
	"net/http"

	// Import this library.
	"github.com/centrifugal/centrifuge"
)

// Function to handle Centrifuge internal logs.
func handleLog(e centrifuge.LogEntry) {
	log.Printf("%s: %v", e.Message, e.Fields)
}

// Authentication middleware. Centrifuge expects Credentials
// with current user ID.
func auth(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		// Put authentication credentials into context. Since we don't have
		// any session backend here – simply set user ID as empty string.
		// Users with empty ID called anonymous users, in real app you should
		// decide whether anonymous users allowed to connect to your server
		// or not. There is also another way to set Credentials - ClientConnecting
		// handler which is called after client sent first command to server
		// called Connect. Without Credentials set connection won't be accepted.
		cred := &centrifuge.Credentials{
			UserID: "",
		}
		newCtx := centrifuge.SetCredentials(ctx, cred)
		r = r.WithContext(newCtx)
		h.ServeHTTP(w, r)
	})
}

func main() {
	// We use default config here as starting point. Default config contains
	// reasonable values for available options.
	cfg := centrifuge.DefaultConfig
	// Centrifuge library exposes logs with different log level. In your app
	// you can set special function to handle these log entries in a way you want.
	cfg.LogLevel = centrifuge.LogLevelDebug
	cfg.LogHandler = handleLog

	// Node is the core object in Centrifuge library responsible for many useful
	// things. Here we initialize new Node instance and pass Config to it.
	node, _ := centrifuge.New(cfg)

	// Set ConnectHandler called when client successfully connected to Node.
	node.On().Connect(func(c *centrifuge.Client) {
		// In our example transport will always be Websocket but it can also be SockJS.
		transportName := c.Transport().Name()
		// In our example clients connect with JSON protocol but it can also be Protobuf.
		transportEncoding := c.Transport().Encoding()
		log.Printf("client connected via %s (%s)", transportName, transportEncoding)
	})

	// Set SubscribeHandler to react on every channel subscription attempt
	// initiated by client. Here you can theoretically return an error or
	// disconnect client from server if needed. But now we just accept
	// all subscriptions to all channels. In real life you may use a more
	// complex permission check here.
	node.On().Subscribe(func(c *centrifuge.Client, e centrifuge.SubscribeEvent) centrifuge.SubscribeReply {
		log.Printf("client subscribes on channel %s", e.Channel)
		return centrifuge.SubscribeReply{}
	})

	// By default, clients can not publish messages into channels. By setting
	// PublishHandler we tell Centrifuge that publish from client side is possible.
	// Now each time client calls publish method this handler will be called and
	// you have a possibility to validate publication request before message will
	// be published into channel and reach active subscribers. In our simple chat
	// app we allow everyone to publish into any channel.
	node.On().Publish(func(c *centrifuge.Client, e centrifuge.PublishEvent) centrifuge.PublishReply {
		log.Printf("client publishes into channel %s: %s", e.Channel, string(e.Data))
		return centrifuge.PublishReply{}
	})

	// Set Disconnect handler to react on client disconnect events.
	node.On().Disconnect(func(c *centrifuge.Client, e centrifuge.DisconnectEvent) {
		log.Printf("client disconnected")
	})

	// Run node. This method does not block.
	if err := node.Run(); err != nil {
		panic(err)
	}

	// Now configure http routes.

	// Serve Websocket connections using WebsocketHandler.
	wsHandler := centrifuge.NewWebsocketHandler(node, centrifuge.WebsocketConfig{})
	http.Handle("/connection/websocket", auth(wsHandler))

	// The second route is for serving index.html file.
	http.Handle("/", http.FileServer(http.Dir("./")))

	log.Printf("Starting server, visit http://localhost:8000")
	if err := http.ListenAndServe(":8000", nil); err != nil {
		panic(err)
	}
}
```

Also create file `index.html` near `main.go` with content:

```html
<!DOCTYPE html>
<html lang="en">
    <head>
        <meta charset="utf-8">
        <script type="text/javascript" src="https://rawgit.com/centrifugal/centrifuge-js/master/dist/centrifuge.min.js"></script>
        <title>Centrifuge library chat example</title>
    </head>
    <body>
        <input type="text" id="input" />
        <script type="text/javascript">
            // Create Centrifuge object with Websocket endpoint address set in main.go
            const centrifuge = new Centrifuge('ws://localhost:8000/connection/websocket');
            function drawText(text) {
                const div = document.createElement('div');
                div.innerHTML = text + '<br>';
                document.body.appendChild(div);
            }
            centrifuge.on('connect', function(ctx){
                drawText('Connected over ' + ctx.transport);
            });
            centrifuge.on('disconnect', function(ctx){
                drawText('Disconnected: ' + ctx.reason);
            });
            const sub = centrifuge.subscribe("chat", function(ctx) {
                drawText(JSON.stringify(ctx.data));
            })
            const input = document.getElementById("input");
            input.addEventListener('keyup', function(e) {
                if (e.keyCode === 13) {
                    sub.publish(this.value);
                    input.value = '';
                }
            });
            // After setting event handlers – initiate actual connection with server.
            centrifuge.connect();
        </script>
    </body>
</html>
```

Then run as usual:

```bash
go run main.go
```

Open several browser tabs with http://localhost:8000 and see chat in action.

This example is only the top of an iceberg. Though it should give you an insight on library API. 

Keep in mind that Centrifuge library is not a framework to build chat apps. It's a general purpose real-time transport for your messages with some helpful primitives. You can build many kinds of real-time apps on top of this library including chats but depending on application you may need to write business logic yourself.
