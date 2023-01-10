Centrifugo is an open-source scalable real-time messaging server. Centrifugo can instantly deliver messages to application online users connected over supported transports (WebSocket, HTTP-streaming, SSE/EventSource, GRPC, SockJS, WebTransport). Centrifugo has the concept of a channel – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Several official client SDKs for browser and mobile development wrap the bidirectional protocol. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev).

## Release notes

### Improvements

* Possibility to disable client protocol v1 using `disable_client_protocol_v1` boolean option. To remind you about client protocol v1 vs v2 migration in Centrifugo v4 take a look at [v3 to v4 migration guide](https://centrifugal.dev/docs/getting-started/migration_v4#client-sdk-migration). Centrifugo v4 uses client protocol v2 by default, all our recent SDKs only support client protocol v2. So if you are using modern stack then you can disable clients to use outdated protocol v1 right now. In Centrifugo v5 support for client protocol v1 will be completely removed, see [Centrifugo v5 roadmap](https://github.com/centrifugal/centrifugo/issues/599).
* New boolean option `disallow_anonymous_connection_tokens`. When the option is set Centrifugo won't accept connections from anonymous users even if they provided a valid JWT. See [#591](https://github.com/centrifugal/centrifugo/issues/591)
* New option `client_connection_rate_limit` to limit the number of new real-time connections Centrifugo may accept per second, see [docs](https://centrifugal.dev/docs/server/configuration#client_connection_rate_limit)
* Implement `sub_refresh` proxy to periodically validate expiring subscriptions over the call from Centrifugo to the backend endpoint, see [#592](https://github.com/centrifugal/centrifugo/issues/592) and [docs](https://centrifugal.dev/docs/server/proxy#sub-refresh-proxy)
* More human-readable tracing logging output (especially in Protobuf protocol case). On the other hand, tracing log level is much more expensive now. We never assumed it will be used in production – so seems an acceptable trade-off.
* Several internal optimizations in client protocol to reduce memory allocations.
* More strict client protocol: only allow one pong message from client to server after receiving ping, disable sending commands over the connection which returned an error to the Connect command

### Fixes

* Fix: slow down subscription dissolver workers while Redis PUB/SUB is unavailable. This solves a CPU usage spike which may happen while Redis PUB/SUB is unavailable and last client unsubscribes from some channel.
* Relative static paths in Centrifugo admin web UI (to fix work behind reverse proxy on sub-path)
