Centrifugo is an open-source scalable real-time messaging server. Centrifugo can instantly deliver messages to application online users connected over supported transports (WebSocket, HTTP-streaming, SSE/EventSource, GRPC, WebTransport). Centrifugo has the concept of a channel – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Several official client SDKs for browser and mobile development wrap the bidirectional protocol. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev).

## What's changed

### Improvements

* Added option to configure a custom token user ID claim. See [#783](https://github.com/centrifugal/centrifugo/pull/783). Although we still recommend using the `sub` claim, there are scenarios where a different claim name is required. You can now configure this using the [token_user_id_claim](https://centrifugal.dev/docs/server/authentication##custom-token-user-id-claim) option.
* Connection durations are now logged as human-readable strings instead of nanoseconds. See [centrifugal/centrifuge#416](https://github.com/centrifugal/centrifuge/pull/416).

### Fixes

* Fixed Fossil delta construction in recovered publications. See [centrifugal/centrifuge#415](https://github.com/centrifugal/centrifuge/pull/415). This prevents `bad checksum` errors during recovery with delta compression enabled.
* Handled the history meta key eviction scenario to avoid publish errors. See [centrifugal/centrifuge#412](https://github.com/centrifugal/centrifuge/pull/412). This fix addresses [#888](https://github.com/centrifugal/centrifugo/issues/888).
* Full basic auth credentials are no longer displayed in proxy endpoint logs. See [#890](https://github.com/centrifugal/centrifugo/pull/890). The URL is now redacted in logs.
* Fixed panic occurring during PostgreSQL consumer dispatch error handling. See [#889](https://github.com/centrifugal/centrifugo/pull/889).
* Added missing `delta_publish` top-level default definition. See [#896](https://github.com/centrifugal/centrifugo/pull/896). This fix addresses an unknown option warning in logs on Centrifugo startup.

### Miscellaneous

* This release is built with Go 1.23.2.
* Check out the [Centrifugo v6 roadmap](https://github.com/centrifugal/centrifugo/issues/832). It outlines important changes planned for the next major release. We have already started working on v6 and are sharing updates in the issue and our community channels.
* See also the corresponding [Centrifugo PRO release](https://github.com/centrifugal/centrifugo-pro/releases/tag/v5.4.7).
