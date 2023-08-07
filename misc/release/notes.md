Centrifugo is an open-source scalable real-time messaging server. Centrifugo can instantly deliver messages to application online users connected over supported transports (WebSocket, HTTP-streaming, SSE/EventSource, GRPC, SockJS, WebTransport). Centrifugo has the concept of a channel – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Several official client SDKs for browser and mobile development wrap the bidirectional protocol. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev).

## What's changed

### Improvements

* Quiet mode and no expiration for gentoken/gensubtoken cli commands [#681](https://github.com/centrifugal/centrifugo/pull/681) - so token generation using cli helpers is more flexible now
* Add `proxy_static_http_headers` option and `static_http_headers` key for granular proxy [#687](https://github.com/centrifugal/centrifugo/pull/687) - so it's possible to append custom headers to HTTP proxy requests.

### Fixes

* Suppress warnings about k8s env vars, see [issue](https://github.com/centrifugal/centrifugo/issues/678)

### Misc

* Release is built with Go 1.20.7
* Dependencies updated (rueidis, quic-go, crypto, etc)
* Replace `interface{}` with `any` in code base, [#682](https://github.com/centrifugal/centrifugo/pull/682)
