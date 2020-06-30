---
template: overrides/blog_base.html
title: Introduction to Centrifuge library for Go
og_title: Introduction to Centrifuge library for Go
og_image: https://habrastorage.org/files/322/29c/285/32229c2853ef43a1bf7a3752ff5565aa.jpg
og_image_type: image/jpeg
---

# Introduction to Centrifuge library for Go

![gopher](https://habrastorage.org/files/322/29c/285/32229c2853ef43a1bf7a3752ff5565aa.jpg)

In this post we will look at the heart of Centrifugo server – Centrifuge real-time messaging library for Go language. While Centrifugo itself is language-agnostic – the library can only be used if you develop your application backend in Go, obviously.

Centrifuge Library [lives on Github](https://github.com/centrifugal/centrifuge).

The library is based on a strict client-server protocol based on Protobuf schema and solves several problems developer may come across when building complex real-time applications – like scalability (millions of connections), proper connection management, fast reconnect with message recovery, fallback option. In previous post in this blog [Scaling WebSocket in Go](scaling_websocket.md) I described some of things real-time messaging server should deal with. Centrifuge library built on top of Gorilla WebSocket library (and Sockjs-Go polyfill) and gives developer many useful things to quicky run a production-ready feature-full WebSocket server.

Let's make a high level overview of library components building the simple real-time app with Centrifuge. Clients will be able to connect to server over Websocket, subscribe to channel `updates` and receive periodic updates from server.

On server side we will accept all connections and will work as simple PUB/SUB proxy without worrying too much about permissions. Just letting all application guests to connect and subscribe to updates channel. 

Make a new project directory and create `main.go` file inside it.

```go
package main

import (
	"context"
	"log"
	"net/http"

	// Import this library.
	"github.com/centrifugal/centrifuge"
)

func main() {
    // We use default config here as starting point.
    // Default config contains reasonable values for available options.
    cfg := centrifuge.DefaultConfig

    // Node is the core object in Centrifuge library responsible for many
    // useful things. Here we initialize new Node instance and pass config
    // to it.
    node, _ := centrifuge.New(cfg)
    
    // ClientConnected node event handler is a point where you generally
    // create a binding between Centrifuge and your app business logic.
    // Callback function you pass here will be called every time new
    // connection established with server. Inside this callback function
    // you can set various event handlers for connection.
	node.On().ClientConnected(func(ctx context.Context, client *centrifuge.Client) {
        // Set Subscribe Handler to react on every channel subscription
        // attempt initiated by client. Here you can theoretically return
        // an error or disconnect client from server if needed. But now
        // we just accept all subscriptions.
		client.On().Subscribe(func(e centrifuge.SubscribeEvent) centrifuge.SubscribeReply {
			log.Printf("client subscribes on channel %s", e.Channel)
			return centrifuge.SubscribeReply{}
		})
        // Set Disconnect handler to react on client disconnect events.
		client.On().Disconnect(func(e centrifuge.DisconnectEvent) centrifuge.DisconnectReply {
			log.Printf("client disconnected")
			return centrifuge.DisconnectReply{}
		})
        log.Printf("client connected via %s (%s)", transportName, transportEncoding)
    })

    // Run node.
	_ = node.Run()

    wsHandler := centrifuge.NewWebsocketHandler(node, centrifuge.WebsocketConfig{})
	http.Handle("/websocket", wsHandler)
    http.ListenAndServe(":8000", nil)
}
```

As said in code comments `Node` is the core object in Centrifuge library, it must be initialized with `centrifuge.Config`, then you can configure callback function which will be called as soon as new client connects to Node, after that `Run` method must be called to start some internal Node routines and `Engine` (we won't talk about `Engine` in this post but hopefully will look at it in future posts).

Node object allows to publish data to channels. Lets send system message to a channel `chat` every 5 seconds:

```go
go func() {
    for range time.NewTicker(5 * time.Second).C {
        _, _ = node.Publish("chat", []byte(`{"text": "hello from server"}`))
    }
}()
```

Then:

```bash
go run main.go
```

– will start s server.

To connect to this server we need a client. Since Centrifuge library has its own client-server protocol (we will look at protocol features in other posts) we can't just use raw WebSocket libraries to connect. What we should do is to get a library for our frontend environment. There are [several libraries](../libraries/client.md) available at this moment. The interesting thing is that every library works both with Centrifugo server and a server based on Centrifuge library for Go.

In our example we will make a browser chat, so we should take a `centrifuge-js` library.

Create file `index.html` near `main.go` with content:

```html
<!DOCTYPE html>
<html lang="en">
    <head>
        <script type="text/javascript" src="https://rawgit.com/centrifugal/centrifuge-js/master/dist/centrifuge.min.js"></script>
    </head>
    <body>
        <script type="text/javascript">
            const centrifuge = new Centrifuge('ws://localhost:8000/websocket');
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
            // After setting event handlers – initiate actual connection with server.
            centrifuge.connect();
        </script>
    </body>
</html>
```

Client also needs to subscribe on channel:

```javascript
centrifuge.subscribe("chat", function(ctx) {
    drawText(JSON.stringify(ctx.data.text));
});
```

To serve `index.html` add the following to `main.go` near WebSocket handler:

```go
http.Handle("/", http.FileServer(http.Dir("./")))
```

Now let's try to run this example app:

```bash
go run main.go
```

Go to http://localhost:8000.

What? `Disconnected: bad request`. Let's try to investigate the reason. You can inject logging handler function to Centrifuge library and inspect server logs. Let's do this. Define a log handler function:

```go
func handleLog(e centrifuge.LogEntry) {
	log.Printf("%s: %v", e.Message, e.Fields)
}
```

And set some fields of config:

```go
// Centrifuge library exposes logs with different log level. In your app
// you can set special function to handle these log entries in a way you want.
cfg.LogLevel = centrifuge.LogLevelDebug
cfg.LogHandler = handleLog
```

Now if you restart an example and reload browser tab you should see a reason why nothing happens:

```
2020/06/20 20:51:54 client credentials not found: map[client:172314cc-8f8a-4f0e-8752-70c11e8b56db]
2020/06/20 20:51:54 disconnect after handling command: map[client:172314cc-8f8a-4f0e-8752-70c11e8b56db command:id:1  reason:bad request user:]
```

Client should be authenticated. Centrifuge library allows to do authentication over using HTTP middleware mechanism or providing JWT from client side. We will use middleware here. But since we don't have any session backend for now we will just allow anonymous users to connect. 

Write a HTTP middleware like this:

```go
func authMiddleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		newCtx := centrifuge.SetCredentials(ctx, &centrifuge.Credentials{
			UserID:   "",
		})
		r = r.WithContext(newCtx)
		h.ServeHTTP(w, r)
	})
}
```

We have to set `centrifuge.Credentials` instance to request context, Centrifuge then will be able to look at provided Credentials for user ID. Here we know nothing about user IDs – so just return empty string as user ID.

Wrap WebSocket handler with this middleware:

```go
http.Handle("/websocket", authMiddleware(wsHandler))
```

And allow anonymous user access to channels which is disabled by default:

```go
cfg.Anonymous = true
```

Now everything should finally work. The full source code of this example can be found [in this gist](https://gist.github.com/FZambia/2c2d3589b3076d1db59fdc3a60e75914).

Open several browser tabs with http://localhost:8000 and see how updates come to every connected subscriber in real-time:

![screen](https://i.imgur.com/tWqOSrk.png)

This example is only the top of an iceberg. Though it should give you an insight on library API. In future we will also describe other possibilities in this blog.
