Centrifugo is an open-source scalable real-time messaging server. Centrifugo can instantly deliver messages to application online users connected over supported transports (WebSocket, HTTP-streaming, Server-Sent Events (SSE/EventSource), GRPC, WebTransport). Centrifugo has the concept of a channel – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Several official client SDKs for browser and mobile development wrap the bidirectional protocol. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev).

## What's changed

### Improvements

* Improved admin web UI:
    * Refreshed login screen with bubbles instead of lines 🫧
    * Sleek cards with totals on Status screen
    * Total subscriptions counter added in addition to Total nodes running and Total clients
    * Better response visualizations on Actions page to improve UX when the same response is returned
    * Monaco JSON editor instead of Ace — lighter size and slightly more visually appealing
    * Updated to React v19 and Material UI v7
* WebTransport: make it work with debug middleware, suppress errors [#1021](https://github.com/centrifugal/centrifugo/pull/1021)
* Introduce source code typo check in CI [#1004](https://github.com/centrifugal/centrifugo/pull/1004)

### Fixes

* Fix existing typos in source code by @yinheli [#1004](https://github.com/centrifugal/centrifugo/pull/1004)

### Miscellaneous

* This release is built with Go 1.24.6.
* Updated dependencies.
* See also the corresponding [Centrifugo PRO release](https://github.com/centrifugal/centrifugo-pro/releases/tag/v6.2.5).
