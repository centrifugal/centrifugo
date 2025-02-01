Centrifugo is an open-source scalable real-time messaging server. Centrifugo can instantly deliver messages to application online users connected over supported transports (WebSocket, HTTP-streaming, SSE/EventSource, GRPC, WebTransport). Centrifugo has the concept of a channel – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Several official client SDKs for browser and mobile development wrap the bidirectional protocol. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev).

## What's changed

### Improvements

* Add development build warning in logs [#934](https://github.com/centrifugal/centrifugo/pull/934). On start, if Centrifugo is built from source without proper version attached (which is done in CI upon release workflow), the warning is now shown in logs.

### Fixes

* Fix not using redis prefix for Redis stream support check [centrifugal/centrifuge#456](https://github.com/centrifugal/centrifuge/pull/456). Addresses issue with Redis ACL, see [#935](https://github.com/centrifugal/centrifugo/issues/935).

### Miscellaneous

* Centrifugo v6 has been recently released 💻✨🔮✨💻. See the details in the [Centrifugo v6 release blog post](https://centrifugal.dev/blog/2025/01/16/centrifugo-v6-released).
* This release is built with Go 1.23.5.
* See also the corresponding [Centrifugo PRO release](https://github.com/centrifugal/centrifugo-pro/releases/tag/v6.0.2).
