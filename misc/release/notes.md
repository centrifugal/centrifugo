Centrifugo is an open-source scalable real-time messaging server. Centrifugo can instantly deliver messages to application online users connected over supported transports (WebSocket, HTTP-streaming, SSE/EventSource, GRPC, SockJS, WebTransport). Centrifugo has the concept of a channel – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Several official client SDKs for browser and mobile development wrap the bidirectional protocol. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev).

## Release notes

### Improvements

* Dynamic JWKS endpoint based on iss and aud – implemented in [#638](https://github.com/centrifugal/centrifugo/pull/638), [documented here](https://centrifugal.dev/docs/server/authentication#dynamic-jwks-endpoint)
* Add [redis_force_resp2](https://centrifugal.dev/docs/server/engines#redis_force_resp2) option, [#641](https://github.com/centrifugal/centrifugo/pull/641)
* Document [client_stale_close_delay](https://centrifugal.dev/docs/server/configuration#client_stale_close_delay), make it 10 sec  instead of 25 sec by default, relates [#639](https://github.com/centrifugal/centrifugo/issues/639)

### Misc

* This release is built with Go 1.20.3
