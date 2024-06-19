Centrifugo is an open-source scalable real-time messaging server. Centrifugo can instantly deliver messages to application online users connected over supported transports (WebSocket, HTTP-streaming, SSE/EventSource, GRPC, WebTransport). Centrifugo has the concept of a channel – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Several official client SDKs for browser and mobile development wrap the bidirectional protocol. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev).

## What's changed

Centrifugo v5.4.1 comes with useful improvements and fixes.

### Improvements

* Improving [delta compression](https://centrifugal.dev/docs/server/delta_compression) - if the size of delta patch is greater than full publication payload – skip sending delta, just use the full payload.
* Kafka Consumer: add partition buffer to optimize processing, [#829](https://github.com/centrifugal/centrifugo/pull/829).
* Support release for Debian 12 Bookworm [#827](https://github.com/centrifugal/centrifugo/pull/827)

### Fixes

* Fix panic on already subscribed error - `panic: close of closed channel` could happen in case due to the race upon already subscribed error. See [centrifugal/centrifuge#390](https://github.com/centrifugal/centrifuge/pull/390).
* [Async consumers](https://centrifugal.dev/docs/server/consumers): fix disabling consumer by using proper mapstructure and JSON tags, [#828](https://github.com/centrifugal/centrifugo/pull/828).

### Miscellaneous

* Release is built with Go 1.22.4
* All dependencies were updated to latest versions
