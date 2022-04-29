Centrifugo is a language-agnostic real-time messaging server. It handles persistent connections from application users (established over WebSocket, HTTP-streaming, SSE/EventSource, GRPC, SockJS transports) and provides API to publish messages to online users in real-time. Centrifugo scales well to load-balance client connections over a cluster of Centrifugo nodes. Chats, live comments, multiplayer games, streaming metrics can be built on top of Centrifugo messaging system. See [Centrifugo docs](https://centrifugal.dev/) for more information.

## Release notes

This release **contains backwards incompatible changes in experimental Tarantool engine** (see details below).

### Improvements

* Support checking `aud` and `iss` JWT claims [#496](https://github.com/centrifugal/centrifugo/pull/496). See more details in docs: [aud](https://centrifugal.dev/docs/server/authentication#aud), [iss](https://centrifugal.dev/docs/server/authentication#iss).
* Channel Publication now has `tags` field (`map[string]string`) – this is a map with arbitrary keys and values which travels with publications. It may help to put some useful info into publication without modifying payload. It can also help to avoid processing payload in some scenarios. Publish and broadcast server APIs got support for setting `tags`. Though supporting this field throughout our ecosystem (for example expose it in all our client SDKs) may take some time. Server API docs for [publish](https://centrifugal.dev/docs/server/server_api#publish) and [broadcast](https://centrifugal.dev/docs/server/server_api#broadcast) commands have been updated.
* Support setting user for Redis ACL-based auth, for Redis itself and for Sentinel. See [in docs](https://centrifugal.dev/docs/server/engines#redis_db).
* Unidirectional transports now return a per-connection generated `session` unique string. This unique string attached to a connection on start, in addition to client ID. It allows controlling unidirectional connections using server API. Previously we suggested using client ID for this – but turns out it's not really a working approach since client ID can be exposed to other users in Publications, presence, join/leave messages. So backend can not distinguish whether user passed its own client ID or not. With `session` which is not shared at all things work in a more secure manner. Server API docs for [subscribe](https://centrifugal.dev/docs/server/server_api#subscribe), [unsubscribe](https://centrifugal.dev/docs/server/server_api#unsubscribe), [disconnect](https://centrifugal.dev/docs/server/server_api#disconnect) and [refresh](https://centrifugal.dev/docs/server/server_api#refresh) commands have been updated.
* Report line and column for JSON config file syntax error – see [#497](https://github.com/centrifugal/centrifugo/issues/497)
* Improve performance (less memory allocations) in message broadcast, during WebSocket initial connect and during disconnect.

### Breaking changes

* **Breaking change in experimental Tarantool integration**. In Centrifugo v3.2.0 we updated code to work with a new version of [tarantool-centrifuge](https://github.com/centrifugal/tarantool-centrifuge). `tarantool-centrifuge` v0.2.0 has an [updated space schema](https://github.com/centrifugal/tarantool-centrifuge/releases/tag/v0.2.0). This means that Centrifugo v3.2.0 will only work with `tarantool-centrifuge` >= v0.2.0 or [rotor](https://github.com/centrifugal/rotor) >= v0.2.0. We do not provide any migration plan for this update – spaces in Tarantool must be created from scratch. We continue considering Tarantool integration experimental.

### Misc

* This release is built with Go 1.17.9.
* We continue working on client protocol v2. Centrifugo v3.2.0 includes more parts of it and includes experimental bidirectional emulation support. More details in [#515](https://github.com/centrifugal/centrifugo/pull/515).
* Check out our progress regarding Centrifugo v4 in [#500](https://github.com/centrifugal/centrifugo/issues/500).
* New community-driven Centrifugo server API library [Centrifugo.AspNetCore](https://github.com/ismkdc/Centrifugo.AspNetCore) for ASP.NET Core released.
