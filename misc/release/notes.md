Centrifugo is an open-source scalable real-time messaging server. Centrifugo can instantly deliver messages to application online users connected over supported transports (WebSocket, HTTP-streaming, SSE/EventSource, GRPC, SockJS, WebTransport). Centrifugo has the concept of a channel – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Several official client SDKs for browser and mobile development wrap the bidirectional protocol. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev).

## What's changed

### Improvements

* Support `expire_at` field of SubscribeResult from Subscribe Proxy [#707](https://github.com/centrifugal/centrifugo/pull/707)
* Option to skip client token signature verification [#708](https://github.com/centrifugal/centrifugo/pull/708)

### Fixes

* Fix connecting to Redis server over unix socket - inherited from [centrifugal/centrifuge#318](https://github.com/centrifugal/centrifuge/pull/318) by @tie

### Misc

* Release is built with Go 1.21.1
* Dependencies updated (centrifuge, quic-go, grpc, and others)
