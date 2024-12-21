Centrifugo is an open-source scalable real-time messaging server. Centrifugo can instantly deliver messages to application online users connected over supported transports (WebSocket, HTTP-streaming, SSE/EventSource, GRPC, WebTransport). Centrifugo has the concept of a channel – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Several official client SDKs for browser and mobile development wrap the bidirectional protocol. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev).

## What's changed

### Improvements

* Change Dockerfile to run `centrifugo` under non-root user [#922](https://github.com/centrifugal/centrifugo/pull/922) by @dmeremyanin

### Fixes

* Fix a deadlock during pub/sub and recovery sync when using Redis engine and server-side subscriptions, fixes [#925](https://github.com/centrifugal/centrifugo/issues/925)
* Fix pause/resume race in Kafka async consumer [#927](https://github.com/centrifugal/centrifugo/pull/927) – the race could lead to a partition non being processed, while in normal condition the chance of the race is minimal, this was observed in a real system under CPU throttling conditions.
* Fix flaky `TestHandleRefreshWithoutProxyServerStart` test [#920](https://github.com/centrifugal/centrifugo/pull/920) by @makhov

### Miscellaneous

* This release is built with Go 1.23.4.
* Check out the [Centrifugo v6 roadmap](https://github.com/centrifugal/centrifugo/issues/832). It outlines important changes planned for the next major release. We have already started working on v6 and are sharing updates in the issue and our community channels.
* See also the corresponding [Centrifugo PRO release](https://github.com/centrifugal/centrifugo-pro/releases/tag/v5.4.10).
