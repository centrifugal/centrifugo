# Frequently Asked Questions

Answers on various questions here.

### How many connections can one Centrifugo instance handle?

This depends on many factors. Hardware, message rate, size of messages, Centrifugo features enabled, client distribution over channels, websocket compression on/off etc. So no certain answer on this question exists. Common sense, tests and monitoring can help here. Generally we suggest to not put more than 50-100k clients on one node - but you should measure for your personal use case.

You can find a benchmark we did in [this docs chapter](misc/benchmark.md) – though the point above is still valid, measure and monitor your own setup.

### Memory usage per connection?

Depending on features enabled an amount of RAM required per each connection can vary. At moment, you can expect that each connection will cost about 30-50 KB of RAM, thus a server with 1 GB of RAM can handle about 20-30k connections.

### Can Centrifugo scale horizontally?

Yes, it can. It can do this using builtin Redis Engine. Redis is very fast – for example it can handle hundreds of thousands requests per second. This should be OK for most applications in an internet. if you approach Redis resource limits (CPU or memory) then it's possible to use Centrifugo consistent sharding support to balance queries between different Redis instances.

### Message delivery model and message order guarantees

The model of message delivery of Centrifugo server is at most once. With recovery feature it's possible to achieve at least once guarantee during retention period to recover missed messages after a temporary disconnect.

So without recovery feature message you send to Centrifugo can be theoretically lost while moving towards your clients.

Offline clients do not receive messages.

Centrifugo tries to do the best effort to prevent message losses on a way to online clients, but you should be aware of possible message loss. Your application should tolerate this. 

As said above Centrifugo has an option to automatically recover messages that have been lost because of short network disconnections. Also, it prevents message loss on a way from Redis to nodes over Redis PUB/SUB using additional sequence check and periodical synchronization.

We also recommend modeling your applications in a way that users don't notice when Centrifugo does not work at all. Use graceful degradation. For example if your user posts a new comment over AJAX call to your application backend - you should not rely only on Centrifugo to get new comment form and display it - you should return new comment data in AJAX call response and render it. This way user that posts a comment will think that everything works just fine. Be careful to not draw comments twice in this case.

Message order in channels guaranteed to be the same while you publish messages into channel one after another or publish them in one request. If you do parallel publishes into the same channel then Centrifugo can't guarantee message order.

### Should I create channels explicitly?

No. By default, channels created automatically as soon as first client subscribed to it. And destroyed automatically when last client unsubscribes from a channel.

When history is on then a window of last messages kept automatically during retention period. So client that comes later and subscribes to channel can retrieve those messages using call to history.

### What about best practices with number of channels?

Channel is a very lightweight entity - Centrifugo can deal with lots of channels, don't be afraid to use many channels.

**But** keep in mind that one client should not be subscribed to lots of channels at the same moment. Use channels for separate real-time features. Using no more than several channels for a client is what you should try to achieve. A good analogy is writing SQL queries – you need to make sure you return content using fixed amount of database queries, as soon as more entries on your page result in more queries - your pages start working very slow at some point. The same for channels - you better to deliver real-time events over fixed amount of channels. It takes a separate frame for a client to subscribe to single channel – more frames mean more heavy initial connection.

### Presence for chat apps - online status of your contacts

While presence is a good feature it does not fit well some apps. For example if you make chat app - you may probably use single personal channel for each user. In this case you cannot find who is online at moment using builtin Centrifugo presence feature as users do not share a common channel. You can solve this using your own separate service that tracks online status of your users (for example in Redis) and has bulk API that returns online status approximation for a list of users. This way you will have efficient scalable way to deal with online statuses. 

### Centrifugo stops accepting new connections, why?

The most popular reason behind this is reaching open file limit. Just make it higher, we described how to do this nearby in this doc.

### Can I use Centrifugo without reverse-proxy like Nginx before it?

Yes, you can - Go standard library designed to allow this. Though proxy before Centrifugo can be very useful for load balancing clients.

### Does Centrifugo work with HTTP/2?

Yes, Centrifugo works with HTTP/2.

You can disable HTTP/2 running Centrifugo server with `GODEBUG` environment variable:

```
GODEBUG="http2server=0" centrifugo -c config.json
```

### Is there a way to use single connection to Centrifugo from different browser tabs?

If underlying transport is HTTP-based, and you use HTTP/2 then this will work automatically. For WebSocket each browser tab creates new connection.

### What if I need to send push notifications to mobile or web applications?

Sometimes it's confusing to see a difference between real-time messages and push notifications. Centrifugo is a real-time messaging server. It can not send push notifications to devices - to Apple iOS devices via APNS, Android devices via GCM or browsers over Web Push API. This is a goal for another software. But the reasonable question here is how can I know when I need to send real-time message to client online or push notification to its device because application closed at client's device at moment. The solution is pretty simple. You can keep critical notifications for client in database. And when client read message send ack to your backend marking that notification as read by client, you save this ack too. Periodically you can check which notifications were sent to clients but they have not read it (no ack received). For such notification you can send push notification to its device using your own or another open-source solution. Look at Firebase for example!

### How can I know a message really delivered to a client?

You can but Centrifugo does not have such API. What you have to do to ensure your client received message is sending confirmation ack from your client to your application backend as soon as client processed message coming from Centrifugo channel.

### Can I publish new messages over websocket connection from a client?

Centrifugo designed to stream messages from server to client. Even though it's possible to publish messages into channels directly from a client (when `publish` channel option enabled) - we strongly discourage this in production usage as those messages will go through Centrifugo without any control.

Theoretically Centrifugo could resend messages published from client to your application backend endpoint (i.e. having some sort of webhook built in) but it does not seem beneficial it terms of overall performance and application architecture at moment. And this will require extra layer of convetions about Centrifugo-to-backend communication. 

So in general when user generates an event it must be first delivered to your app backend using a convenient way (for example AJAX POST request for web application), processed on backend (validated, saved into main application database) and then published to Centrifugo using Centrifugo HTTP or GRPC API.

Sometimes publishing from a client directly into a channel can be useful though - for personal projects, for demonstrations (like we do in our [examples](https://github.com/centrifugal/examples)) or if you trust your users and want to build application without backend. In all cases when you don't need any message control
on your backend.

Since Centrifugo v2.3.0 it's possible to utilize RPC proxy feature – in this case you can call RPC over Centrifugo WebSocket which will be translated to HTTP request to your backend. This way you can utilize WebSocket transport in bidirectional way.

### How to create secure channel for two users only (private chat case)?

There are several ways to achieve it:

* use a private channel (starting with `$`) - every time user will try to subscribe on it your backend should provide sign to confirm that subscription request. Read more in [special chapter about channels](https://centrifugal.github.io/centrifugo/server/channels/#private-channel-prefix)
* next is [user limited channels](https://centrifugal.github.io/centrifugo/server/channels/#user-channel-boundary) (with `#`) - you can create a channel with name like `dialog#42,567` to limit subscribers only to user with id `42` and user with ID `567`
* finally, you can create hard to guess channel name (based on some secret key and user IDs or just generate and save this long unique name into your main app database) so other users won't know this channel to subscribe on it. This is the simplest but not the safest way - but can be reasonable to consider in many situations.

### What's the best way to organize channel configuration?

In most situations your application need several real-time features. We suggest to use namespaces for every real-time feature if it requires some option enabled.

For example if you need join/leave messages for chat app - create a special channel namespace with this `join_leave` option enabled. Otherwise, your other channels will receive join/leave messages too - increasing load and traffic in system but not actually used by clients.

The same relates to other channel options.

### Can I rely on Centrifugo and its message history for guaranteed message delivery?

The short answer is – no, you cannot.

As you may know Centrifugo can keep message history for a while. But there are several caveats you need to know:

* Centrifugo with Memory engine will reset all history after restart
* When using Redis Engine history will inherit properties of your Redis setup
* Centrifugo client protocol designed to give a client a tip that some messages lost after reconnect (when message recovery feature is on)

### Does Centrifugo support webhooks?

Centrifugo designed in a way where messages mostly flow one direction: from server to client. In idiomatic case you publish messages to your backend first, then after saving to your main database publish to channel over Centrifugo API to deliver a real-time message to all active channel subscribers. Now if you need any extra callbacks/webhooks you can call your application backend yourself from client side (for example just after connect event fired in client library). There are several reasons why we can't simply add webhooks – some of them described in [this issue](https://github.com/centrifugal/centrifugo/issues/195).

A bit tricky thing are disconnects. It's pretty hard as there are no guarantee that disconnect code will have time to execute on client side (as client can just switch off its device or simply lose internet connection). If you need to know that client disconnected and program your business logic around this fact then a reasonable approach is periodically call your backend from client side and update user status somewhere on backend (use Redis maybe). This is a pretty robust solution where you can't occasionally miss disconnect event.

HTTP [proxy feature](https://centrifugal.github.io/centrifugo/server/proxy/) added in v2.3.0 allows integrating Centrifugo with your own session mechanism and provides a way to react on connection events. Also, it opens a road for bidirectional communication with RPC calls. But the note above about disconnects is still true - we can't simply call your app in case of client disconnects as loosing one such event can result in broken business logic inside your app.

### How scalable is presence and join/leave information?

Presence is good for small channels with a reasonable number of subscribers, as soon as there are tons of subscribers presence info becomes very expensive in terms of bandwidth (as it contains information about all clients in channel). There is `presence_stats` API method that can be helpful if you only need to know a number of clients (or unique users) in a channel. But in case of Redis engine even `presence stats` not optimized for channels with more that several thousands active subscribers. You may consider using separate service to deal with presense status information that provides information in near real-time maybe with some reasonable approximation.

The same is true for join/leave messages - as soon as you turn on join/leave events for a channel with many subscribers every join/leave event (which generally happen relatively frequently) result into many messages sent to each subscriber in a channel, drastically multiplying amount of messages travelling through the system. So be careful and estimate possible load.

### What is the difference between Centrifugo and Centrifuge

[Centrifugo](https://github.com/centrifugal/centrifugo) is a server built on top of [Centrifuge](https://github.com/centrifugal/centrifuge) library for Go language.

This documentation built to describe Centrifugo. Though many things said here can be considered as extra documentation for Centrifuge library.

### I have not found an answer on my question here:

We have [Gitter chat room](https://gitter.im/centrifugal/centrifugo) and [Telegram group](https://t.me/joinchat/ABFVWBE0AhkyyhREoaboXQ) - welcome!

### I want to contribute to this awesome project

We have many things you can help with – just ask us in our chat rooms.
