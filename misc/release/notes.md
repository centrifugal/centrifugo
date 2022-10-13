Centrifugo is an open-source scalable real-time messaging server. Centrifugo can instantly deliver messages to application online users connected over supported transports (WebSocket, HTTP-streaming, SSE/EventSource, GRPC, SockJS, WebTransport). Centrifugo has the concept of a channel – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Several official client SDKs for browser and mobile development wrap the bidirectional protocol. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev).

## Release notes

This release contains one more fix of v4 degradation (not respecting `force_push_join_leave` option for top-level namespace), comes with updated admin web UI and other improvements.

### Fixes

* Handle `force_push_join_leave` option set for top-level namespace – it was ignored so join/leave messages were not delivered to clients, [commit](https://github.com/centrifugal/centrifugo/commit/a2409fb7465e348275d87a9d94db5bea5bae357d)

### Improvements

* Updated admin web UI. It now uses modern React stack, fresh look based on Material UI and several other small improvements. See [#566](https://github.com/centrifugal/centrifugo/pull/566) for more details
* Case-insensitive http proxy header configuration [#558](https://github.com/centrifugal/centrifugo/pull/558)
* Use Alpine 3.16 instead of 3.13 for Docker builds, [commit](https://github.com/centrifugal/centrifugo/commit/0c1332ffb335266ce4ff88750985c27263b13de2)
* Add missing empty object results to API command responses, [commit](https://github.com/centrifugal/centrifugo/commit/bdfdc1eeadd99eb4e30162c70accb02b1b1e32d2)
* Disconnect clients in case of inappropriate protocol [centrifugal/centrifuge#256](https://github.com/centrifugal/centrifuge/pull/256)

### Misc

* This release is built with Go 1.19.2
