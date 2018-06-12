[![Join the chat at https://gitter.im/centrifugal/centrifuge](https://badges.gitter.im/Join%20Chat.svg)](https://gitter.im/centrifugal/centrifuge?utm_source=badge&utm_medium=badge&utm_campaign=pr-badge&utm_content=badge)
[![Build Status](https://travis-ci.org/centrifugal/centrifuge.svg)](https://travis-ci.org/centrifugal/centrifuge)

**Work in progress**. *Library has all features working but in active development stage so API is not stable at all. It has not been tested in production yet so use with caution. Feedback is highly appreciated.*

Centrifuge library represents real-time core for [Centrifugo](https://github.com/centrifugal/centrifugo) server. It's also aimed to be a general purpose real-time messaging library for Go programming language.

Message transports:

* Websocket transport with JSON or binary Protobuf protocol
* SockJS polyfill library support (JSON only)

Features:

* Fast and optimized for low-latency communication with thousands of client connections
* Scaling to many nodes with Redis PUB/SUB, built-in Redis sharding, Sentinel for HA
* Bidirectional asynchronous message communication, RPC calls
* Channel (room) concept to broadcast message to all channel subscribers
* Presence information for channels (show all active clients in channel)
* History information for channels (last messages published into channel)
* Join/leave events for channels (aka client goes online/offline)
* Message recovery mechanism for channels to survive short network disconnects
* MIT license

Clients (also work in progress but with most features already supported):

* [centrifuge-js](https://github.com/centrifugal/centrifuge-js/tree/c2) – for browser, NodeJS and React Native.
* [centrifuge-go](https://github.com/centrifugal/centrifuge-go/tree/c2) - for Go language.
* [centrifuge-mobile](https://github.com/centrifugal/centrifuge-mobile/c2) - for iOS and Android using `centrifuge-go` and `gomobile` project.

[Godoc](https://godoc.org/github.com/centrifugal/centrifuge) and [examples](https://github.com/centrifugal/centrifuge/tree/master/examples)
