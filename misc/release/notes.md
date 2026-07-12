Centrifugo is an open-source scalable real-time messaging server. It instantly delivers messages to application online users connected over supported transports (WebSocket, HTTP-streaming, Server-Sent Events (SSE), GRPC, WebTransport). Centrifugo is built around channel subscriptions – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, AI streaming responses, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Official client SDKs are available for JavaScript (browser, Node.js, React Native), Dart/Flutter, Swift, Java, Python, Go, and .NET. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev). For runnable demos see [centrifugal/examples](https://github.com/centrifugal/examples).

## What's changed

### Improvements

* Possibility to choose color accents in admin web UI.

### Fixes

* Admin web UI: add previously missing API commands to the web UI – map API (`map_publish`, `map_remove`, `map_read_state`, `map_read_stream`, `map_stats`, `map_clear`), shared poll API, etc. – and update the embedded web UI ([#1184](https://github.com/centrifugal/centrifugo/pull/1184), commit [`3fc933d9`](https://github.com/centrifugal/centrifugo/commit/3fc933d9)).
* Fix a `WARN` log about a missing `dev` key on start – some options and flags left over from the dev page removed in v6.9.0 were not fully cleaned up ([#1185](https://github.com/centrifugal/centrifugo/pull/1185), commit [`8cee857f`](https://github.com/centrifugal/centrifugo/commit/8cee857f)).
* Admin: drop the undocumented and unused fallback that read the admin auth token from a `token` URL query parameter – the token must be supplied via the `Authorization` header ([#1187](https://github.com/centrifugal/centrifugo/pull/1187), commit [`bc33546f`](https://github.com/centrifugal/centrifugo/commit/bc33546f)).
* Removed an unused map `ordered` option ([#1186](https://github.com/centrifugal/centrifugo/pull/1186), commit [`8752f242`](https://github.com/centrifugal/centrifugo/commit/8752f242)).

### Miscellaneous

* Docs: expanded the [load balancing](https://centrifugal.dev/docs/server/load_balancing) chapter with configs for Envoy, Caddy, and Traefik (in addition to Nginx and HAProxy) – all validated in a [playground](https://github.com/centrifugal/examples/tree/master/v6/proxy_load_balancing) running two nodes behind each proxy across every transport (WebSocket, HTTP streaming, SSE) over plaintext and TLS, up to 10k connections.
* Docs: added interactive widgets throughout the documentation to clarify concepts where newcomers get confused most – for example the [connection JWT explorer](https://centrifugal.dev/docs/server/authentication#try-it-connection-jwt-explorer), [subscription JWT explorer](https://centrifugal.dev/docs/server/channel_token_auth#try-it-subscription-jwt-explorer), [channel resolver](https://centrifugal.dev/docs/server/channels#try-it-channel-resolver), [subscribe permission model](https://centrifugal.dev/docs/server/channel_permissions#subscribe-permission-model), and [interactive proxy explorer](https://centrifugal.dev/docs/server/proxy#interactive-proxy-explorer).
* This release is built with Go 1.26.5
* Dependency updates
* See also the corresponding [Centrifugo PRO release](https://github.com/centrifugal/centrifugo-pro/releases/tag/v6.9.1).
