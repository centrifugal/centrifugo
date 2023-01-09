Centrifugo is an open-source scalable real-time messaging server. Centrifugo can instantly deliver messages to application online users connected over supported transports (WebSocket, HTTP-streaming, SSE/EventSource, GRPC, SockJS, WebTransport). Centrifugo has the concept of a channel – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Several official client SDKs for browser and mobile development wrap the bidirectional protocol. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev).

## Release notes

### Improvements

* More human-readable tracing logging output (especially in Protobuf protocol case). On the other hand, tracing log level is much more expensive now. We never assumed it will be used in production – so seems an acceptable trade-off.
* Several internal optimizations in client protocol to reduce memory allocations.

### Fixes

* Fix: slow down subscription dissolver workers while Redis PUB/SUB is unavailable. This solves a CPU usage spike which may happen while Redis PUB/SUB is unavailable and last client unsubscribes from some channel.
* Relative static paths in Centrifugo admin web UI (to fix work behind reverse proxy on sub-path)
