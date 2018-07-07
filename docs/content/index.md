## Centrifugo overview

**This is a work in progress documentation for Centrifugo v2**

First of all we have chat on Gitter – welcome!

[![Join the chat at https://gitter.im/centrifugal/centrifugo](https://badges.gitter.im/Join%20Chat.svg)](https://gitter.im/centrifugal/centrifugo?utm_source=badge&utm_medium=badge&utm_campaign=pr-badge&utm_content=badge)

Centrifugo is a language-agnostic real-time messaging server. Language-agnostic means that it does not matter which programming language your application uses on frontend or backend sides - Centrifugo can work in conjunction with any. Real-time messages are messages that delivered to your clients almost immediately after some event happened - think live comments, chats, real-time charts, dynamic counters.

There are several main transports Centrifugo supports at moment:

* Websocket (JSON or binary Protobuf)
* SockJS (library that tries to establish Websocket connection first and then falls back to HTTP transports automatically in case of problems with Websocket connection)

## Motivation of project

Centrifugo was originally born to help applications written in language or framework without builtin concurency support to introduce real-time updates. For example frameworks like Django, Flask, Yii, Laravel, Ruby on Rails etc has poor support of working with many persistent connections. Centrifugo aims to help with this and continue to write backend in your favorite language and favorite framework. It also has some features that can simplify your life as a developer even if you are writing backend in asynchronous concurrent language.

## Concepts

Centrifugo is language-agnostic real-time server. It is running as standalone server and takes care of handling persistent connections from your frontend application users. Your application backend and frontend can be written in any programming language. Your clients connect to Centrifugo from frontend using connection credentials provided by your application backend, subscribe on channels. As soon as some event happens your application backend can publish message with event into channel using Centrifugo API. And that message will be delivered to all clients currently subscribed on channel. Here is a simplified scheme: 
<br><br>
![Architecture](images/scheme.png)
