Centrifugo is an open-source scalable real-time messaging server. Centrifugo can instantly deliver messages to application online users connected over supported transports (WebSocket, HTTP-streaming, Server-Sent Events (SSE/EventSource), GRPC, WebTransport). Centrifugo has the concept of a channel – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Several official client SDKs for browser and mobile development wrap the bidirectional protocol. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev).

## What's changed

### Improvements

* Add `use_existing_consumer` option for NATS async consumer by @b43 [#1074](https://github.com/centrifugal/centrifugo/pull/1074)
* Support for grpc `static_metadata`, tweak http `static_headers` behaviour to match the doc [#1076](https://github.com/centrifugal/centrifugo/pull/1076)
* New `publication_data_format` channel global and namespace option [#1085](https://github.com/centrifugal/centrifugo/pull/1085)
* Better performance: Eliminate allocations under active batching scenario coming from Prometheus metrics. See [centrifugal/centrifuge#530](https://github.com/centrifugal/centrifuge/pull/530)
* Better performance: Redis key build optimizations [centrifugal/centrifuge#534](https://github.com/centrifugal/centrifuge/pull/534)

### Fixes

* This release addresses [#1083](https://github.com/centrifugal/centrifugo/issues/1083) by introducing `publication_data_format` mentioned above.

### Miscellaneous

* This release is built with Go 1.25.5.
* Updated dependencies.
* See also the corresponding [Centrifugo PRO release](https://github.com/centrifugal/centrifugo-pro/releases/tag/v6.5.2).
