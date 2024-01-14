Centrifugo is an open-source scalable real-time messaging server. Centrifugo can instantly deliver messages to application online users connected over supported transports (WebSocket, HTTP-streaming, SSE/EventSource, GRPC, SockJS, WebTransport). Centrifugo has the concept of a channel – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Several official client SDKs for browser and mobile development wrap the bidirectional protocol. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev).

## What's changed

### Improvements

* Support for OKP JWKs based on Ed25519, [#756](https://github.com/centrifugal/centrifugo/pull/756)
* New boolean option `global_redis_presence_user_mapping` to drastically improve presence stats performance for channels with large number of active subscribers when using Redis engine. See [#750](https://github.com/centrifugal/centrifugo/issues/750) for the motivation and implementation details. Also, see in [docs](https://centrifugal.dev/docs/server/engines#optimize-getting-presence-stats) 
* Admin web UI now uses `/admin/api/settings` endpoint on start to load admin UI configuration options. Admin web UI status page now does not call `info` API periodically when browser window is not visible (using [visibility browser API](https://developer.mozilla.org/en-US/docs/Web/API/Page_Visibility_API)). Also, some minor CSS style improvements were made.

### Fixes

* Fix nil pointer dereference upon channel regex check inside namespace, [#760](https://github.com/centrifugal/centrifugo/pull/760)

### Misc

* Release is built using Go v1.21.6
* Use grouped dependabot version updates by @j178, [#762](https://github.com/centrifugal/centrifugo/pull/762)
