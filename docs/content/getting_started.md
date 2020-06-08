# What is Centrifugo

Centrifugo is a scalable real-time messaging server in language-agnostic way.

The term `language-agnostic` in this definition means that it does not matter which programming language your application uses on a frontend or backend sides – Centrifugo works in conjunction with any. 

Centrifugo is fast and scales well to support millions of client connections.

Real-time messages are messages delivered to your application users almost immediately after some event happened - think live comments, chat apps, real-time charts, dynamic counters and multiplayer games.

There are several real-time messaging transports Centrifugo supports at moment:

* [Websocket](https://en.wikipedia.org/wiki/WebSocket) with JSON or binary Protobuf protocols
* [SockJS](https://github.com/sockjs/sockjs-client) – library that tries to establish Websocket connection first and then falls back to HTTP transports (Server-Sent Events, XHR-streaming, XHR-polling etc) automatically in case of problems with Websocket connection

## Join community

We have rooms in Telegram and Discord – welcome!

[![Join the chat at https://t.me/joinchat/ABFVWBE0AhkyyhREoaboXQ](https://img.shields.io/badge/Telegram-Group-orange?style=flat&logo=telegram)](https://t.me/joinchat/ABFVWBE0AhkyyhREoaboXQ) &nbsp;[![Join the chat at https://discord.gg/tYgADKx](https://img.shields.io/discord/719186998686122046?style=flat&label=Discord&logo=discord)](https://discord.gg/tYgADKx)

## Motivation of project

Centrifugo was originally born to help applications with server side written in language or framework without built-in concurrency support. In this case Centrifugo is a very straightforward and non-obtrusive way to introduce real-time updates. For example frameworks like Django, Flask, Yii, Laravel, Ruby on Rails etc has poor or not very performant support of working with many persistent connections. Centrifugo aims to help with this and continue to write a backend with your favorite language or favorite framework. Centrifugo also has some features (performance, scalability, connection management, message recovery on reconnect etc) that can simplify your life as a developer even if you are writing backend in asynchronous concurrent language.

## Concepts

Centrifugo runs as standalone server and takes care of handling persistent connections from your application users. Your application backend and frontend can be written in any programming language. Your clients connect to Centrifugo from a frontend and subscribe to channels. As soon as some event happens your application backend can publish a message with event into a channel using Centrifugo API. That message will be delivered to all clients currently subscribed on a channel. So actually Centrifugo is a user-facing PUB/SUB server. Here is a simplified scheme: 
<br><br>
![Centrifugo scheme](images/scheme.png)

## Highlights

Here is a list with main Centrifugo highlights:

* Centrifugo is fast and capable to scale to millions of simultaneous connections
* Simple integration with any application – works as separate service
* Simple server API (HTTP or GRPC)
* Client-side libraries for popular frontend environments
* JSON and binary Protobuf Websocket client protocol based on strict schema
* SockJS polyfill for web browsers without Websocket support
* User authentication with JWT or over connection request proxy to configured HTTP endpoint
* Proper connection management and expiration control
* Various types of channels: private, user-limited
* Various types of subscriptions: client-side or server-side
* Transform RPC calls over WebSocket/SockJS to configured HTTP endpoint call
* Presence information for channels (show all active clients in channel)
* History information for channels (last messages published into channel)
* Join/leave events for channels (client goes online/offline)
* Automatic recovery of missed messages between client reconnects over configured retention period
* Built-in administrative web panel
* Cross platform – works on Linux, MacOS and Windows
* Ready to deploy (Docker, RPM/DEB packages, automatic Let's Encrypt TLS certificates, Prometheus/Graphite monitoring)
* MIT license
