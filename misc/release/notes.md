Centrifugo is an open-source scalable real-time messaging server. Centrifugo can instantly deliver messages to application online users connected over supported transports (WebSocket, HTTP-streaming, Server-Sent Events (SSE/EventSource), GRPC, WebTransport). Centrifugo has the concept of a channel – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Several official client SDKs for browser and mobile development wrap the bidirectional protocol. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev).

## What's changed

### Improvements

* Better configdoc UI [#1092](https://github.com/centrifugal/centrifugo/pull/1092). Redesigned `centrifugo configdoc` interface with top-level navigation, search, JSON/YAML snippets (🔥), and dark/light themes.
* Add `hmac_previous_secret_key` and `hmac_previous_secret_key_valid_until` options to provide a possibility to rotate HMAC token [#1103](https://github.com/centrifugal/centrifugo/pull/1103)
* Adding `json_object` publication data format – more strict format to ensure a JSON object in channels [#1091](https://github.com/centrifugal/centrifugo/pull/1091)
* Centrifugo Helm chart v13 [was released](https://github.com/centrifugal/helm-charts/releases/tag/centrifugo-13.0.0) - comes with many improvements, documentation and examples. 
* Adopt latest `quic-go` and `webtransport-go` changes, WebTransport test [#1101](https://github.com/centrifugal/centrifugo/pull/1101)
* Refactor metrics – makes metrics configurable on server start and discoverable from one place [#1093](https://github.com/centrifugal/centrifugo/pull/1093)

### Fixes

* Redis broker: avoid offset incrementing on publication suppress by version [centrifugal/centrifuge#549](https://github.com/centrifugal/centrifuge/pull/549)
* Add missing mutex Unlock() by @palkan in [centrifugal/centrifuge#552](https://github.com/centrifugal/centrifuge/pull/552)

### Miscellaneous

* This release is built with Go 1.25.7
* Updated dependencies
* See also the corresponding [Centrifugo PRO release](https://github.com/centrifugal/centrifugo-pro/releases/tag/v6.6.1).
