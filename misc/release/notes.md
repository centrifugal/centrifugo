Centrifugo is an open-source scalable real-time messaging server. Centrifugo can instantly deliver messages to application online users connected over supported transports (WebSocket, HTTP-streaming, SSE/EventSource, GRPC, SockJS, WebTransport). Centrifugo has the concept of a channel – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Several official client SDKs for browser and mobile development wrap the bidirectional protocol. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev).

## What's changed in Centrifugo v5

In Centrifugo v5 we're phasing out old client protocol support, introducing a more intuitive HTTP API, adjusting token management behaviour in SDKs, improving configuration process, and refactoring the history meta ttl option. As the result you get a cleaner, more user-friendly, and optimized Centrifugo experience.

All the major details about the release may be found in [Centrifugo v5 release announcement](https://centrifugal.dev/blog/2023/06/29/centrifugo-v5-released) in our blog.

We also prepared [Centrifugo v5 migration guide](https://centrifugal.dev/docs/getting-started/migration_v5) which has more specific details about changes.

### Misc

* This release is built with Go 1.20.5
