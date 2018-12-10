# FAQ

Answers on various questions here.

### How many connections can one Centrifugo instance handle?

This depends on many factors. Hardware, message rate, size of messages, channel options enabled, client distribution over channels, websocket compression on/off etc. So no certain answer on this question exists. Common sense, tests and monitoring can help here. Generally we suggest to not put more than 50-100k clients on one node - but you should measure.

### Can Centrifugo scale horizontally?

Yes, it can. It can do this using builtin Redis Engine. Redis is very fast – for example it can handle hundreds of thousands requests per second. This should be OK for most applications in internet. But if you are using Centrifugo and approaching this limit then it's possible to add sharding support to balance queries between different Redis instances.

### Message delivery model and message order guarantees

The model of message delivery of Centrifugo server is at most once.

This means that message you send to Centrifugo can be theoretically lost while moving towards your clients. Centrifugo tries to do a best effort to prevent message losses but you should be aware of this fact. Your application should tolerate this. Centrifugo has an option to automatically recover messages that have been lost because of short network disconnections. But there are cases when Centrifugo can't guarantee message delivery. We also recommend to model your applications in a way that users don't notice when message have been lost. For example if your user posts a new comment over AJAX call to your application backend - you should not rely only on Centrifugo to get new comment form and display it - you should return new comment data in AJAX call response and render it. Be careful to not draw comments twice in this case.

Message order in channels guaranteed to be the same while you publish messages into channel one after another or publish them in one request. If you do parallel publishes into the same channel then Centrifugo can't guarantee message order.

### Centrifugo stops accepting new connections, why?

The most popular reason behind this is reaching open file limit. Just make it higher, we described how to do this nearby in this doc.

### Can I use Centrifugo without reverse-proxy like Nginx before it?

Yes, you can - Go standard library designed to allow this. But proxy before Centrifugo can
be very useful for load balancing clients for example.

### Does Centrifugo work with HTTP/2?

Yes, Centrifugo works with HTTP/2.

You can disable HTTP/2 running Centrifugo server with `GODEBUG` environment variable:

```
GODEBUG="http2server=0" centrifugo -c config.json
```

### Is there a way to use single connection to Centrifugo from different browser tabs?

If underlying transport is HTTP-based and you use HTTP/2 then this will work automatically. In case of websocket connection there is a way to do this using `SharedWorker` object though we have no support to work this way in our Javascript library.

### What if I need to send push notifications to mobile or web applications?

Sometimes it's confusing to see a difference between real-time messages and push notifications. Centrifugo is a real-time messaging server. It can not send push notifications to devices - to Apple iOS devices via APNS, Android devices via GCM or browsers over Web Push API. This is a goal for another software. But the reasonable question here is how can I know when I need to send real-time message to client online or push notification to its device because application closed at client's device at moment. The solution is pretty simple. You can keep critical notifications for client in database. And when client read message send ack to your backend marking that notification as read by client, you save this ack too. Periodically you can check which notifications were sent to clients but they have not read it (no ack received). For such notification you can send push notification to its device using your own or another open-source solution. Look at Firebase for example!

### Can I know message was really delivered to client?

You can but Centrifugo does not have such API. What you have to do to ensure your client received message is sending confirmation ack from your client to your application backend as soon as client processed message coming from Centrifugo channel.

### Can I publish new messages over websocket connection from client?

Centrifugo designed to stream messages from server to client. Even though it's possible to publish messages into channels directly from client (when `publish` channel option enabled) - we strongly discourage this in production usage as those messages will go through Centrifugo without any control.

Theoretically Centrifugo could resend messages published from client to your application backend endpoint (i.e. having some sort of webhook built in) but it does not seem beneficial it terms of overall performance and application architecture at moment. And this will require extra layer of convetions about Centrifugo-to-backend communication. 

So in general when user generates an event it must be first delivered to your app backend using a convenient way (for example AJAX POST request for web application), processed on backend (validated, saved into main application database) and then published to Centrifugo using Centrifugo HTTP API or Redis queue.

Sometimes publishing from client directly into channel can be useful though - for personal projects, for demonstrations (like we do in our [examples](https://github.com/centrifugal/examples)) or if you trust your users and want to build application without backend. In all cases when you don't need any message control
on your backend.

### How to create secure channel for two users only (private chat case)?

There are several ways to achieve it:

* use private channel (starting with `$`) - every time user will try to subscribe on it your backend should provide sign to confirm that subscription request. Read more in [special chapter about channels](https://centrifugal.github.io/centrifugo/server/channels/#private-channel-prefix)
* next is [user limited channels](https://centrifugal.github.io/centrifugo/server/channels/#user-channel-boundary) (with `#`) - you can create channel with name like `dialog#42,567` to limit subscribers only to user with id `42` and user with ID `567`
* finally you can create hard to guess channel name (based on some secret key and user IDs or just generate and save this long unique name into your main app database) so other users won't know this channel to subscribe on it. This is the simplest but not the safest way - but can be reasonable to consider in many situations.

### What's a best way to organize channel configuration?

In most situations your application need several real-time features. We suggest to use namespaces for every real-time feature if it requires some option enabled.

For example if you need join/leave messages for chat app - create special channel namespace with this `join_leave` option enabled. Otherwise your other channels will receive join/leave messages too - increasing load and traffic in system but not actually used by clients.

The same relates to other channel options.

### Can I rely on Centrifugo and its message history for guaranteed message delivery?

No - Centrifugo is best-effort transport. This means that if you want strongly guaranteed message delivery to your clients then you can't just rely on Centrifugo and its message history cache. In this case you still can use Centrifugo for real-time but you should build some additional logic on top your application backend and main data storage to satisfy your guarantees.

Centrifugo can keep message history for a while and you can want to rely on it for your needs. Centrifugo is not designed as data storage - it uses message history mostly for recovering missed messages after short client internet connection disconnects. It's not designed to be used to sync client state after being offline for a long time - this logic should be on your app backend.

### What is the difference between Centrifugo and Centrifuge

[Centrifugo](https://github.com/centrifugal/centrifugo) is server built on top of [Centrifuge](https://github.com/centrifugal/centrifuge) library for Go language.

This documentation was built to describe Centrifugo. Though many things said here can be considered as extra documentation for Centrifuge library.

### I have not found an answer on my question here:

We have [Gitter chat room](https://gitter.im/centrifugal/centrifugo) and [Telegram group](https://t.me/joinchat/ABFVWBE0AhkyyhREoaboXQ) - welcome!

### I want to contribute to this awesome project

We have many things you can help with – just ask us in our chat rooms.
