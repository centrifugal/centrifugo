## Centrifuge

**Work in progress**. Not ready for production yet. Contributions are welcome.

This library represents real-time core for [Centrifugo](https://github.com/centrifugal/centrifugo) server. It's also aimed to be a general real-time messaging library for Go programming language.

Message transports:

* Websocket transport using JSON or binary Protobuf protocol
* GRPC bidirectional streaming over HTTP/2
* SockJS polyfill library support (JSON only)

Features:

* Fast and optimized for low-latency communication with thousands of client connections
* Scaling to many nodes with Redis PUB/SUB, built-in Redis sharding, Sentinel for HA
* Bidirectional asynchronous message communication
* Built-in RPC support
* Presence information for channels (show all active clients in channel)
* History information for channels (last messages published into channel)
* Join/leave events for channels (aka client goes online/offline)
* Message recovery mechanism for channels to survive short network disconnects
* MIT license

[Godoc](https://godoc.org/github.com/centrifugal/centrifuge)
