Centrifugo is an open-source scalable real-time messaging server. Centrifugo can instantly deliver messages to application online users connected over supported transports (WebSocket, HTTP-streaming, SSE/EventSource, GRPC, SockJS, WebTransport). Centrifugo has the concept of a channel – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Several official client SDKs for browser and mobile development wrap the bidirectional protocol. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev).

## Release notes

This release contains an important fix of Centrifugo memory leak. The leak happens in all setups which use Centrifugo v4.0.2 or v4.0.3.

### Fixes

* Fix goroutine leak on connection close introduced by v4.0.2, [commit](https://github.com/centrifugal/centrifuge/commit/82107b38a42561ca022d50f7ee2ca038a6f120e9)

### Misc

* This release is built with Go 1.19.3
