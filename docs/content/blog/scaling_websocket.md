---
template: overrides/blog_base.html
title: Scaling WebSocket in Go and beyond
description: Several tips to write scalable WebSocket servers with Go language and beyond.
og_title: Scaling WebSocket in Go and beyond
og_image: https://i.imgur.com/QOJ1M9a.png
og_image_width: 2000
og_image_height: 1000
---

# Scaling WebSocket in Go and beyond

![gopher-broker](https://i.imgur.com/QOJ1M9a.png)

I believe that in 2020 WebSocket is still an entertaining technology which is not so well-known and understood like HTTP. In this blog post I'd like to tell about state of WebSocket in Go language ecosystem, and a way we could write scalable WebSocket servers with Go and beyond Go.

We won't talk a lot about WebSocket transport pros and cons – I'll provide links to other resources on this topic. Most advices here are generic enough and can be easily approximated to other programming languages. Also in this post we won't talk about ready to use solutions (if you are looking for it – check out [Real-time Web Technologies guide](https://www.leggetter.co.uk/real-time-web-technologies-guide/) by Phil Leggetter), just general considerations. There is not so much information about scaling WebSocket on the internet so if you are interested in WebSocket and real-time messaging technologies - keep on reading.

If you don't know what WebSocket is – check out the following curious links:

* https://hpbn.co/websocket/ – a wonderful chapter of great book by Ilya Grigorik
* https://lucumr.pocoo.org/2012/9/24/websockets-101/ – valuable thoughts about WebSocket from Armin Ronacher

As soon as you know WebSocket basics – we can proceed.

## WebSocket server tasks

Speaking about scalable servers that work with many persistent WebSocket connections – I found several important tasks such a server should be able to do:

* Maintain many active connections
* Send many messages to clients
* Support WebSocket fallback to scale to every client
* Authenticate incoming connections and invalidate connections
* Survive massive reconnect of all clients without loosing messages

!!!note
    Of course not all of these points equally important in various situations.

Below we will look at some tips which relate to these points.

![one_hour_scale](https://i.imgur.com/4lYjJSP.png)

## WebSocket libraries

In Go language ecosystem we have several libraries which can be used as a building block for a WebSocket server.

Package [golang.org/x/net/websocket](https://godoc.org/golang.org/x/net/websocket) is considered **deprecated**.

The default choice in the community is [gorilla/websocket](https://github.com/gorilla/websocket) library. Made by Gary Burd (who also gifted us an awesome [Redigo](https://github.com/gomodule/redigo) package to communicate with Redis) – it's widely used, performs well, has a very good API – so in most cases you should go with it. Some people think that library not actively maintained at moment – but this is not quite true, it implements full WebSocket RFC, so actually it can be considered done.

In 2018 my ex-colleague Sergey Kamardin open-sourced [gobwas/ws](https://github.com/gobwas/ws) library. It provides a bit lower-level API than `gorilla/websocket` thus allows reducing RAM usage per connection and has nice optimizations for WebSocket upgrade process. It does not support WebSocket `permessage-deflate` compression but otherwise a good alternative you can consider using. If you have not read Sergey's famous post [A Million WebSockets and Go](https://www.freecodecamp.org/news/million-websockets-and-go-cc58418460bb/) – make a bookmark!

One more library is [nhooyr/websocket](https://github.com/nhooyr/websocket). It's the youngest one and actively maintained. It compiles to WASM which can be a cool thing for someone. The API is a bit different from what `gorilla/websocket` offers, and one of the big advantages I see is that it solves a problem with a proper WebSocket closing handshake which is [a bit hard to do right with Gorilla WebSocket](https://github.com/gorilla/websocket/issues/448).

You can consider all listed libraries except one from `x/net` for your project. Take a library, follow its examples (make attention to goroutine-safety of various API operations). Personally I prefer Gorilla WebSocket at moment since it's feature-complete and battle tested by tons of projects around Go world.

## OS tuning

OK, so you have chosen a library and built a server on top of it. As soon as you put it in production the interesting things start happening.

Let's start with several OS specific key things you should do to prepare for many connections from WebSocket clients.

Every connection will cost you an open file descriptor, so you should tune a maximum number of open file descriptors your process can use. An errors like `too many open files` raise due to OS limit on file descriptors which is usually 256-1024 by default (see with `ulimit -n` on Unix). A nice overview on how to do this on different systems can be found [in Riak docs](https://docs.riak.com/riak/kv/2.2.3/using/performance/open-files-limit.1.html). Wanna more connections? Make this limit higher.

Nice tip here is to limit a maximum number of connections your process can serve – making it less than known file descriptor limit:

```go
// ulimit -n == 65535
if conns.Len() >= 65500 {
    return errors.New("connection limit reached")
}
conns.Add(conn)
```

– otherwise you have a risk to not even able to look at `pprof` when things go bad. And you always need monitoring of open file descriptors.

You can also consider using [netutil.LimitListener](https://godoc.org/golang.org/x/net/netutil#LimitListener) for this task, but don't forget to put pprof on another port with another HTTP server instance in this case.

Keep attention on *Ephemeral ports* problem which is often happens between your load balancer and your WebSocket server. The problem arises due to the fact that each TCP connection uniquely identified in the OS by the 4-part-tuple:

```
source ip | source port | destination ip | destination port
```

On balancer/server boundary you are limited in 65536 possible variants by default. But actually due to some OS limits and sockets in TIME_WAIT state the number is even less. A very good explanation and how to deal with it can be found [in Pusher blog](https://making.pusher.com/ephemeral-port-exhaustion-and-how-to-avoid-it/).

Your possible number of connections also limited by conntrack table. Netfilter framework which is part of iptables keeps information about all connections and has limited size for this information. See how to see its limits and instructions to increase [in this article](https://morganwu277.github.io/2018/05/26/Solve-production-issue-of-nf-conntrack-table-full-dropping-packet/).

One more thing you can do is tune your network stack for performance. Do this only if you understand that you need it. Maybe start [with this gist](https://gist.github.com/mustafaturan/47268d8ad6d56cadda357e4c438f51ca), but don't optimize without full understanding why you are doing this. 

## Sending many messages

Now let's speak about sending many messages. The general tips follows.

**Make payload smaller**. This is obvious – fewer data means more effective work on all layers. BTW WebSocket framing overhead is minimal and adds only 2-8 bytes to your payload. You can read detailed dedicated research in [Dissecting WebSocket's Overhead](https://crossbario.com/blog/Dissecting-Websocket-Overhead/) article. You can reduce an amount of data traveling over network with `permessage-deflate` WebSocket extension, so your data will be compressed. Though using `permessage-deflate` is not always a good thing for server due to [poor performance of flate](https://github.com/gorilla/websocket/issues/203), so you should be prepared for a CPU and RAM resource usage on server side. While Gorilla WebSocket has a lot of optimizations internally by reusing flate writers, overhead is still noticeable. The increase value heavily depends on your load profile.

**Make less system calls**. Every syscall will have a constant overhead, and actually in WebSocket server under load you will mostly see read and write system calls in your CPU profiles. An advice here – try to use client-server protocol that supports message batching, so you can join individual messages together.

**Use effective message serialization protocol**. Maybe use code generation for JSON to avoid extensive usage of reflect package done by Go std lib. Maybe use sth like [gogo/protobuf](https://github.com/gogo/protobuf) package which allows to speedup Protobuf marshalling and unmarshalling. Unfortunately Gogo Protobuf [is going through hard times
](https://github.com/gogo/protobuf/issues/691) at this moment. Try to serialize a message only once when sending to many subscribers.

**Have a way to scale to several machines** - more power, more possible messages. We will talk about this very soon.

## WebSocket fallback transport

![ie](https://i.imgur.com/IAOyvmg.png)

Even in 2020 there are still users which cannot establish connection with WebSocket server. Actually the problem mostly appears with browsers. Some users still use old browsers. But they have a choice – install a newer browser. Still, there could also be users behind corporate proxies. Employees can have a trusted certificate installed on their machine so company proxy can re-encrypt even TLS traffic. Also, some browser extensions can block WebSocket traffic.

One ready solution to this is [Sockjs-Go](https://github.com/igm/sockjs-go/) library. This is a mature library that provides fallback transport for WebSocket. If client does not succeed with WebSocket connection establishment then client can use some of HTTP transports for client-server communication: [EventSource aka Server-Sent Events](https://hpbn.co/server-sent-events-sse/), XHR-streaming, Long-Polling etc. The downside with those transports is that to achieve bidirectional communication you should use sticky sessions on your load balancer since SockJS keeps connection session state in process memory. We will talk about many instances of your WebSocket server very soon.

You can implement WebSocket fallback yourself, this should be simple if you have a sliding window message stream on your backend which we will discuss very soon.

Maybe look at [GRPC](https://grpc.io/docs/what-is-grpc/introduction/), depending on application it could be better or worse than WebSocket – in general you can expect a better performance and less resource consumption from WebSocket for bidirectional communication case. My measurements for a **bidirectional** scenario showed 3x win for WebSocket (binary + GOGO protobuf) in terms of server CPU consumption and 4 times less RAM per connection. Though if you only need RPC then GRPC can be a better choice. But you need additional proxy to work with GRPC from a browser. 

## Performance is not scalability

You can optimize client-server protocol, tune your OS, but at some point you won't be able to use only one process on one server machine. You need to scale connections and work your server does over different server machines. Horizontal scaling is also good for a server high availability. Actually there are some sort of real-time applications where a single isolated process makes sense - for example multiplayer games where limited number of players play independent game rounds.

![many_instances](https://i.imgur.com/8ElqpjI.png)

As soon as you distribute connections over several machines you have to find a way to deliver a message to a certain user. The basic approach here is to publish messages to all server instances. This can work but this does not scale well. You need a sort of instance discovery to make this less painful.

Here comes PUB/SUB, where you can connect WebSocket server instances over central PUB/SUB broker. Clients that establish connections with your WebSocket server subscribe to topics (channels) in a broker, and as soon as you publish a message to that topic it will be delivered to all active subscribers on WebSocket server instances. If server node does not have interested subscriber then it won't get a message from a broker thus you are getting effective network communication.

Actually the main picture of this post illustrates exactly this architecture:

![gopher-broker](https://i.imgur.com/QOJ1M9a.png)

Let's think about requirements for a broker for real-time messaging application. We want a broker:

* with reasonable performance and possibility to scale
* which maintains message order in topics
* can support millions of topics, where each topic should be ephemeral and lightweight – topics can be created when user comes to application and removed after user goes away
* possibility to keep a sliding window of messages inside channel to help us survive massive reconnect scenario (will talk about this later below, can be a separate part from broker actually)

Personally when we talk about such brokers here are some options that come into my mind:

* [RabbitMQ](https://www.rabbitmq.com/)
* [Kafka](https://kafka.apache.org/) or [Pulsar](https://pulsar.apache.org/)
* [Nats or Nats-Streaming](https://nats.io/)
* [Tarantool](https://www.tarantool.io/en/)
* [Redis](https://redis.io/)

**Sure there are more exist** including libraries like [ZeroMQ](https://zeromq.org/) or [nanomsg](https://nanomsg.org/).

Below I'll try to consider these solutions for the task of making scalable WebSocket server facing many user connections from Internet.

If you are looking for unreliable at most once PUB/SUB then any of solutions mentioned above should be sufficient. Many real-time messaging apps are ok with at most once guarantee delivery.

If you don't want to miss messages then things are a bit harder. Let's try to evaluate these options for a task where application has lots of different topics from which it wants to receive messages with at least once guarantee (having a personal topic per client is common thing in applications). A short analysis below can be a bit biased, but I believe thoughts are reasonable enough. I did not found enough information on the internet about scaling WebSocket beyond a single server process, so I'll try to fill the gap a little based on my personal knowledge without pretending to be absolutely objective in these considerations.

In some posts on the internet about scaling WebSocket I saw advices to use RabbitMQ for PUB/SUB stuff in real-time messaging server. While this is a great messaging server, it does not like a high rate of queue bind and unbind type of load. It will work, but you will need to use a lot of server resources for not so big number of clients (imagine having millions of queues inside RabbitMQ). I have an example from my practice where RabbitMQ consumed about 70 CPU cores to serve real-time messages for 100k online connections. After replacing it with Redis keeping the same message delivery semantics we got only 0.3 CPU consumption on broker side.

Kafka and Pulsar are great solutions, but not for this task I believe. The problem is again in dynamic ephemeral nature of our topics. Kafka also likes a more stable configuration of its topics. Keeping messages on disk can be an overkill for real-time messaging task. Also your consumers on Kafka server should pull from millions of different topics, not sure how well it performs, but my thoughts at moment - this should not perform very well. Kafka itself scales perfectly, you will definitely be able to achieve a goal but resource usage will be significant. Here is [a post from Trello](https://tech.trello.com/why-we-chose-kafka/) where they moved from RabbitMQ to Kafka for similar real-time messaging task and got about 5x resource usage improvements. Note also that the more partitions you have the more heavy failover process you get.

Nats and Nats-Streaming. Raw Nats can only provide at most once guarantee. BTW recently Nats developers [released native WebSocket support](https://github.com/nats-io/nats-server/issues/315), so you can consider it for your application. Nats-Streaming server as broker will allow you to not lose messages. To be fair I don't have enough information about how well Nats-Streaming scales to millions of topics. An upcoming [Jetstream](https://github.com/nats-io/jetstream) which will be a part of Nats server can also be an interesting option – like Kafka it provides a persistent stream of messages for at least once delivery semantics. But again, it involves disk storage, a nice thing for backend microservices communication but can be an overkill for real-time messaging task.

Sure Tarantool can fit to this task well too. It's fast, im-memory and flexible. Some possible problems with Tarantool are not so healthy state of its client libraries, complexity and the fact that it's heavily enterprise-oriented. You should invest enough time to benefit from it, but this can worth it actually. See [an article](https://hackernoon.com/tarantool-when-it-takes-500-lines-of-code-to-notify-a-million-users-11d340523493) on how to do a performant broker for WebSocket applications with Tarantool.

Building PUB/SUB system on top of ZeroMQ will require you to build separate broker yourself. This could be an unnecessary complexity for your system. It's possible to implement PUB/SUB pattern with ZeroMQ and nanomsg without a central broker, but in this case messages without active subscribers on a server will be dropped on a consumer side thus all publications will travel to all server nodes. 

My personal choice at moment is Redis. While **Redis PUB/SUB itself provides at most once guarantee**, you can build at least once delivery on top of PUB/SUB and Redis data structures (though this can be challenging enough). Redis is very fast (especially when using pipelining protocol feature), and what is more important – **very predictable**. It gives you a good understanding of operation time complexity. You can shard topics over different Redis instances running in HA setup - with Sentinel or with Redis Cluster. It allows writing LUA procedures with some advanced logic which can be uploaded over client protocol thus feels like ordinary commands. You can use Redis to keep sliding window event stream which gives you access to missed messages from a certain position. We will talk about this later.

OK, the end of opinionated thoughts here :)

Depending on your choice the implementation of your system will vary and will have different properties – so try to evaluate possible solutions based on your application requirements. Anyway, whatever broker will be your choice, try to follow this rules to build effective PUB/SUB system:

* take into account message delivery guarantees of your system: at most once or at least once, ideally you should have an option to have both for different real-time features in your app
* make sure to use one or pool of connections between your server and a broker, don't create new connection per each client or topic that comes to your WebSocket server
* use effective serialization format between your WebSocket server and broker

## Massive reconnect

![mass_reconnect](https://i.imgur.com/S9koKYg.png)

Let's talk about one more problem that is unique for Websocket servers compared to HTTP. Your app can have thousands or millions of active WebSocket connections. In contract to stateless HTTP APIs your application is stateful. It uses push model. As soon as you deploying your WebSocket server or reload your load balancer (Nginx maybe) – connections got dropped and all that army of users start reconnecting. And this can be like an avalanche actually. How to survive?

First of all - use exponential backoff strategies on client side. I.e. reconnect with intervals like 1, 2, 4, 8, 16 seconds with some random jitter.

Turn on various rate limiting strategies on your WebSocket server, some of them should be turned on your backend load balancer level (like controlling TCP connection establishment rate), some are application specific (maybe limit an amount of requests from certain user).

One more interesting technique to survive massive reconnect is using JWT (JSON Web Token) for authentication. I'll try to explain why this can be useful.

![jwt](https://i.imgur.com/aaTEhXo.png)

As soon as your client start reconnecting you will have to authenticate each connection. In massive setups with many persistent connection this can be a very significant load on your Session backend. Since you need an extra request to your session storage for every client coming back. This can be a no problem for some infrastructures but can be really disastrous for others. JWT allows to reduce this spike in load on session storage since it can have all required authentication information inside its payload. When using JWT make sure you have chosen a reasonable JWT expiration time – expiration interval depends on your application nature and just one of trade-offs you should deal with as developer.

Don't forget about making an effective connection between your WebSocket server and broker – as soon as all clients start reconnecting you should resubscribe your server nodes to all topics as fast as possible. Use techniques like smart batching at this moment.

Let's look at a small piece of code that demonstrates this technique. Imagine we have a source channel from which we get items to process. We don’t want to process items individually but in batch. For this we wait for first item coming from channel, then try to collect as many items from channel buffer as we want without blocking and timeouts involved. And then process slice of items we collected at once. For example build Redis pipeline from them and send to Redis in one connection write call.

```go
maxBatchSize := 50

for {
    select {
    case item := <-sourceCh:
        batch := []string{item}
    loop:
        for len(batch) < maxBatchSize {
            select {
            case item := <-sourceCh:
                batch = append(batch, item)
            default:
                break loop
            }
        }
        // Do sth with collected batch of items.
        println(len(batch))
    }
}
```

Look at a complete example in a Go playground: https://play.golang.org/p/u7SAGOLmDke.

I also made a repo where I demonstrate how this technique together with Redis pipelining feature allows to fully utilize connection for a good performance https://github.com/FZambia/redigo-smart-batching.

Another advice for those who run WebSocket services in Kubernetes. Learn how your ingress behaves – for example Nginx ingress can reload its configuration on every change inside Kubernetes services map resulting into closing all active WebSocket connections. Proxies like Envoy don't have this behaviour, so you can reduce number of mass disconnections in your system. You can also proxy WebSocket without using ingress at all over configured WebSocket service NodePort.

## Message event stream benefits

Here comes a final part of this post. Maybe the most important one.

Not only mass client re-connections could create a significant load on a session backend but also a huge load on your main application database. Why? Because WebSocket applications are stateful. Clients rely on a stream of messages coming from a backend to maintain its state actual. As soon as connection dropped client tries to reconnect. In some scenarios it also wants to restore its actual state. What if client reconnected after 3 seconds? How many state updates it could miss? Nobody knows. So to make sure state is actual client tries to get it from application database. This is again **a significant spike in load on your main database** in massive reconnect scenario. In can be really painful with many active connections.

So what I think is nice to have for scenarios where we can't afford to miss messages (like in chat-like apps for example) is having effective and performant stream of messages inside each channel. Keep this stream in fast in-memory storage. This stream can have time retention and be limited in size (think about it as a sliding window of messages). I already mentioned that Redis can do this – it's possible to keep messages in Redis List or Redis Stream data structures. Other broker solutions could give you access to such a stream inside each channel out of the box.

So as soon as client reconnects it can restore its state from fast in-memory event stream without even querying your database. Actually to survive mass reconnect scenario you don't need to keep such a stream for a long time – several minutes should be enough. You can **even create your own Websocket fallback implementation (like Long-Polling) utilizing event stream with limited retention**.

## Conclusion

Hope advices given here will be useful for a reader and will help writing a more robust and more scalable real-time application backends.

[Centrifugo server](https://github.com/centrifugal/centrifugo/) and [Centrifuge library for Go language](https://github.com/centrifugal/centrifuge) have most of the mechanics described here including the last one – message stream for topics limited by size and retention period. Both also have techniques to prevent message loss due to at most once nature of Redis PUB/SUB giving at least once delivery guarantee inside message history window size and retention period.
