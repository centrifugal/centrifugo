Centrifugo is an open-source scalable real-time messaging server. It instantly delivers messages to application online users connected over supported transports (WebSocket, HTTP-streaming, Server-Sent Events (SSE), GRPC, WebTransport). Centrifugo is built around channel subscriptions – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, AI streaming responses, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Official client SDKs are available for JavaScript (browser, Node.js, React Native), Dart/Flutter, Swift, Java, Python, Go, and .NET. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev). For runnable demos see [centrifugal/examples](https://github.com/centrifugal/examples).

## What's changed

### Improvements

* Connection runtime stability, consistency, and performance improvements coming from the underlying [Centrifuge](https://github.com/centrifugal/centrifuge) library. Under connection churn and concurrent subscribe/unsubscribe on the same channel, Centrifugo now keeps its internal subscription state consistent, fixes several resource and presence leaks, and avoids possible metric drift. The periodic presence updates also use noticeably less CPU and memory. As part of this work some internal operations became up to 500x faster under certain conditions ([centrifugal/centrifuge#590](https://github.com/centrifugal/centrifuge/pull/590)).
* Less memory allocation and garbage collection pressure on the message broadcast path. This mostly helps nodes that deliver many messages per second ([centrifugal/centrifuge#598](https://github.com/centrifugal/centrifuge/pull/598)).
* Redis: read commands now fail within the expected time when Redis is unreachable, instead of silently retrying until Redis comes back. This makes behavior during a Redis outage predictable and matches how writes already worked ([#1191](https://github.com/centrifugal/centrifugo/pull/1191), commit [`b9396cfc`](https://github.com/centrifugal/centrifugo/commit/b9396cfc), and [centrifugal/centrifuge#591](https://github.com/centrifugal/centrifuge/pull/591)).
* Redis: bound the worst-case time to detect a silently stalled connection (the peer stops replying but the TCP connection stays open) to under 5 seconds by tuning the keepalive ([#1192](https://github.com/centrifugal/centrifugo/pull/1192), commit [`e690724f`](https://github.com/centrifugal/centrifugo/commit/e690724f), and [centrifugal/centrifuge#592](https://github.com/centrifugal/centrifuge/pull/592)).
* Redis: verify PUB/SUB delivery with liveness probes to detect a broken subscription connection earlier ([centrifugal/centrifuge#594](https://github.com/centrifugal/centrifuge/pull/594)).
* Add per-IP throttling of failed attempts on the admin password login endpoint (`POST /admin/auth`) to slow down brute-force. A valid login is not affected, even while an attack is in progress. This is best-effort protection only – the admin endpoint should still be protected at the infrastructure level (firewall rules, private network, authenticating reverse proxy) ([#1204](https://github.com/centrifugal/centrifugo/pull/1204), commit [`7f8e8f6b`](https://github.com/centrifugal/centrifugo/commit/7f8e8f6b)).
* Harden the binary (Protobuf) protocol frame decoder so that a crafted length prefix can no longer make the server allocate too much memory. The configured message size limit is now always applied when reading client commands ([#1203](https://github.com/centrifugal/centrifugo/pull/1203), commit [`558c7dda`](https://github.com/centrifugal/centrifugo/commit/558c7dda), and [centrifugal/centrifuge#612](https://github.com/centrifugal/centrifuge/pull/612)).
* Redis Sentinel: the Sentinel client now periodically refreshes its topology, so changes such as a new master after a failover are picked up more reliably ([#1201](https://github.com/centrifugal/centrifugo/pull/1201), commit [`9015e98d`](https://github.com/centrifugal/centrifugo/commit/9015e98d), and [centrifugal/centrifuge#611](https://github.com/centrifugal/centrifuge/pull/611)).

### Fixes

* Fix a possible server process crash (nil pointer panic) that could happen when a delta publication failed to encode to JSON. The broadcast goroutine could panic and terminate the whole node ([centrifugal/centrifuge#597](https://github.com/centrifugal/centrifuge/pull/597)).
* Fix a case where a client could end up with several concurrent connections on the server under Redis load. On disconnect the transport is now closed before the per-channel cleanup, so a slow Redis no longer delays the socket teardown of the old connection ([centrifugal/centrifuge#595](https://github.com/centrifugal/centrifuge/pull/595)).
* Async consumers: do not acknowledge messages that were still being consumed during shutdown, so such messages are re-delivered later instead of being lost ([#1196](https://github.com/centrifugal/centrifugo/pull/1196), commit [`0708523c`](https://github.com/centrifugal/centrifugo/commit/0708523c)).
* Kafka consumer: clone records before pushing them to the partition queue to avoid their data being overwritten ([#1195](https://github.com/centrifugal/centrifugo/pull/1195), commit [`fe0207ef`](https://github.com/centrifugal/centrifugo/commit/fe0207ef)).
* Logging: raise the internal log handler buffer from 64 to 1024 to reduce the chance of blocking on logging under bursts ([#1190](https://github.com/centrifugal/centrifugo/pull/1190), commit [`cb43d83f`](https://github.com/centrifugal/centrifugo/commit/cb43d83f)).

### Miscellaneous

* This release is built with Go 1.26.6.
* Base Docker image updated to Alpine 3.24 ([#1194](https://github.com/centrifugal/centrifugo/pull/1194), commit [`b155119b`](https://github.com/centrifugal/centrifugo/commit/b155119b)).
* Dependency updates, including the latest Centrifuge library and gRPC ([#1200](https://github.com/centrifugal/centrifugo/pull/1200), [#1199](https://github.com/centrifugal/centrifugo/pull/1199), [#1193](https://github.com/centrifugal/centrifugo/pull/1193), [#1197](https://github.com/centrifugal/centrifugo/pull/1197)).
* See also the corresponding [Centrifugo PRO release](https://github.com/centrifugal/centrifugo-pro/releases/tag/v6.9.2).
