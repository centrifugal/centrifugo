Centrifugo is an open-source scalable real-time messaging server. Centrifugo can instantly deliver messages to application online users connected over supported transports (WebSocket, HTTP-streaming, Server-Sent Events (SSE/EventSource), GRPC, WebTransport). Centrifugo has the concept of a channel – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Several official client SDKs for browser and mobile development wrap the bidirectional protocol. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev).

## What's changed

### Fixes

* Transport write must return after data written [#1106](https://github.com/centrifugal/centrifugo/pull/1106). This was noticed in CI after a [pull request](https://github.com/centrifugal/centrifuge-js/pull/349) made by @phront3nd3r. This is a regression from v6.6.0 due to malformed buffer reuse in WriteManyFn callback of client writer. This resulted into broken data written into connection – thus connection issues. The problem was reproducing in HTTP Stream and SSE transports (bidirectional and unidirectional). WebSocket, Webtransport, uni GRPC were not affected because they already return once data is written into connection. 

### Miscellaneous

* This release is built with Go 1.25.7
* See also the corresponding [Centrifugo PRO release](https://github.com/centrifugal/centrifugo-pro/releases/tag/v6.6.2).
