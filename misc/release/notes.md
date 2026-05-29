Centrifugo is an open-source scalable real-time messaging server. It instantly delivers messages to application online users connected over supported transports (WebSocket, HTTP-streaming, Server-Sent Events (SSE), GRPC, WebTransport). Centrifugo is built around channel subscriptions – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, AI streaming responses, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Official client SDKs are available for JavaScript (browser, Node.js, React Native), Dart/Flutter, Swift, Java, Python, Go, and .NET. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev). For runnable demos see [centrifugal/examples](https://github.com/centrifugal/examples).

## What's changed

### Improvements

* Support Prometheus native histograms, see [#1136](https://github.com/centrifugal/centrifugo/pull/1136).
* Kafka consumer: don't re-init the client on retriable fetch errors, see [#1137](https://github.com/centrifugal/centrifugo/pull/1137).

### Fixes

* Add missing `envconfig` tags to NATS JetStream consumer config so its fields can be configured via environment variables, see [#1117](https://github.com/centrifugal/centrifugo/pull/1117) by @thuy-le-kafi. Applied the same fix to the Redis Streams and Azure Service Bus consumer configs, which had the same gap.
* Fix a bunch of flaky integration tests.

### Miscellaneous

* This release is built with Go 1.26.3
* Dependency updates
* See also the corresponding [Centrifugo PRO release](https://github.com/centrifugal/centrifugo-pro/releases/tag/v6.8.1).
