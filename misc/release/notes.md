Centrifugo is an open-source scalable real-time messaging server. Centrifugo can instantly deliver messages to application online users connected over supported transports (WebSocket, HTTP-streaming, Server-Sent Events (SSE/EventSource), GRPC, WebTransport). Centrifugo has the concept of a channel – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Several official client SDKs for browser and mobile development wrap the bidirectional protocol. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev).

## What's changed

### Improvements

* Unidirectional WebSocket transport now makes closing handshake to better comply with WebSocket RFC by default. Added `uni_websocket.disable_closing_handshake` option to restore the behavior prior to Centrifugo v6.5.1 if needed. [#1071](https://github.com/centrifugal/centrifugo/pull/1071)
* Unidirectional WebSocket transport sends disconnect push messages by default. Added `uni_websocket.disable_disconnect_push` option to disable this behavior if needed (for example, if you want to extract code/reason from WebSocket close frame and want to avoid ambiguity). [#1071](https://github.com/centrifugal/centrifugo/pull/1071)
* Support client `insecure` mode for server side subs and take `allow_user_limited_channels` option into account when processing server subs. [#1070](https://github.com/centrifugal/centrifugo/pull/1070)

### Miscellaneous

* This release is built with Go 1.25.4.
* Updated dependencies.
* See also the corresponding [Centrifugo PRO release](https://github.com/centrifugal/centrifugo-pro/releases/tag/v6.5.0).
