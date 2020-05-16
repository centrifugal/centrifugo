# What is Centrifugo

Centrifugo is a scalable real-time messaging server in language-agnostic way.

The term language-agnostic in this definition means that it does not matter which programming language your application uses on a frontend or backend sides – Centrifugo can work in conjunction with any. 

Centrifugo is fast and scales well to support millions of client connections. 

Real-time messages are messages delivered to your application users almost immediately after some event happened - think live comments, chat apps, real-time charts, dynamic counters and multiplayer games.

There are several real-time messaging transports Centrifugo supports at moment:

* [Websocket](https://en.wikipedia.org/wiki/WebSocket) with JSON or binary Protobuf protocols
* [SockJS](https://github.com/sockjs/sockjs-client) – library that tries to establish Websocket connection first and then falls back to HTTP transports (Server-Sent Events, XHR-streaming, XHR-polling etc) automatically in case of problems with Websocket connection

## Join community

We have chats on Gitter and in Telegram – welcome!

[![Join the chat at https://gitter.im/centrifugal/centrifugo](https://badges.gitter.im/Join%20Chat.svg)](https://gitter.im/centrifugal/centrifugo?utm_source=badge&utm_medium=badge&utm_campaign=pr-badge&utm_content=badge) [![Join the chat at https://t.me/joinchat/ABFVWBE0AhkyyhREoaboXQ](https://img.shields.io/badge/Telegram-Group-blue.svg)](https://t.me/joinchat/ABFVWBE0AhkyyhREoaboXQ)

## Motivation of project

Centrifugo was originally born to help applications written in language or framework without builtin concurency support to introduce real-time updates. For example frameworks like Django, Flask, Yii, Laravel, Ruby on Rails etc has poor support of working with many persistent connections. Centrifugo aims to help with this and continue to write backend in your favorite language and favorite framework. It also has some features (performance, scalability, connection management, message recovery on reconnect etc) that can simplify your life as a developer even if you are writing backend in asynchronous concurrent language.

## Concepts

Centrifugo runs as standalone server and takes care of handling persistent connections from your application users. Your application backend and frontend can be written in any programming language. Your clients connect to Centrifugo from frontend using connection token (JWT) provided by your application backend and subscribe to channels. As soon as some event happens your application backend can publish message with event into channel using Centrifugo API. And that message will be delivered to all clients currently subscribed on channel. So actually Centrifugo is a user-facing PUB/SUB server. Here is a simplified scheme: 
<br><br>
![Centrifugo scheme](images/scheme.png)
