Centrifugo is an open-source scalable real-time messaging server. Centrifugo can instantly deliver messages to application online users connected over supported transports (WebSocket, HTTP-streaming, SSE/EventSource, GRPC, SockJS, WebTransport). Centrifugo has the concept of a channel – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Several official client SDKs for browser and mobile development wrap the bidirectional protocol. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev).

## Release notes

### Improvements

* Fully rewritten Redis engine using [rueian/rueidis](https://github.com/rueian/rueidis) library. Many thanks to [@j178](https://github.com/j178) and [@rueian](https://github.com/rueian) for the help. Check out details in our blog post [Improving Centrifugo Redis Engine throughput and allocation efficiency with Rueidis Go library](https://centrifugal.dev/blog/2022/12/20/improving-redis-engine-performance). We expect that new implementation is backwards compatible with the previous one except some timeout options which were not documented, please report issues if any.
* Extended TLS configuration for Redis – it's now possible to set CA root cert, client TLS certs, set custom server name for TLS. See more details in the [updated Redis Engine option docs](https://centrifugal.dev/docs/server/engines#redis-engine-options). Also, it's now possible to provide certificates as strings.
