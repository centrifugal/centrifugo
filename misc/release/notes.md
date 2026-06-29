Centrifugo is an open-source scalable real-time messaging server. It instantly delivers messages to application online users connected over supported transports (WebSocket, HTTP-streaming, Server-Sent Events (SSE), GRPC, WebTransport). Centrifugo is built around channel subscriptions – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, AI streaming responses, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Official client SDKs are available for JavaScript (browser, Node.js, React Native), Dart/Flutter, Swift, Java, Python, Go, and .NET. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev). For runnable demos see [centrifugal/examples](https://github.com/centrifugal/examples).

## What's changed

### Fixes

* Centrifugo now bounds the size of a WebSocket message after `permessage-deflate` decompression, see [#1162](https://github.com/centrifugal/centrifugo/pull/1162). [`websocket.message_size_limit`](https://centrifugal.dev/docs/transports/websocket#websocketmessage_size_limit) alone only bounds the compressed bytes received on the wire, so without an additional limit a small compressed frame could be inflated into a much larger amount of memory (a "decompression bomb"). The new [`websocket.decompressed_message_size_limit`](https://centrifugal.dev/docs/transports/websocket#websocketdecompressed_message_size_limit) option (and its [`uni_websocket.decompressed_message_size_limit`](https://centrifugal.dev/docs/transports/uni_websocket) counterpart) lets you tune this limit. When set to `0` (the default), the limit is derived from `message_size_limit` multiplied by the default multiplier (`10`); messages exceeding it cause Centrifugo to close the connection with a `message too big` close code. Only effective when [compression](https://centrifugal.dev/docs/transports/websocket#websocketcompression) is enabled. Reported by @alanturing881 via [GHSA-q6mr-3g59-5m8x](https://github.com/centrifugal/centrifugo/security/advisories/GHSA-q6mr-3g59-5m8x).

### Miscellaneous

* New blog post [Scaling Redis Pub/Sub to Millions of Channels and Hundreds of Subscriber Nodes](https://centrifugal.dev/blog/2026/06/29/scaling-redis-pub-sub) – a deep dive into how Centrifugo talks to Redis efficiently, shards across isolated Redis instances, and makes PUB/SUB work on Redis Cluster (sharded PUB/SUB, slot balance, connection count, and efficient resubscribes).
* PostgreSQL broker metrics were split into per-kind `broker_*` / `map_broker_*` subsystems and renamed with a `postgres_` prefix to align with the rest of the broker metric taxonomy, see [#1161](https://github.com/centrifugal/centrifugo/pull/1161). Previously stream and map PG brokers shared a `pg_broker_*` subsystem and were told apart by a `broker` label, so brokers with the same (or default) name collided. This is a **breaking change** for dashboards and alerts in PRO deployments using the PostgreSQL broker – the `broker` label is now `broker_name`, and metric names changed as follows: `pg_broker_cleanup_rows_deleted_total` → `broker_postgres_cleanup_removed_total`, `pg_broker_outbox_cursor_lag_seconds` → `broker_postgres_outbox_cursor_lag_seconds` / `map_broker_postgres_outbox_cursor_lag_seconds`, `pg_broker_partitions` → `broker_postgres_partitions` / `map_broker_postgres_partitions`. See [exposed metrics](https://centrifugal.dev/docs/server/observability#exposed-metrics) for the full list.
* This release is built with Go 1.26.4
* Dependency updates
* See also the corresponding [Centrifugo PRO release](https://github.com/centrifugal/centrifugo-pro/releases/tag/v6.8.4).
