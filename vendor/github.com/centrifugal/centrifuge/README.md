## Centrifuge

**Work in progress**. Not ready for production yet. Contributions are welcome.

This library represents real-time core for Centrifugo server. This is also aimed to be a standalone general real-time messaging library for Go programming language.

Message transports:

* Websocket transport using JSON or binary Protobuf protocol
* GRPC bidirectional streaming over HTTP/2 (Protobuf only)
* SockJS polyfill library support (JSON only)

Features:

* Fast and optimized for low-latency communication with thousands of client connections
* Scaling to many nodes with Redis PUB/SUB, built-in Redis sharding
* Presence information for channels (show all active clients in channel)
* History information for channels (last messages sent into channels)
* Join/leave events for channels (client goes online/offline)
* Message recovery mechanism to survive short network disconnects
* RPC support to call custom handlers in your Go code
* MIT license

[Godoc](https://godoc.org/github.com/centrifugal/centrifuge)
