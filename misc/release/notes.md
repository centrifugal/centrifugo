Centrifugo is an open-source scalable real-time messaging server. Centrifugo can instantly deliver messages to application online users connected over supported transports (WebSocket, HTTP-streaming, SSE/EventSource, GRPC, SockJS, WebTransport). Centrifugo has the concept of a channel – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Several official client SDKs for browser and mobile development wrap the bidirectional protocol. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev).

## What's changed

### Improvements

* Option to extract client connection user ID from HTTP header [#730](https://github.com/centrifugal/centrifugo/pull/730). See [documentation](https://centrifugal.dev/docs/server/configuration#client_user_id_http_header) for it.
* Speed up channel config operations by using atomic.Value and reduce allocations upon channel namespace extraction by using channel options cache, [#727](https://github.com/centrifugal/centrifugo/pull/727)
* New metrics for the size of messages sent and received by Centrifugo real-time transport. And we finally described all the metrics exposed by Centrifugo in docs - see [Server observability -> Exposed metrics](https://centrifugal.dev/docs/server/observability#exposed-metrics)

### Fixes

* Fix `Lua redis lib command arguments must be strings or integers script` error when calling Redis reversed history and the stream metadata key does not exist, [#732](https://github.com/centrifugal/centrifugo/issues/732)

### Misc

* Dependencies updated (rueidis, quic-go, etc)
* Improved logging for bidirectional emulation transports and unidirectional transports - avoid unnecessary error logs
