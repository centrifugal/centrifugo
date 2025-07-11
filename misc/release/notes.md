Centrifugo is an open-source scalable real-time messaging server. Centrifugo can instantly deliver messages to application online users connected over supported transports (WebSocket, HTTP-streaming, Server-Sent Events (SSE/EventSource), GRPC, WebTransport). Centrifugo has the concept of a channel – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Several official client SDKs for browser and mobile development wrap the bidirectional protocol. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev).

## What's changed

### Improvements

* Use our internal fork of the Gorilla WebSocket library for unidirectional WebSocket transport, which provides slightly better Upgrade performance [#1005](https://github.com/centrifugal/centrifugo/pull/1005). The fork was previously used only for bidirectional WebSocket transport.
* Kafka consumer: introduce a per-partition size-limited queue for backpressure and a configurable `fetch_max_wait` [#997](https://github.com/centrifugal/centrifugo/pull/997). Centrifugo now polls Kafka with `500ms` by default and avoids using the `SetOffsets` API of the `franz-go` library. Instead of a buffered channel, we now use a queue (limited by size, configured over `partition_queue_max_size`). The `partition_buffer_size` option was removed since the internal consumer implementation changed, but we do not expect this to be a problem for users based on the added benchmark and improved throughput in the new implementation. However, it may result in a warning in logs about using the non-existing `partition_buffer_size` option, which may be an inconvenience. See [updated docs](https://centrifugal.dev/docs/server/consumers#kafka-consumer).
* Kafka consumer: switch to ReadCommitted mode for consuming by default, adding a boolean `fetch_read_uncommitted` option to consume with ReadUncommitted mode [#1001](https://github.com/centrifugal/centrifugo/pull/1001). See [updated docs](https://centrifugal.dev/docs/server/consumers#kafka-consumer).

### Fixes

* Fix Kubernetes environment variable regex by @yinheli [#1003](https://github.com/centrifugal/centrifugo/pull/1003) – in some cases, this allows avoiding extra warnings in logs about variables with the `CENTRIFUGO_` prefix automatically injected by the K8S runtime.
* Fix panic when using the server `subscribe` API during Redis unavailability [#centrifugal/centrifuge#491](https://github.com/centrifugal/centrifuge/pull/491)

### Miscellaneous

* This release is built with Go 1.24.5.
* Updated dependencies.
* See also the corresponding [Centrifugo PRO release](https://github.com/centrifugal/centrifugo-pro/releases/tag/v6.2.3).
