Centrifugo is an open-source scalable real-time messaging server. Centrifugo can instantly deliver messages to application online users connected over supported transports (WebSocket, HTTP-streaming, Server-Sent Events (SSE/EventSource), GRPC, WebTransport). Centrifugo has the concept of a channel – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Several official client SDKs for browser and mobile development wrap the bidirectional protocol. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev).

## What's changed

See the `Fixes` section for a breaking change in this release (flags vs env priority).

### Improvements

* WebSocket frame ping-pong now inherits values from `client.ping_interval` and `client.pong_timeout` [#1033](https://github.com/centrifugal/centrifugo/pull/1033). This applies to both bidirectional and unidirectional WebSocket transports.
* Support for `cf_connect` in unidirectional WebSocket, similar to what Centrifugo provides for unidirectional SSE [#1033](https://github.com/centrifugal/centrifugo/pull/1033). This allows connecting to the unidirectional WebSocket endpoint without requiring the client to send the first connect message to the server. See the [updated docs](https://centrifugal.dev/docs/transports/uni_websocket#send-connect-request).
* Slightly faster unidirectional WebSocket connection establishment due to reduced allocations [#1033](https://github.com/centrifugal/centrifugo/pull/1033).
* Extrapolate custom environment variables in `MapStringString` config fields [#1034](https://github.com/centrifugal/centrifugo/pull/1034). This helps define secret map values in config via separate environment variables. See the [updated docs for environment variables](https://centrifugal.dev/docs/server/configuration#os-environment-variables).
* Centrifugo Helm chart is now published to GitHub Container Registry. See https://github.com/orgs/centrifugal/packages?repo_name=helm-charts. Contributed by @1995parham.
* Small improvement in the web UI to show milliseconds in request times sent from the Actions page.

### Fixes

* Fix flags priority: flags must override environment variables [#1029](https://github.com/centrifugal/centrifugo/pull/1029). This regression in Centrifugo v6 changes behavior, but it restores the [documented behavior](https://centrifugal.dev/docs/server/configuration#configuration-sources), matches previous versions, and is more natural for software. In general, this should not affect production setups, which rarely use command-line flags.
* Fix `none` log level by using the proper `zerolog` level [#1027](https://github.com/centrifugal/centrifugo/pull/1027). Also warns if the configured log level is incorrect.
* Fix missing WebTransport in usage stats [#1028](https://github.com/centrifugal/centrifugo/pull/1028).

### Miscellaneous

* This release is built with Go 1.24.6.
* Updated dependencies.
* See also the corresponding [Centrifugo PRO release](https://github.com/centrifugal/centrifugo-pro/releases/tag/v6.3.0).
