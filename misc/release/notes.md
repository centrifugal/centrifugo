Centrifugo is an open-source scalable real-time messaging server. Centrifugo can instantly deliver messages to application online users connected over supported transports (WebSocket, HTTP-streaming, SSE/EventSource, GRPC, WebTransport). Centrifugo has the concept of a channel – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Several official client SDKs for browser and mobile development wrap the bidirectional protocol. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev).

## What's changed

### Improvements

* Option to configure custom token user id claim. See [#783](https://github.com/centrifugal/centrifugo/pull/783). We still recommend using `sub` claim for that, but there are cases when you need to use a different claim name. Now you can configure it using [token_user_id_claim](https://centrifugal.dev/docs/server/authentication##custom-token-user-id-claim) option.
* Log connection durations as human-readable string instead of nanoseconds [centrifugal/centrifuge#416](https://github.com/centrifugal/centrifuge/pull/416)

### Fixes

* Fix Fossil delta construction in recovered publications [centrifugal/centrifuge#415](https://github.com/centrifugal/centrifuge/pull/415) - prevents `bad checksum` errors during recovery with delta compression on.
* Handle history meta key eviction scenario to avoid publish errors [centrifugal/centrifuge#412](https://github.com/centrifugal/centrifuge/pull/412), fixes [#888](https://github.com/centrifugal/centrifugo/issues/888).
* Avoid showing full basic auth credentials in proxy endpoint logs, [#890](https://github.com/centrifugal/centrifugo/pull/890) - now URL is redacted in logs.
* Fix panic during PostgreSQL consumer dispatch error handling [#889](https://github.com/centrifugal/centrifugo/pull/889).
* Add missing `delta_publish` top-level default definition [#896](https://github.com/centrifugal/centrifugo/pull/896), fixes unknown option warning in logs on Centrifugo start.

### Miscellaneous

* Release is built with Go 1.23.2
* Check out [Centrifugo v6 roadmap](https://github.com/centrifugal/centrifugo/issues/832) issue. It outlines some important changes planned for the next major release. We already started working on v6 and sharing some updates in the issue and our community rooms.
* See also the corresponding [Centrifugo PRO release](https://github.com/centrifugal/centrifugo-pro/releases/tag/v5.4.7).
