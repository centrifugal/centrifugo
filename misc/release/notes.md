Centrifugo is an open-source scalable real-time messaging server. It instantly delivers messages to application online users connected over supported transports (WebSocket, HTTP-streaming, Server-Sent Events (SSE), GRPC, WebTransport). Centrifugo is built around channel subscriptions – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, AI streaming responses, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Official client SDKs are available for JavaScript (browser, Node.js, React Native), Dart/Flutter, Swift, Java, Python, Go, and .NET. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev). For runnable demos see [centrifugal/examples](https://github.com/centrifugal/examples).

## What's changed

### Improvements

* New [`centrifugo_transport_frame_size`](https://centrifugal.dev/docs/server/observability#centrifugo_transport_frame_size) metric – a histogram of frame sizes received from client connections. One frame may contain several commands, so this can't be calculated from the existing per-command counters. Use it to choose a good value for `websocket.message_size_limit`, which is applied to the whole frame ([centrifugal/centrifuge#620](https://github.com/centrifugal/centrifuge/pull/620)).
* Fewer allocations when writing to Protobuf client connections. The same PR made whole-frame Protobuf command encoding ~2.1x faster – this mostly helps Go clients like `centrifuge-go` ([centrifugal/protocol#36](https://github.com/centrifugal/protocol/pull/36)).
* Less work on every client command: config accessors and the channel options cache no longer copy structs on each call ([#1215](https://github.com/centrifugal/centrifugo/pull/1215)).
* PostgreSQL broker, map broker and controller now validate `table_prefix` on start. A prefix with a hyphen, space or dot used to produce a confusing syntax error inside a generated `CREATE` statement ([#1223](https://github.com/centrifugal/centrifugo/pull/1223)).

### Fixes

* Async consumers: a `context.Canceled` error returned by the message dispatcher was treated as a shutdown signal. In the Kafka consumer this could skip records and then commit an offset after them, losing messages. Fixed for Kafka and SQS ([#1221](https://github.com/centrifugal/centrifugo/pull/1221)).
* PostgreSQL engine: handle the case when several nodes create the schema at the same time. Before that a node which lost the race could fail to start with `error initializing Postgres ... schema`. Thanks to [@AlexeyShalaev](https://github.com/AlexeyShalaev) for the initial fix ([#1220](https://github.com/centrifugal/centrifugo/pull/1220) and [#1222](https://github.com/centrifugal/centrifugo/pull/1222)).
* PostgreSQL map broker: the fast path in `EnsureSchema` never worked, so every node start ran the full DDL batch again. It works now and also refreshes the partition lookahead ([#1223](https://github.com/centrifugal/centrifugo/pull/1223)).
* Fix a race in log throttling of the connection limit check – it could print many duplicate warnings instead of one per throttle interval ([#1214](https://github.com/centrifugal/centrifugo/pull/1214)).
* Some config validation errors mentioned keys which don't exist, so it was hard to find the option to fix ([#1213](https://github.com/centrifugal/centrifugo/pull/1213)).
* Admin web UI: fix uptime formatting on the Status page ([centrifugal/web#71](https://github.com/centrifugal/web/pull/71)).

### Miscellaneous

* This release is built with Go 1.26.8.
* Dependency updates.
* Embedded admin web UI updated ([#1225](https://github.com/centrifugal/centrifugo/pull/1225)).
* See also the corresponding [Centrifugo PRO release](https://github.com/centrifugal/centrifugo-pro/releases/tag/v6.9.4).
