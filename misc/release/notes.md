Centrifugo is an open-source scalable real-time messaging server. Centrifugo can instantly deliver messages to application online users connected over supported transports (WebSocket, HTTP-streaming, Server-Sent Events (SSE/EventSource), GRPC, WebTransport). Centrifugo has the concept of a channel – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Several official client SDKs for browser and mobile development wrap the bidirectional protocol. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev).

## What's changed

### Improvements

* Option to use try lock for postgresql consumer, [#988](https://github.com/centrifugal/centrifugo/pull/988). New option for PG consumer `use_try_lock` (boolean, default `false`) to use `pg_try_advisory_xact_lock` instead of `pg_advisory_xact_lock` for locking outbox table. This may help to reduce the number of long-running transactions on PG side.

### Fixes

* Fix log entries about engine on start [#992](https://github.com/centrifugal/centrifugo/pull/992).

### Miscellaneous

* This release is built with Go 1.24.4.
* Updated dependencies.
* See also the corresponding [Centrifugo PRO release](https://github.com/centrifugal/centrifugo-pro/releases/tag/v6.2.2).
