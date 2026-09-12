Centrifugo is an open-source scalable real-time messaging server. It instantly delivers messages to application online users connected over supported transports (WebSocket, HTTP-streaming, Server-Sent Events (SSE), GRPC, WebTransport). Centrifugo is built around channel subscriptions – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, AI streaming responses, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Official client SDKs are available for JavaScript (browser, Node.js, React Native), Dart/Flutter, Swift, Java, Python, Go, and .NET. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev). For runnable demos see [centrifugal/examples](https://github.com/centrifugal/examples).

## What's changed

### Improvements

* Slightly less work when recording `centrifugo_client_ping_pong_duration_seconds` – the histogram observer is now cached instead of being looked up on every pong ([centrifugal/centrifuge#622](https://github.com/centrifugal/centrifuge/pull/622)).

### Fixes

* Fix possible panic in the channel options cache on long-running nodes. Its internal counter overflowed `int32` after enough cache misses, which gave a negative slot index and crashed the node with `index out of range` ([#1227](https://github.com/centrifugal/centrifugo/pull/1227)).
* Map subscriptions: the subscribe reply did not include `expires`/`ttl`, so clients could not refresh the subscription token in time. Also, if the map state was loaded in several pages, the client's subscription refresh was rejected with a `bad request` disconnect. A subscription whose expiration time was already in the past was accepted ([centrifugal/centrifuge#626](https://github.com/centrifugal/centrifuge/pull/626)).
* `allowed_origins` patterns with upper-case letters (e.g. `https://App.Example.com`) never matched, because the request `Origin` was lower-cased before matching and the pattern was not. Patterns are now matched case-insensitively ([#1230](https://github.com/centrifugal/centrifugo/pull/1230)).
* `client_name` was ignored for Redis used by `redis_stream` async consumers ([#1229](https://github.com/centrifugal/centrifugo/pull/1229)).
* PostgreSQL engine: handle one more case of several nodes creating the schema at the same time – a node could fail on start with `type ... already exists` (SQLSTATE `42710`) ([#1231](https://github.com/centrifugal/centrifugo/pull/1231)).
* The `centrifugo_client_subscriptions_accepted` metric was registered but never incremented, so it always showed zero ([centrifugal/centrifuge#622](https://github.com/centrifugal/centrifuge/pull/622)).
* The `client closed or unsubscribed after adding subscription` message is now logged at `info` level instead of `error`. It happens when a client goes away while its subscription is still being set up – this is expected and is not a server error ([centrifugal/centrifuge#621](https://github.com/centrifugal/centrifuge/pull/621)).

### Miscellaneous

* This release is built with Go 1.26.8.
* Dependency updates.
* See also the corresponding [Centrifugo PRO release](https://github.com/centrifugal/centrifugo-pro/releases/tag/v6.9.5).
