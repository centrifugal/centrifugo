---
template: overrides/blog_base.html
title: Scaling WebSocket in Go
og_title: Scaling WebSocket in Go
og_image: https://i.imgur.com/QOJ1M9a.png
---

# Scaling WebSocket in Go

![gopher-broker](https://i.imgur.com/QOJ1M9a.png)

I believe that in 2020 WebSocket is still an entertaining technology that is not well-known and understood like HTTP yet. In this blog post I'd like to tell about state of WebSocket in Go language ecosystem, and a way we could write a scalable WebSocket servers. Most advices here are generic enough and can be easily approximated to other programming languages. So if you are interested in WebSocket and messaging technologies - keep on reading.

Speaking about scalable servers that work with many persistent WebSocket connections – I found several important tasks such a server should be able to do:

* Maintain a large number of active connections
* Send a large number of messages to clients
* Support WebSocket fallback to work with every client
* Authenticate incoming connections and invalidate connections
* Survive massive reconnect of all clients without loosing messages

In this post we will look at some tips which relate to these points. If you don't know what WebSocket is here are some useful links to read:

* https://hpbn.co/websocket/ – a wonderful chapter of great book by Ilya Grigorik
* https://lucumr.pocoo.org/2012/9/24/websockets-101/ – valuable thoughts abour WebSocket from Armin Ronacher

As soon as you know WebSocket basics – we can proceed.

![one_hour_scale](https://i.imgur.com/4lYjJSP.png)

## WebSocket libraries

In Go language ecosystem we currently have several libraries that can be used as a building block for WebSocket server. Package [golang.org/x/net/websocket](https://godoc.org/golang.org/x/net/websocket) is considered **deprecated**.

The de-facto choice in the community is [github.com/gorilla/websocket](https://github.com/gorilla/websocket) library. Made by Gary Burd (who also gave us an awesome Redigo package) – it's widely used, performs well, has a very good API – so in most cases you should go with it. Some think that library is not actively maintained at moment – but this is not quite true, it implements full WebSocket RFC, so actually it can be considered done.

Recently [github.com/gobwas/ws](https://github.com/gobwas/ws) by my ex-collegue Sergey Kamardin has been open-sourced. It provides a bit lower-level API than `gorilla/websocket` which allows to reduce memory usage and has nice optimizations for WebSocket upgrade process. It does not support WebSocket permessage-deflate compression (which is not always a good thing due to poor performance of flate and some browser bugs) but otherwise a good alternative you can consider using. If you have not read his famous post [A Million WebSockets and Go](https://www.freecodecamp.org/news/million-websockets-and-go-cc58418460bb/) – make a bookmark!

And one more library is [github.com/nhooyr/websocket](https://github.com/nhooyr/websocket). It's the youngest one and actively maintained. It compiles to WASM which can be a cool thing for someone. The API is a bit different from what `gorilla/websocket` offers, and one of the big advantages I see is that it solves a problem with a proper WebSocket closing handshake which is a bit hard to do right with Gorilla WebSocket.

You can consider all listed libraries except one from `x/net` for your project. Personally I prefer Gorilla WebSocket at moment since it's feature-complete and battle tested by tons of projects around Go world.

## OS tuning

OK, so you have chosen a library and built a server on top of it. As soon as you put it in production the interesting things start happenning.

Let's start with several OS specific key things you should do to prepare for many connections from WebSocket clients.

Every connection will cost you an open file descriptor so you should tune a maximum number of open file descriptors your process can use. A nice overview on how to do this on different systems can be found [in Riak docs](https://docs.riak.com/riak/kv/2.2.3/using/performance/open-files-limit.1.html). Wanna more connections? Make this limit higher.

Limit a maximum number of connection your process can server, make it less than known file descriptor limit:

```go
// ulimit -n == 65535
if conns.Len() >= 65500 {
    return errors.New("connection limit reached")
}
conns.Add(conn)
```

Keep attention on Ephemeral ports problem between your load balancer and your WebSocket server. A very good explanation can be found [in Pusher blog](https://making.pusher.com/ephemeral-port-exhaustion-and-how-to-avoid-it/)

Also your possible number of connections limited by conntrack table. Netfilter framework which is part of iptables keeps information about all connections and has limited size for this information. See how to see its limits and instructions to increase [in this article](https://morganwu277.github.io/2018/05/26/Solve-production-issue-of-nf-conntrack-table-full-dropping-packet/).

One more thing you can do is tune your network stack for performance. Do this only if you understand that you need it. Maybe start [with this gist](https://gist.github.com/mustafaturan/47268d8ad6d56cadda357e4c438f51ca).

Now let's speak about sending many messages. The general tips here:

* Make payload smaller. Btw WebSocket framing overhead is minimal and adds only [2-8 bytes to your payload](https://crossbario.com/blog/Dissecting-Websocket-Overhead/)
* Make less syscalls, every syscall will have a constant overhead, and actually in WebSocket server under load you will mostly see read and write syscalls in your CPU profiles. Obvious advice here: try to use client-server protocol that supports message batching so you can join individual messages together.
* Use effective message serialization protocol. Maybe use code generation for JSON to avoid extensive usage of reflect package done by Go std lib. Maybe use sth like [github.com/gogo/protobuf](https://github.com/gogo/protobuf) package that allows to speedup Protobuf marshalling/unmarshalling in the same way. 

## WebSocket fallback transport

![ie](https://i.imgur.com/IAOyvmg.png)

Even in 2020 there are still users that can not make connection with WebSocket server. Actually the problem mostly appears with browsers. Some users still use old browsers. But they have a choice - install a better newer browser. But there could also be users behind corporative proxy. Empoyees can have a trusted certificate installed on their machine so company proxy can reencrypt even TLS traffic.

A very good solution to this is [Sockjs-Go](https://github.com/igm/sockjs-go/) library. this is a mature library that provides fallback transport for WebSocket. If client does not succeeds with WebSocket he/she can use some of HTTP transports for client-server communication: EventSource (Server-Sent Events), XHR-streaming, Long-Polling etc. The downside with those transports is that to achieve bidirectional communication you should use sticky sessions on your load balancer since SockJS keeps connection session state in process memory. We will talk about many instances of your WebSocket server very soon.

## Performance is not scalability

You can optimize client-server protocol, tune your OS, but at some point you won't be able to use only one process on one server machine. You need to scale connections and work your server does over different server machines. Horizontal scaling is also good for server high availability. Actually there are some sort of real-time applications where a single isolated process makes sense - for example multiplayer games where limited number of players play independent game rounds.

![many_instances](https://i.imgur.com/8ElqpjI.png)

As soon as you distribute connections over several machines you have to find a way to deliver message to special user. The basic approach here is publish messages to all server instances. This can work but this does not scale well. You need a sort of instance discovery to make this less painful.

Here comes PUB/SUB, where you can connect WebSocket server instances over central PUB/SUB broker. Clients that establish connections subscribe to topics (channels) in broker, and as soon as you publish message to that topic it will be delivered to all active subscribers on WebSocket server instances. No more messages than required will travel bbetween broker and WebSocket server instances.

Actually the main picture here illustrates exactly this architecture:

![gopher-broker](https://i.imgur.com/QOJ1M9a.png)

Let's think about requirements for a broker for real-time messaging application. We want a broker:

* with reasonable performance and possibility to scale
* which maintains message order in topics
* can support millions of topics, where each topic should be ephemeral and lightweight – topics can be created when user comes to application and removed after user goes away
* possibility to keep a sliding window of messages inside channel to help us survive massive reconnect scenario (will talk about this later below, can be a separate part from broker actually)

Personally when we talk about such brokers here are some options that come into my mind:

* RabbitMQ
* Kafka or Pulsar
* Nats or Nats-Streaming
* Tarantool
* Redis

Sure there are more exist.

If I were you I won't go with RabbitMQ. This is a great messaging server but it does not like a high rate of queue bind and unbind type of load. It will work but you will need to use a lot of server resiurces for not so big number of clients (having a personal topic per client is a very commong thing, now imagine having a millions of queues inside RabbitMQ).

Kafka and Pulsar are great solutions, but not for this task I believe. The problem is again in dynamic ephemeral nature of our topics. Kafka also likes a more stable configuration of its topics. Keeping messages on disk can be an overkill for real-time messaging task. Also your consumers on Kafka server shouls pull from thousands of different topics, not sure how well it performs but my thoughts at moment - this should not perform very well. Here is [a post from Trello](https://tech.trello.com/why-we-chose-kafka/) where they moved from RabbitMQ to Kafka for similar real-time messaging task and got about 5x resource usage improvements.

Nats and Nats-Streaming. Sure Nats has a lot of sence here - I suppose it can be a good candidate. Nats-Streaming will also allow you to not lose messages, but I don't have enough information about how well Nats-Streaming scales to millions of topics. Recently Nats developers released native WebSocket support so you can consider it for your application (though I am not sure it's flexible enough to solve all tasks of modern real-time apps). An upcoming Jetstream which will be a part of Nats server can also be an interesting option – but again, it involves disk storage, a nice thing for backend microservices communication but can be an overkill for messaging with application clients.

Sure Tarantool can fit to this task well too. There is an article and repo which shows how to achieve our goal with Tarantool with pretty low resource requirements for broker. Maybe the only problem with Tarantool is not a very healthy state of its client libraries and complexity. You should invest enough time to benefit from it. But this can worth it actually. See [an article](https://hackernoon.com/tarantool-when-it-takes-500-lines-of-code-to-notify-a-million-users-11d340523493) on how to do a performant broker for WebSocket applications with Tarantool.

My personal choice here is Redis. It's very fast (especially when using pipeling protocol feature), very **predictable**. It gives you a good understanding of operation time complexity. You can shard topics over different Redis instances running in HA setup - with Sentinel or with Redis Cluster. It allows to write LUA procedures with some advanced logic which can be uploaded over client protocol thus feels like ordinary commands. So we can also use Redis as sliding window message cache. Again - we will talk why this can be important later.

Anyway, whatever broker will be your choice try to follow this rules to build effective PUB/SUB system:

* make sure to use one of pool of connections, don't create new connection per each client or topic that comes to tour WebSocket server
* Use effective serialization format between your WebSocket server and broker

## Massive reconnect

![mass_reconnect](https://i.imgur.com/S9koKYg.png)

Let's talk about one more problem that is unique for Websocket servers compared to HTTP. Your app can have thousands or millions of active WebSocket connections. In contract to stetless HTTP APIs your application is stateful. It uses push model. As soon as you deploying your WebSocket server or reload your load balancer (Nginx maybe) – connections got dropped and all that army of users start reconnecting. And this can be like an avalanche actually. How to survive?

First of all - use exponential backoff strategies on client side. I.e. reconnect with intervals like 1, 2, 4, 8, 16 seconds with some random jitter.

Turn on rate limiting strategies on your WebSocket server.

And one more interesting technique is using JWT (JSON Web Token) for authentication. I'll try to explain why this can be useful.

![jwt](https://i.imgur.com/aaTEhXo.png)

As soon as your client start reconnecting you will have to authenticate each connection. In massive setups with many persistent connection this can be a high load on your Session backend. Since you need an extra request to your session storage for every client coming back. This can be a no problem for some infrastructures but can be really disastrous for others. JWT allows to reduce this spike in load on session storage since it can have all required authentication information inside its payload. When using JWT make sure you have chosen a reasonable JWT expiration time - expiration interval depends on your application nature and just one of trade-offs you should deal with as developer.

And don't forget about making an effective connection between your WebSocket server and broker – as soon as all clients start reconnecting you should resubscribe your server nodes to all topics as fast as possible. Use techniques like smart batching at this moment.

Let's look at small piece of code that demostrates this technique. Imagine we have a source channel from which we get items to process. We don’t want to process items individually but in batch. For this we wait for first item coming from channel, then try to collect as many items from channel buffer as we want without blocking and timeouts involved. And then process slice of items we collected at once. For example build Redis pipeline from them and send to Redis in one connection write call.

```go
maxBatchSize := 50

for {
    select {
    case channel := <-subCh:
        batch := []string{channel}
    loop:
        for len(batch) < maxBatchSize {
            select {
            case channel := <-subCh:
                batch = append(batch, channel)
            default:
                break loop
            }
        }
        // Do sth with collected batch – send 
        // pipeline request to Redis for ex.
    }
}
```

Also look at example in Go playground: https://play.golang.org/p/u7SAGOLmDke.

I also made a repo where I demostrate how this technique together with Redis pipelining feature allows to fully utilize connection for very good performance https://github.com/FZambia/redigo-smart-batching.

Now we come to a final part of this post. Maybe the most interesting one.

Not only mass client reconnections create a load on session backend but also a huge load can be created on your main application database. Why? Because WebSocket applications are stateful. Clients rely on stream of messages coming from backend to maintain its state actual. As soon as connection dropped client tries to reconnect. But it some scenarios it also wants to restore its actual state. what if client reconnected after 3 seconds? How many state updates it could miss? Nobody knows. So do make sure state is actual client restores it from application database. This is again a spike in load. In can be really painful with many active connections.

So what I think is nice to have is effective and performant sliding window cache of messages inside each channel where we can't afford to miss messages. Keep this cache in some in-memory storage. Redis suits well for this task too. It's possible to keep message cache in Redis List or Redis Stream data structures.

So as soon as client reconnects it can restore its state from fast in-memory storage without even quering your database. Very good! You can also create your own Websocket fallback implementation (like Long-Polling) utilizing that sliding window message cache.

## Conclusion

Centrifugo server has most of mechanics described here including the last described message cache for topics with limited size and retention period. Some details on how message recovery on reconnect works in Centrifugo can be found [here in docs](../server/recover.md). It also has techniques to prevent message lost due to at most once nature of Redis PUB/SUB giving at least once delivery guarantee in message history retention period and size.

Making a cool real-time app involves much more things than described above. But hope advices given here will be useful for a reader and help writing a more robust and more scalable WebSocket servers. Even if not using Centrifugo ;)
