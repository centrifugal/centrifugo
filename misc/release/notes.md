Centrifugo is an open-source scalable real-time messaging server. Centrifugo can instantly deliver messages to application online users connected over supported transports (WebSocket, HTTP-streaming, SSE/EventSource, GRPC, WebTransport). Centrifugo has the concept of a channel – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Several official client SDKs for browser and mobile development wrap the bidirectional protocol. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev).

## What's changed

This release adds validation of proper `history_ttl` and `history_meta_ttl` configuration. Adding validation means possible errors on Centrifugo start if you have improperly configured history meta TTL expiration. See more details below.

### Improvements

* Better error message for invalid connection token with channel claim, see [#776](https://github.com/centrifugal/centrifugo/issues/776)

### Fixes

* ❗Add validation on Centrifugo start for `history_meta_ttl` option to be greater than or equal to `history_ttl` option. Without this validation your history streams may eventually return error `The ID specified in XADD is equal or smaller than the target stream top item` during publish operation. See [#768](https://github.com/centrifugal/centrifugo/issues/768) for the details. This change won't affect you if you don't have `history_ttl` more than 30 days. Also, documentation about `history_meta_ttl` option was fixed since it contained different default values in different parts of the doc (the default changed in v5 together with [history meta ttl refactoring](https://centrifugal.dev/blog/2023/06/29/centrifugo-v5-released#history_meta_ttl-refactoring), but the doc was not properly updated). 
* When using in-memory broker (default) history meta TTL was not properly inherited from channel namespace configuration – the global one was used instead. Fixed in [centrifugal/centrifuge#366](https://github.com/centrifugal/centrifuge/pull/366)
* Web UI: redirect to the login screen in case of unauthorized errors from server, this fixes a regression introduced by Centrifugo v5.2.1
* Fix setting custom TTL in `gensubtoken` cli, see [#769](https://github.com/centrifugal/centrifugo/pull/769)

### Misc

* Release is built with Go 1.22.1
* All dependencies were updated to latest versions
* Built-in integration with Heroku was removed [#779](https://github.com/centrifugal/centrifugo/pull/779)
* ❗If you use SockJS – pay attention to [#765](https://github.com/centrifugal/centrifugo/issues/765) – in Centrifugo v6 SockJS support will be removed
