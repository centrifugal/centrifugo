Centrifugo is an open-source scalable real-time messaging server. Centrifugo can instantly deliver messages to application online users connected over supported transports (WebSocket, HTTP-streaming, Server-Sent Events (SSE/EventSource), GRPC, WebTransport). Centrifugo has the concept of a channel – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Several official client SDKs for browser and mobile development wrap the bidirectional protocol. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev).

## What's changed

### Improvements

* Adding `has_recovered_publications` label (`yes|no`) to `centrifugo_client_recover` counter metric to distinguish between recoveries that included recovered publications and those which did not (but still succeeded).
* Boolean option `prometheus.recovered_publications_histogram` to enable recovered publications histogram [#1017](https://github.com/centrifugal/centrifuge/pull/1017). The flag enables a histogram to track the distribution of recovered publications number in successful recoveries. This allows to visualize and evaluate the number of publications successfully recovered. The insight can help to fine-tune history settings.

### Fixes

* Fix memory leak in Kafka backpressure mechanism [#1018](https://github.com/centrifugal/centrifuge/pull/1018)
* Fix and improve redis mode logging [#1010](https://github.com/centrifugal/centrifuge/pull/1010) - previously Centrifugo was setting up Redis mode correctly, but logged it wrong. Now it was fixed, and Centrifugo includes more details about Redis mode logging to see the configured setup properties.
* Fix duration overflow due to large `exp` in JWT tokens, max ttl 1 year for connection and subscription token refreshed [centrifugal/centrifuge#495](https://github.com/centrifugal/centrifuge/pull/495). The bug in setting `exp` in JWT could result into Centrifugo timer logic failure (because Centrifuge had `Duration` type overflow, which is approximately 290 years). As a consequence - pings were never sent by Centrifugo. Added a reasonable protection from such cases.

### Miscellaneous

* This release is built with Go 1.24.5.
* Updated dependencies.
* See also the corresponding [Centrifugo PRO release](https://github.com/centrifugal/centrifugo-pro/releases/tag/v6.2.4).
