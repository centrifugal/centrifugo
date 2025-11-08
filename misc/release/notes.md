Centrifugo is an open-source scalable real-time messaging server. Centrifugo can instantly deliver messages to application online users connected over supported transports (WebSocket, HTTP-streaming, Server-Sent Events (SSE/EventSource), GRPC, WebTransport). Centrifugo has the concept of a channel – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Several official client SDKs for browser and mobile development wrap the bidirectional protocol. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev).

## What's changed

### Improvements

* Experimental WebSocket over HTTP/2 ([RFC 8441](https://www.rfc-editor.org/rfc/rfc8441.html)), [#1061](https://github.com/centrifugal/centrifugo/pull/1061). This makes it possible to open WebSocket "connections" inside a single HTTP/2 connection – using separate HTTP/2 streams for each WS connection. See [WebSocket over HTTP/2 (RFC 8441)](https://centrifugal.dev/docs/transports/websocket#websocket-over-http2-rfc-8441) documentation page for details.
* Support for `tags` and `idempotency_key` in admin Web UI `publish` and `broadcast` API request forms.
* Introduce [StringKeyValues](https://centrifugal.dev/docs/server/configuration#stringkeyvalues-type) configuration type [#1063](https://github.com/centrifugal/centrifugo/pull/1063). A new configuration type allows flexible key-value handling via strings without some drawbacks of maps with Viper. See PR for details. This type is for now used only in Centrifugo PRO, but may be later used for some OSS configuration options as well.
* The `MapStringString` configuration type now may be set over the format similar to `StringKeyValues` type to provide a workaround for Viper map handling limitations.
* Documentation was improved and is now more clear about [non-primitive configuration types](https://centrifugal.dev/docs/server/configuration#configuration-types) and how to set them in different sources.
* Refactor JWT verifier [#1062](https://github.com/centrifugal/centrifugo/pull/1062). The JWT verification logic has been refactored for better maintainability and to avoid repetitive code.

### Miscellaneous

* This release is built with Go 1.25.4.
* Updated dependencies.
* See also the corresponding [Centrifugo PRO release](https://github.com/centrifugal/centrifugo-pro/releases/tag/v6.5.0).
