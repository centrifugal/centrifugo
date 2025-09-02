Centrifugo is an open-source scalable real-time messaging server. Centrifugo can instantly deliver messages to application online users connected over supported transports (WebSocket, HTTP-streaming, Server-Sent Events (SSE/EventSource), GRPC, WebTransport). Centrifugo has the concept of a channel – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Several official client SDKs for browser and mobile development wrap the bidirectional protocol. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev).

## What's changed

See the `Fixes` section for a possible breaking change in this release.

### Improvements

* WebSocket frame ping pong now inherits values from `client.ping_interval` and `client.pong_timeout` [#1033](https://github.com/centrifugal/centrifugo/pull/1033). This is true for bot bidirectional and unidirectional WebSocket transports.
* Support for `cf_connect` for unidirectional WebSocket, similar to what Centrifugo has for unidirectional SSE, [#1033](https://github.com/centrifugal/centrifugo/pull/1033). This helps to connect to the unidirectional WebSocket endpoint without the requirement to send first connect message from client to server. See [updated docs](https://centrifugal.dev/docs/transports/uni_websocket#send-connect-request).
* Slightly faster unidirectional WebSocket connection establishment due to reduced allocations [#1033](https://github.com/centrifugal/centrifugo/pull/1033)
* Extrapolate custom env variables in `MapStringString` config fields [#1034](https://github.com/centrifugal/centrifugo/pull/1034). This may help to define secret map values in config via separate env variables. See [updated docs for env vars](https://centrifugal.dev/docs/server/configuration#os-environment-variables).

### Fixes

* Fix flags priority: flags must override envs [#1029](https://github.com/centrifugal/centrifugo/pull/1029) - this is a regression in Centrifugo v6. This changes Centrifugo behavior, but fixing this seems a right step – we follow [the documented behavior](https://centrifugal.dev/docs/server/configuration#configuration-sources), it worked like this in the previous Centrifugo versions, more natural for software. And generally this should not affect production setups which rarely use command line flags.
* Fix none log level using proper `zerolog` level [#1027](https://github.com/centrifugal/centrifugo/pull/1027)
* Fix missing webtransport in usage stats [#1028](https://github.com/centrifugal/centrifugo/pull/1028)

### Miscellaneous

* This release is built with Go 1.24.6.
* Updated dependencies.
* See also the corresponding [Centrifugo PRO release](https://github.com/centrifugal/centrifugo-pro/releases/tag/v6.3.0).
