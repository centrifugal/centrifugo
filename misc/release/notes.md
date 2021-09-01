Centrifugo is a scalable real-time messaging server in a language-agnostic way.

Centrifugo v3 release is targeting to improve Centrifugo adoption for basic real-time application cases, improves server performance and extends existing features with new functionality. It comes with unidirectional real-time transports, protocol speedups, super-fast engine implementation based on Tarantool, new documentation site ([centrifugal.dev](https://centrifugal.dev)), GRPC proxy, API extensions and PRO version which provides unique possibilities for business adopters.

[Centrifugo v3 introductory blog post](https://centrifugal.dev/blog/2021/08/31/hello-centrifugo-v3) describes the most notable aspects of v3.

The release has backward incompatible changes. [Centrifugo v3 migration guide](https://centrifugal.dev/docs/getting-started/migration_v3) aims to help with v2 to v3 migration. Migration guide contains a best-effort configuration converter to adapt the existing configuration file for v3.

Release summary:

* License changed from MIT to Apache 2.0.
* Support for unidirectional transports (Eventsource, GRPC, HTTP-streaming, WebSocket). [Docs](https://centrifugal.dev/docs/transports/overview) and [introductory blog post](https://centrifugal.dev/blog/2021/08/31/hello-centrifugo-v3) provide more information.
* Better performance:
    * Drastically improved JSON client protocol performance, addresses [#460](https://github.com/centrifugal/centrifugo/issues/460)
    * Sharded in-memory connections hub to reduce lock contention
    * Less memory allocations during message broadcast
    * 5% overall additional performance boost due to using Go 1.17
    * More details in the [introductory blog post](https://centrifugal.dev/blog/2021/08/31/hello-centrifugo-v3)
* Enhanced client history API (iteration support, [docs](https://centrifugal.dev/docs/server/history_and_recovery)), breaking change in client history call behavior – history now does not return all existing publications in a channel by default. [Migration guide](https://centrifugal.dev/docs/getting-started/migration_v3) covers this.
* Configuration options cleanups and changes (breaking changes described in details in [migration guide](https://centrifugal.dev/docs/getting-started/migration_v3)).
    * Channel options changes
    * Time intervals now set as human-readable durations
    * Client transport endpoint requests with Origin header set should now explicitly match patterns in `allowed_origins`
    * Improved Redis Engine configuration
    * SockJS disabled by default
    * Some options renamed, and some removed
    * `history_max_publication_limit` and `recovery_max_publication_limit` options added
    * `proxy_binary_encoding` option to give a tip to proxy that data should be encoded into base64.
    * HTTP proxy does not have default headers – should be set explicitly now
* JWT improvements and changes:
    * Possibility to pass a list of server-side subscriptions with channel options
    * Better control on token and connection expiration
    * See [docs](https://centrifugal.dev/docs/server/authentication)
* Redis Engine uses Redis Streams by default. [Migration guide](https://centrifugal.dev/docs/getting-started/migration_v3) covers this.
* GRPC proxy in addition to the existing HTTP proxy. [Docs](https://centrifugal.dev/docs/server/proxy#grpc-proxy).
* Experimental Tarantool Engine. [Docs](https://centrifugal.dev/docs/server/engines#tarantool-engine).
* Enhanced server API functionality:
    * `publish` API now returns offset and epoch for channels with history, fixes [#446](https://github.com/centrifugal/centrifugo/issues/446)
    * Refactor `channels` API call to work in all cases (was unavailable for Redis Cluster and Nats broker before, fixes [#382](https://github.com/centrifugal/centrifugo/issues/382))
    * `b64data` and `skip_history` added to Publish API
    * Enhanced `history` API (iteration)
    * Extended `disconnect` API
    * Extended `unsubscribe` API
    * New `subscribe` API call, fixes [#367](https://github.com/centrifugal/centrifugo/issues/367)
    * Refer to the [docs](https://centrifugal.dev/docs/server/server_api).
* Enhanced proxy functionality
    * Add custom data in subscribe response to a client
    * `skip_history` for publish proxy
    * More control over disconnect
    * Possibility to override subscription related channel options
    * Possibility to attach custom connection `meta` information which won't be visible by clients, addresses [#457](https://github.com/centrifugal/centrifugo/issues/457)
    * Refer to the [docs](https://centrifugal.dev/docs/server/proxy).
* `trace` log level added
* Better clustering – handle node shutdown
* Better GRPC API package name – fixes [#379](https://github.com/centrifugal/centrifugo/issues/379)
* Show total number of different subscriptions on Centrifugo node in Web UI
* Check user ID to be the same as the current user ID during a client-side refresh, fixes [#456](https://github.com/centrifugal/centrifugo/issues/456)
* Drop support for POSTing Protobuf to HTTP API endpoint
* Using Alpine 3.13 as Docker image base
* Various client connector libraries improvements
* Various admin web UI improvements
* Introducing [Centrifugo PRO](https://centrifugal.dev/docs/pro/overview)
