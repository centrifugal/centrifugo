Centrifugo is an open-source scalable real-time messaging server. Centrifugo can instantly deliver messages to application online users connected over supported transports (WebSocket, HTTP-streaming, SSE/EventSource, GRPC, WebTransport). Centrifugo has the concept of a channel – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Several official client SDKs for browser and mobile development wrap the bidirectional protocol. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev).

## What's changed

### Improvements

* Added Valkey - https://valkey.io/ - to the Redis engine integration tests suite. This is becoming important as AWS moves Elasticache to Valkey under the hood.
* Optimizations for unidirectional transports [#941](https://github.com/centrifugal/centrifugo/pull/941)
    * `uni_sse` – now supports writing many messages to the underlying HTTP connection buffer before `Flush`, thereby inheriting Centrifugo client protocol message batching capabilities. This may result into fewer syscalls => better CPU usage.
    * `uni_http_stream` – implements the same improvements as `uni_sse`.
    * `uni_websocket` – new boolean option `uni_websocket.join_push_messages`. Once enabled, it allows joining messages into a single websocket frame. It is a separate option for `uni_websocket` because, in this case, the client side must be prepared to decode the frame into separate messages. The messages follow the Centrifugal client protocol format (for JSON – new-line delimited). Essentially, this mirrors the format used by Centrifugo for the bidirectional websocket.
    * Reduced allocations during the unidirectional connect stage by eliminating the intermediary `ConnectRequest` object.
* Optimizations for bidirectional emulation:
    * The `http_stream` and `sse` transports used in bidirectional emulation now benefit from the same improvements regarding connection buffer writes as described for unidirectional transports.
    * The `/emulation` endpoint now uses faster JSON decoder for incoming requests.
* Improved documentation:
    * Enhanced readability in the [configuration](https://centrifugal.dev/docs/server/configuration) docs – available options are now clearly visible.
    * More detailed [unidirectional protocol description](https://centrifugal.dev/docs/transports/uni_client_protocol).
    * More structured descriptions for specific real-time transports.

### Miscellaneous

* Centrifugo v6 has been recently released. See the details in the [Centrifugo v6 release blog post](https://centrifugal.dev/blog/2025/01/16/centrifugo-v6-released).
* This release is built with Go 1.23.6.
* See also the corresponding [Centrifugo PRO release](https://github.com/centrifugal/centrifugo-pro/releases/tag/v6.0.3).
