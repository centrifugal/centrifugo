Centrifugo is an open-source scalable real-time messaging server. Centrifugo can instantly deliver messages to application online users connected over supported transports (WebSocket, HTTP-streaming, SSE/EventSource, GRPC, SockJS, WebTransport). Centrifugo has the concept of a channel – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Several official client SDKs for browser and mobile development wrap the bidirectional protocol. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev).

## Release notes

### Fixes

* Fix non-working bidirectional emulation in multi-node case [#590](https://github.com/centrifugal/centrifugo/issues/590)
* Process client channels for no-credentials case also, see issue [#581](https://github.com/centrifugal/centrifugo/issues/581)
* Fix setting `allow_positioning` for top-level namespace, [commit](https://github.com/centrifugal/centrifugo/commit/dbaf01776ff294ee6731cd5422146c0f23107cce)

### Misc

* This release is built with Go 1.19.4
