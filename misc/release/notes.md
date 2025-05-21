Centrifugo is an open-source scalable real-time messaging server. Centrifugo can instantly deliver messages to application online users connected over supported transports (WebSocket, HTTP-streaming, Server-Sent Events (SSE/EventSource), GRPC, WebTransport). Centrifugo has the concept of a channel – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Several official client SDKs for browser and mobile development wrap the bidirectional protocol. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev).

## What's changed

### Improvements

* Minor tweaks in web UI: method select with quick search in Actions tab.

### Fixes

* Fixed concurrent map iteration and write panic occurring during Redis issues [centrifugal/centrifuge#473](https://github.com/centrifugal/centrifuge/pull/473).
* Fixed unmarshalling of `duration` type from environment variable JSON [#973](https://github.com/centrifugal/centrifugo/pull/973).
* Fixed an issue where `channel_replacements` were not applied when publishing to a channel via the Centrifugo API in NATS Raw Mode. See [#977](https://github.com/centrifugal/centrifugo/issues/977).

### Miscellaneous

* This release is built with Go 1.24.3.
* Updated dependencies.
* See also the corresponding [Centrifugo PRO release](https://github.com/centrifugal/centrifugo-pro/releases/tag/v6.2.1).
