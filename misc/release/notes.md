Centrifugo is an open-source scalable real-time messaging server. Centrifugo can instantly deliver messages to application online users connected over supported transports (WebSocket, HTTP-streaming, SSE/EventSource, GRPC, WebTransport). Centrifugo has the concept of a channel – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Several official client SDKs for browser and mobile development wrap the bidirectional protocol. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev).

## What's changed

### Improvements

* Code transforms for HTTP proxy and unidirectional connect [#903](https://github.com/centrifugal/centrifugo/pull/903). See [the description in docs](https://centrifugal.dev/docs/server/proxy#unexpected-error-handling-and-code-transforms).
* Support Kafka `scram-sha-256`, `scram-sha-512` and `aws-msk-iam` SASL [#912](https://github.com/centrifugal/centrifugo/pull/912). See [updated docs](https://centrifugal.dev/docs/server/consumers#kafka-consumer-options) for Kafka consumer.

### Miscellaneous

* This release is built with Go 1.23.2.
* Check out the [Centrifugo v6 roadmap](https://github.com/centrifugal/centrifugo/issues/832). It outlines important changes planned for the next major release. We have already started working on v6 and are sharing updates in the issue and our community channels.
* See also the corresponding [Centrifugo PRO release](https://github.com/centrifugal/centrifugo-pro/releases/tag/v5.4.8).
