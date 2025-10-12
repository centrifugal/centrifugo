Centrifugo is an open-source scalable real-time messaging server. Centrifugo can instantly deliver messages to application online users connected over supported transports (WebSocket, HTTP-streaming, Server-Sent Events (SSE/EventSource), GRPC, WebTransport). Centrifugo has the concept of a channel – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Several official client SDKs for browser and mobile development wrap the bidirectional protocol. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev).

## What's changed

### Improvements

* Support for server-side publication filtering by tags. Client SDK (at this point only `centrifuge-js` >= v5.5.0) can supply a filter when subscribing to a channel, and server will deliver only publications with tags matching the filter. See [Channel publication filtering](https://centrifugal.dev/docs/server/publication_filtering) documentation chapter. Also read the new post on [Centrifugo blog](https://centrifugal.dev/blog/2025/10/14/server-side-publication-filtering-by-tags) which uncovers some design decisions behind this feature and demonstrates it in action.
* Various visual improvements on [https://centrifugal.dev](https://centrifugal.dev/) site. Check it out!

### Fixes

* Fix deadlock when using single connection feature [#1050](https://github.com/centrifugal/centrifugo/pull/1050). See more details in [#1044](https://github.com/centrifugal/centrifugo/issues/1044).

### Miscellaneous

* This release is built with Go 1.25.2.
* Updated dependencies.
* See also the corresponding [Centrifugo PRO release](https://github.com/centrifugal/centrifugo-pro/releases/tag/v6.4.0).
