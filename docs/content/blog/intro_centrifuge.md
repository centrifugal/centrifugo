---
template: overrides/blog_base.html
title: An introduction to Centrifuge – real-time messaging with Go
description: Introducing Centrifuge real-time messaging library for Go language
og_title: An introduction to Centrifuge – real-time messaging with Go
og_image: https://i.imgur.com/W1PeoJL.jpg
og_image_width: 1280
og_image_height: 640
---

# An introduction to Centrifuge – real-time messaging with Go

![Centrifuge](https://i.imgur.com/W1PeoJL.jpg)

Today is 4th January – New Year holidays in progress. I am sitting inside a chair with a ticking clock on the wall and a cup of coffee on the table. The blue winter semidarkness outside the window makes the room magnificent. I spent enough time coding last year – not sure I'll find a better time to write about the open-source library I was working on and made big progress with.

Maybe you've already heard about it – it's called [Centrifuge](https://github.com/centrifugal/centrifuge). This is a real-time messaging library for the Go language. OK, I think this is a mistake to call Centrifuge a library – `framework` suits better. As a Gopher, I don't like frameworks a lot. As soon as you start using a framework it dictates you pretty much how things should be done. I suppose that's why I still avoid describing Centrifuge with the right word. But the code is written – so I can only move on with it :)

Centrifuge can do many things for you. Here I'll try to introduce Centrifuge and its possibilities.

This post is going to be pretty long (looks like I am a huge fan of long posts) – so make sure you also have a drink and let's go! 

## How it's all started

I wrote several blog posts before ([for example this one](https://medium.com/@fzambia/four-years-in-centrifuge-ce7a94e8b1a8) – sorry, it's on Medium) about an original motivation of [Centrifugo](https://github.com/centrifugal/centrifugo) server.

!!!danger
    Centrifugo server is not the same as Centrifuge library for Go. It's a full-featured project built on top of Centrifuge library. Naming can be confusing, but it's not too hard once you spend some time with ecosystem.

In short – Centrifugo was implemented to help traditional web frameworks dealing with many persistent connections (like WebSocket or SockJS HTTP transports). So frameworks like Django or Ruby on Rails, or frameworks from the PHP world could be used on a backend but still provide real-time messaging features like chats, multiplayer browser games, etc for users. With a little help from Centrifugo.

Now there are cases when Centrifugo server used in conjunction even with a backend written in Go. While Go mostly has no problems dealing with many concurrent connections – Centrifugo provides some features beyond simple message passing between a client and a server. That makes it useful, especially since design is pretty non-obtrusive and fits well microservices world. Centrifugo is used in some well-known projects (like ManyChat, Yoola.io, Spot.im, Badoo etc).

At the end of 2018, I released Centrifugo v2 based on a real-time messaging library for Go language – Centrifuge – the subject of this post.

It was a pretty hard experience to decouple Centrifuge out of the monolithic Centrifugo server – I was unable to make all the things right immediately, so Centrifuge library API went through several iterations where I introduced backward-incompatible changes. All those changes targeted to make Centrifuge a more generic tool and remove opinionated or limiting parts.

## So what is Centrifuge?

This is ... well, a framework to build real-time messaging applications with Go language. If you ever heard about [socket.io](https://socket.io) – then you can think about Centrifuge as an analogue. I think the most popular applications these days are chats of different forms, but I want to emphasize that Centrifuge is not a framework to build chats – it's a generic instrument that can be used to create different sorts of real-time applications – real-time charts, multiplayer games.

The obvious choice for real-time messaging transport to achieve fast and cross-platform bidirectional communication these days is WebSocket. Especially if you are targeting a browser environment. You mostly don't need to use WebSocket HTTP polyfills in 2021 (though there are still corner cases so Centrifuge supports [SockJS](https://github.com/sockjs/sockjs-client) polyfill).

Centrifuge has its own custom protocol on top of plain WebSocket or SockJS frames. 

The reason why Centrifuge has its own protocol on top of underlying transport is that it provides several useful primitives to build real-time applications. The protocol [described as strict Protobuf schema](https://github.com/centrifugal/protocol/blob/master/definitions/client.proto). It's possible to pass JSON or binary Protobuf-encoded data over the wire with Centrifuge.

!!!note
    GRPC is very handy these days too (and can be used in a browser with a help of additional proxies), some developers prefer using it for real-time messaging apps – especially when one-way communication needed. It can be a bit better from integration perspective but more resource-consuming on server side and a bit trickier to deploy.

!!!note
    Take a look at [WebTransport](https://w3c.github.io/webtransport/) – a brand-new spec for web browsers to allow fast communication between a client and a server on top of QUIC – it may be a good alternative to WebSocket in the future. This in a draft status at the moment, but it's [already possible to play with in Chrome](https://centrifugal.github.io/centrifugo/blog/quic_web_transport/).

Own protocol is one of the things that prove the framework status of Centrifuge. This dictates certain limits (for example, you can't just use an alternative message encoding) and makes developers use custom client connectors on a front-end side to communicate with a Centrifuge-based server (see more about connectors in ecosystem part).

But protocol solves many practical tasks – and here we are going to look at real-time features it provides for a developer.

## Centrifuge Node

To start working with Centrifuge you need to start Centrifuge server Node. Node is a core of Centrifuge – it has many useful methods – set event handlers, publish messages to channels, etc. We will look at some events and channels concept very soon.

Also, Node abstracts away scalability aspects, so you don't need to think about how to scale WebSocket connections over different server instances and still have a way to deliver published messages to interested clients.

For now, let's start a single instance of Node that will serve connections for us:

```go
node, err := centrifuge.New(centrifuge.DefaultConfig)
if err != nil {
    log.Fatal(err)
}

if err := node.Run(); err != nil {
    log.Fatal(err)
}
```

It's also required to serve a WebSocket handler – this is possible just by registering `centrifuge.WebsocketHandler` in HTTP mux:

```go
wsHandler := centrifuge.NewWebsocketHandler(node, centrifuge.WebsocketConfig{})
http.Handle("/connection/websocket", wsHandler)
```

Now it's possible to connect to a server (using Centrifuge connector for a browser called `centrifuge-js`):

```javascript
const centrifuge = new Centrifuge('ws://localhost:8000/connection/websocket');
centrifuge.connect();
```

But connection will be rejected since we also need to provide authentication details – Centrifuge expects explicitly provided connection `Credentials` to accept connection.

## Authentication

Let's look at how we can tell Centrifuge details about connected user identity, so it could accept an incoming connection.

There are two main ways to authenticate client connection in Centrifuge.

The first one is over the native middleware mechanism. It's possible to wrap `centrifuge.WebsocketHandler` or `centrifuge.SockjsHandler` with middleware that checks user authentication and tells Centrifuge current user ID over `context.Context`:

```go
func auth(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cred := &centrifuge.Credentials{
			UserID: "42",
		}
		newCtx := centrifuge.SetCredentials(r.Context(), cred)
		r = r.WithContext(newCtx)
		h.ServeHTTP(w, r)
	})
}
```

So WebsocketHandler can be registered this way (note that a handler now wrapped by auth middleware):

```go
wsHandler := centrifuge.NewWebsocketHandler(node, centrifuge.WebsocketConfig{})
http.Handle("/connection/websocket", auth(wsHandler))
```

Another authentication way is a bit more generic – developers can authenticate connection based on custom token sent from a client inside first WebSocket/SockJS frame. This is called `connect` frame in terms of Centrifuge protocol. Any string token can be set – this opens a way to use JWT, Paceto, and any other kind of authentication tokens. For example [see an authenticaton with JWT](https://github.com/centrifugal/centrifuge/tree/master/_examples/jwt_token).

!!!note
    BTW it's also possible to pass any information from client side with a first connect message from client to server and return custom information about server state to a client. But this is out of post scope.

Nothing prevents you to [integrate Centrifuge with OAuth2](https://github.com/centrifugal/centrifuge/tree/master/_examples/chat_oauth2) or another framework session mechanism – [like Gin for example](https://github.com/centrifugal/centrifuge/tree/master/_examples/chat_oauth2).

## Channel subscriptions

As soon as the client connected and successfully authenticated it can subscribe to channels. Channel (room or topic in other systems) is a lightweight and ephemeral entity in Centrifuge. Channel can have different features (we will look at some channel features below). Channels are created automatically as soon as the first subscriber joins and destroyed as soon as the last subscriber left.

The application can have many real-time features – even on one app screen. So sometimes client subscribes to several channels – each related to a specific real-time feature (for example one channel for chat updates, one channel likes notification stream, etc).

Channel is just an ASCII string. A developer is responsible to find the best channel naming convention suitable for an application. Channel naming convention is an important aspect since in many cases developers want to authorize subscription to a channel on the server side – so only authorized users could listen to specific channel updates.

Let's look at a basic subscription example on the client-side:

```javascript
centrifuge.subscribe('example', function(msgCtx) {
    console.log(msgCtx)
})
```

And on the server-side, you need to define the subscribe event handler. If the subscribe event handler is not set then the connection won't be able to subscribe to channels at all. Subscribe handler is where a developer may want to check permissions of the current connection to read channel updates. Here is a basic example of a subscribe handler that simply allows subscriptions to channel `example` for all authenticated connections and reject subscriptions to all other channels:

```go
node.OnConnect(func(client *centrifuge.Client) {
    client.OnSubscribe(func(e centrifuge.SubscribeEvent, cb centrifuge.SubscribeCallback) {
        if e.Channel != "example" {
            cb(centrifuge.SubscribeReply{}, centrifuge.ErrorPermissionDenied)
            return
        }
        cb(centrifuge.SubscribeReply{}, nil)
    })
})
```

You may already notice a callback style of reacting to connection related things. While not being very idiomatic for Go it's very practical actually. The reason why we use callback style inside client event handlers is that it gives a developer possibility to control operation concurrency (i.e. process sth in separate goroutines or goroutine pool) and still control the order of events. See [an example](https://github.com/centrifugal/centrifuge/tree/master/_examples/concurrency) that demonstrates concurrency control in action.

Now if some event published to a channel:

```go
// Here is how we can publish data to a channel.
node.Publish("example", []byte(`{"input": "hello"}`))
```

– data will be delivered to a subscribed client, and message will be printed to Javascript console. PUB/SUB in its usual form.

!!!note
    Though Centrifuge protocol based on Protobuf schema in example above we published a JSON message into a channel. By default, we can only send JSON to connections since default protocol format is JSON. But we can switch to Protobuf-based binary protocol by connecting to `ws://localhost:8000/connection/websocket?format=protobuf` endpoint – then it's possible to send binary data to clients.

## Async message passing

While Centrifuge mostly shines when you need channel semantics it's also possible to send any data to connection directly – to achieve bidirectional asynchronous communication, just what a native WebSocket provides.

To send a message to a server one can use the `send` method on the client-side:

```javascript
centrifuge.send({"input": "hello"});
```

On the server-side data will be available inside a message handler:

```go
client.OnMessage(func(e centrifuge.MessageEvent) {
    log.Printf("message from client: %s", e.Data)
})
```

And vice-versa, to send data to a client use `Send` method of `centrifuge.Client`:

```go
client.Send([]byte(`{"input": "hello"}`))
```

To listen to it on the client-side:

```javascript
centrifuge.on('message', function(data) {
    console.log(data);
});
```

## RPC

RPC is a primitive for sending a request from a client to a server and waiting for a response (in this case all communication still happens via asynchronous message passing internally, but Centrifuge takes care of matching response data to request previously sent).

On client side it's as simple as:

```javascript
const resp = await centrifuge.namedRPC('my_method', {});
```

On server side RPC event handler should be set to make calls available:

```go
client.OnRPC(func(e centrifuge.RPCEvent, cb centrifuge.RPCCallback) {
    if e.Method == "my_method" {
        cb(centrifuge.RPCReply{Data: []byte(`{"result": "42"}`)}, nil)
        return
    }
    cb(centrifuge.RPCReply{}, centrifuge.ErrorMethodNotFound)
})
```

Note, that it's possible to pass the name of RPC and depending on it and custom request params return different results to a client – just like a regular HTTP request but over asynchronous WebSocket (or SockJS) connection.

## Server-side subscriptions

In many cases, a client is a source of knowledge which channels it wants to subscribe to on a specific application screen. But sometimes you want to control subscriptions to channels on a server-side. This is also possible in Centrifuge.

It's possible to provide a slice of channels to subscribe connection to at the moment of connection establishment phase:

```go
node.OnConnecting(func(ctx context.Context, e centrifuge.ConnectEvent) (centrifuge.ConnectReply, error) {
    return centrifuge.ConnectReply{
        Subscriptions: map[string]centrifuge.SubscribeOptions{
            "example": {},
        },
    }, nil
})
```

Note, that `OnConnecting` does not follow callback-style – this is because it can only happen once at the start of each connection – so there is no need to control operation concurrency.

In this case on the client-side you will have access to messages published to channels by listening to `on('publish')` event:

```javascript
centrifuge.on('publish', function(msgCtx) {
    console.log(msgCtx);
});
```

Also, `centrifuge.Client` has `Subscribe` and `Unsubscribe` methods so it's possible to subscribe/unsubscribe client to/from channel somewhere in the middle of its long WebSocket session.

## Windowed history in channel

Every time a message published to a channel it's possible to provide custom history options. For example:

```go
node.Publish(
    "example",
    []byte(`{"input": "hello"}`),
    centrifuge.WithHistory(300, time.Minute),
)
```

In this case, Centrifuge will maintain a windowed Publication cache for a channel - or in other words, maintain a publication stream. This stream will have time retention (one minute in the example above) and the maximum size will be limited to the value provided during Publish (300 in the example above).

Every message inside a history stream has an incremental `offset` field. Also, a stream has a field called `epoch` – this is a unique identifier of stream generation - thus client will have a possibility to distinguish situations where a stream is completely removed and there is no guarantee that no messages have been lost in between even if offset looks fine.

Client protocol provides a possibility to paginate over a stream from a certain position with a limit:

```javascript
const streamPosition = {'offset': 0, epoch: 'xyz'} 
resp = await sub.history({since: streamPosition, limit: 10});
```

Iteration over history stream is only available with Javascript client at the moment and only works for subscription object - i.e. client must be subscribed to a channel to iterate over its history. There are plans to extend this further in the future – for example allow history iteration on a client-side without being subscribed.

Also, Centrifuge has an automatic message recovery feature. Automatic recovery is very useful in scenarios when tons of persistent connections start reconnecting at once. I already described why this is useful in one of my previous posts about Websocket scalability. In short – since WebSocket connections are stateful then at the moment of mass reconnect they can create a very big spike in load on your main application database. Such mass reconnects are a usual thing in practice - for example when you reload your load balancers or re-deploying the Websocket server (new code version).

Of course, recovery can also be useful for regular short network disconnects - when a user travels in the subway for example. But you always need a way to load an actual state from the main application database in case of an unsuccessful recovery.

To enable automatic recovery you can provide the `Recover` flag in subscribe options:

```go
client.OnSubscribe(func(e centrifuge.SubscribeEvent, cb centrifuge.SubscribeCallback) {
    cb(centrifuge.SubscribeReply{
        Options: centrifuge.SubscribeOptions{
            Recover:   true,
        },
    }, nil)
})
```

Obviously, recovery will work only for channels where history stream maintained. The limitation in recovery is that all missed publications sent to client in one protocol frame – pagination is not supported during recovery process. This means that recovery is mostly effective for not too long offline time without tons of missed messages.

## Presence and presence stats

Another cool thing Centrifuge exposes to developers is presence information for channels. Presence information contains a list of active channel subscribers. This is useful to show the online status of players in a game for example.

Also, it's possible to turn on Join/Leave message feature inside channels: so each time connection subscribes to a channel all channel subscribers receive a Join message with client information (client ID, user ID). As soon as the client unsubscribes Leave message is sent to remaining channel subscribers with information who left a channel.

Here is how to enable both presence and join/leave features for a subscription to channel:

```go
client.OnSubscribe(func(e centrifuge.SubscribeEvent, cb centrifuge.SubscribeCallback) {
    cb(centrifuge.SubscribeReply{
        Options: centrifuge.SubscribeOptions{
            Presence:   true,
            JoinLeave:  true,
        },
    }, nil)
})
```

On a client-side then it's possible to call for the presence and setting event handler for join/leave messages. 

The important thing to be aware of when using Join/Leave messages is that this feature can dramatically increase CPU utilization and overall traffic in channels with a big number of active subscribers – since on every client connect/disconnect event such Join or Leave message must be sent to all subscribers. The advice here – avoid using Join/Leave messages or be ready to scale (Join/Leave messages scale well when adding more Centrifuge Nodes – more about scalability below).

One more thing to remember is that presence information can also be pretty expensive to request in channels with many active subscribers – since it returns information about all connections – thus payload in response can be large. To help a bit with this situation Centrifuge has a presence stats client API method. Presence stats only contain two counters: the number of active connections in the channel and amount of unique users in the channel.

If you still need to somehow process presence in rooms with a massive number of active subscribers – then I think you better do it in near real-time - for example with fast OLAP like [ClickHouse](https://clickhouse.tech/).

## Scalability aspects

To be fair it's not too hard to implement most of the features above inside one in-memory process. Yes, it takes time, but the code is mostly straightforward. When it comes to scalability things tend to be a bit harder.

Centrifuge designed with the idea in mind that one machine is not enough to handle all application WebSocket connections. Connections should scale over application backend instances, and it should be simple to add more application nodes when the amount of users (connections) grows.

Centrifuge abstracts scalability over the `Node` instance and two interfaces: `Broker` interface and `PresenceManager` interface.

A broker is responsible for PUB/SUB and streaming semantics:

```go
type Broker interface {
	Run(BrokerEventHandler) error
	Subscribe(ch string) error
	Unsubscribe(ch string) error
	Publish(ch string, data []byte, opts PublishOptions) (StreamPosition, error)
	PublishJoin(ch string, info *ClientInfo) error
	PublishLeave(ch string, info *ClientInfo) error
	PublishControl(data []byte, nodeID string) error
	History(ch string, filter HistoryFilter) ([]*Publication, StreamPosition, error)
	RemoveHistory(ch string) error
}
```

See [full version with comments](https://github.com/centrifugal/centrifuge/blob/v0.14.2/engine.go#L98) in source code.

Every Centrifuge Node subscribes to channels via a broker. This provides a possibility to scale connections over many node instances – published messages will flow only to nodes with active channel subscribers.

It's and important thing to combine PUB/SUB with history inside a Broker implementation to achieve an atomicity of saving message into history stream and publishing it to PUB/SUB with generated offset.

PresenceManager is responsible for presence information management:

```go
type PresenceManager interface {
	Presence(ch string) (map[string]*ClientInfo, error)
	PresenceStats(ch string) (PresenceStats, error)
	AddPresence(ch string, clientID string, info *ClientInfo, expire time.Duration) error
	RemovePresence(ch string, clientID string) error
}
```

[Full code with comments](https://github.com/centrifugal/centrifuge/blob/v0.14.2/engine.go#L150).

`Broker` and `PresenceManager` together form an `Engine` interface:

```go
type Engine interface {
	Broker
	PresenceManager
}
```

By default, Centrifuge uses `MemoryEngine` that does not use any external services but limits developers to using only one Centrifuge Node (i.e. one server instance). Memory Engine is fast and can be suitable for some scenarios - even in production (with configured backup instance) – but as soon as the number of connections grows – you may need to load balance connections to different server instances. Here comes the Redis Engine.

Redis Engine utilizes Redis for Broker and PresenceManager parts.

History cache saved to Redis STREAM or Redis LIST data structures. For presence, Centrifuge uses a combination of HASH and ZSET structures.

Centrifuge tries to fully utilize the connection between Node and Redis by using pipelining where possible and smart batching technique. All operations done in a single RTT with the help of Lua scripts loaded automatically to Redis on engine start.

Redis is pretty fast and will allow your app to scale to some limits. When Redis starts being a bottleneck it's possible to shard data over different Redis instances. Client-side consistent sharding is built-in in Centrifuge and allows scaling further.

It's also possible to achieve Redis's high availability with built-in Sentinel support. Redis Cluster supported too. So Redis Engine covers many options to communicate with Redis deployed in different ways.

At Avito we served about 800k active connections in the messenger app with ease using a slightly adapted Centrifuge Redis Engine, so an approach proved to be working for rather big applications. We will look at some more concrete numbers below in the performance section.

Both `Broker` and `PresenceManager` are pluggable, so it's possible to replace them with alternative implementations. Examples show [how to use Nats server](https://github.com/centrifugal/centrifuge/tree/master/_examples/custom_broker_nats) for at most once only PUB/SUB together with Centrifuge. Also, we have [an example of full-featured Engine for Tarantool database](https://github.com/centrifugal/centrifuge/tree/master/_examples/custom_engine_tarantool) – Tarantool Engine shows even better throughput for history and presence operations than Redis-based Engine (up to 10x for some ops).

## Order and delivery properties

Since Centrifuge is a messaging system I also want to describe its order and message delivery guarantees.

Message ordering in channels supported. As soon as you publish messages into channels one after another of course.

Message delivery model is at most once by default. This is mostly comes from PUB/SUB model – message can be dropped on Centrifuge level if subscriber is offline or simply on broker level – since Redis PUB/SUB also works with at most once guarantee.

Though if you maintain history stream inside a channel then things become a bit different. In this case you can tell Centrifuge to check client position inside stream. Since every publication has a unique incremental offset Centrifuge can track that client has correct offset inside a channel stream. If Centrifuge detects any missed messages it disconnects a client with special code – thus make it reconnect and recover messages from history stream. Since a message first saved to history stream and then published to PUB/SUB inside broker these mechanisms allow achieving at least once message delivery guarantee.

![What happens on publish](https://i.imgur.com/PLb9xS5.jpg)

Even if stream completely expired or dropped from broker memory Centrifuge will give a client a tip that messages could be lost – so client has a chance to restore state from a main application database.

## Ecosystem

Here I want to be fair with my readers – Centrifuge is not ideal. This is a project maintained mostly by one person at the moment with all consequences. This hits an ecosystem a lot, can make some design choices opinionated or non-optimal.

I mentioned in the first post that Centrifuge built on top of the custom protocol. The protocol is based on a strict Protobuf schema, works with JSON and binary data transfer, supports many features. But – this means that to connect to the Centrifuge-based server developers have to use custom connectors that can speak with Centrifuge over its custom protocol.

The difficulty here is that protocol is asynchronous. Asynchronous protocols are harder to implement than synchronous ones. Multiplexing frames allows achieving good performance and fully utilize a single connection – but it hurts simplicity.

At this moment Centrifuge has client connectors for:

* [centrifuge-js](https://github.com/centrifugal/centrifuge-js) - Javascript client for a browser, NodeJS and React Native
* [centrifuge-go](https://github.com/centrifugal/centrifuge-go) - for Go language
* [centrifuge-mobile](https://github.com/centrifugal/centrifuge-mobile) - for mobile development based on centrifuge-go and [gomobile](https://github.com/golang/mobile) project
* [centrifuge-swift](https://github.com/centrifugal/centrifuge-swift) - for iOS native development
* [centrifuge-java](https://github.com/centrifugal/centrifuge-java) - for Android native development and general Java
* [centrifuge-dart](https://github.com/centrifugal/centrifuge-dart) - for Dart and Flutter

Not all clients support all protocol features. Another drawback is that all clients do not have a persistent maintainer – I mostly maintain everything myself. Connectors can have non-idiomatic and pretty dumb code since I had no previous experience with mobile development, they lack proper tests and documentation. This is unfortunate.

The good thing is that all connectors feel very similar, I am quickly releasing new versions when someone sends a pull request with improvements or bug fixes. So all connectors are alive.

I maintain a feature matrix in connectors to let users understand what's supported. Actually feature support is pretty nice throughout all these connectors - there are only several things missing and not so much work required to make all connectors full-featured. But I really need help here.

It will be a big mistake to not mention Centrifugo as a big plus for Centrifuge library ecosystem. Centrifugo is a server deployed in many projects throughout the world. Many features of Centrifuge library and its connectors have already been tested by Centrifugo users.

One more thing to mention is that Centrifuge does not have v1 release. It still evolves – I believe that the most dramatic changes have already been made and backward compatibility issues will be minimal in the next releases – but can't say for sure.

## Performance

I made a test stand in Kubernetes with one million connections.

I can't call this a proper benchmark – since in a benchmark your main goal is to destroy a system, in my test I just achieved some reasonable numbers on limited hardware. These numbers should give a good insight into a possible throughput, latency, and estimate hardware requirements (at least approximately).

Connections landed on different server pods, 5 Redis instances have been used to scale connections between pods.

The detailed test stand description [can be found in Centrifugo documentation](https://centrifugal.github.io/centrifugo/misc/benchmark/).

![Benchmark](../images/benchmark.gif)

Some quick conclusions are:

* One connection costs about 30kb of RAM
* Redis broker CPU utilization increases linearly with more messages traveling around
* 1 million connections with 500k **delivered** messages per second with 200ms delivery latency in 99 percentile can be served with hardware amount equal to one modern physical server machine. The possible amount of messages can vary a lot depending on the number of channel subscribers though.

## Limitations

Centrifuge does not allow subscribing on the same channel twice inside a single connection. It's not simple to add due to design decisions made – though there was no single user report about this in seven years of Centrifugo/Centrifuge history.

Centrifuge does not support wildcard subscriptions. Not only because I never needed this myself but also due to some design choices made – so be aware of this.

SockJS fallback does not support binary data - only JSON. If you want to use binary in your application then you can only use WebSocket with Centrifuge - there is no built-in fallback transport in this case.

SockJS also requires sticky session support from your load balancer to emulate a stateful bidirectional connection with its HTTP fallback transports. Ideally, Centrifuge will go away from SockJS at some point, maybe when WebTransport becomes mature so users will have a choice between WebTransport or WebSocket.

Websocket permessage-deflate compression supported (thanks to Gorilla WebSocket), but it can be pretty expensive in terms of CPU utilization and memory usage.

As said above you cannot only rely on Centrifuge for state recovery – it's still required to have a way to fully load application state from the main database.

Also, I am not very happy with current error and disconnect handling throughout the connector ecosystem – this can be improved though.

## Examples

I am adding examples to [_examples](https://github.com/centrifugal/centrifuge/tree/master/_examples) folder of Centrifuge repo. These examples completely cover Centrifuge API - including things not mentioned here.

Check out the [tips & tricks](https://github.com/centrifugal/centrifuge#tips-and-tricks) section of README – it contains some additional insights about an implementation.

## Conclusion

I think [Centrifuge](https://github.com/centrifugal/centrifuge) could be a nice alternative to [socket.io](https://socket.io) - with a better performance, main server implementation in Go language, and even more builtin features to build real-time apps.

Centrifuge ecosystem definitely needs more work, especially in client connectors area, tutorials, community, stabilizing API, etc.

Centrifuge fits pretty well proprietary application development where time matters and deadlines are close, so developers tend to choose a ready solution instead of writing their own. I believe Centrifuge can be a great time saver here.

For Centrifugo server users Centrifuge package provides a way to write a more flexible server code adapted for business requirements but still use the same real-time core and have the same protocol features.
