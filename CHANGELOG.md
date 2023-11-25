v5.1.2 and higher
=================

Since Centrifugo v5.1.2 we do not maintain CHANGELOG.md file.

All changes may be found on [Centrifugo releases page](https://github.com/centrifugal/centrifugo/releases) on Github.

v5.1.1
======

### Improvements

* Option to extract client connection user ID from HTTP header [#730](https://github.com/centrifugal/centrifugo/pull/730). See [documentation](https://centrifugal.dev/docs/server/configuration#client_user_id_http_header) for it.
* Speed up channel config operations by using atomic.Value and reduce allocations upon channel namespace extraction by using channel options cache, [#727](https://github.com/centrifugal/centrifugo/pull/727)
* New metrics for the size of messages sent and received by Centrifugo real-time transport. And we finally described all the metrics exposed by Centrifugo in docs - see [Server observability -> Exposed metrics](https://centrifugal.dev/docs/server/observability#exposed-metrics)

### Fixes

* Fix `Lua redis lib command arguments must be strings or integers script` error when calling Redis reversed history and the stream metadata key does not exist, [#732](https://github.com/centrifugal/centrifugo/issues/732)

### Misc

* Dependencies updated (rueidis, quic-go, etc)
* Improved logging for bidirectional emulation transports and unidirectional transports - avoid unnecessary error logs

v5.1.0
======

### Improvements

* Support for EC keys in JWK sets and EC JWTs when using JWKS [#720](https://github.com/centrifugal/centrifugo/pull/720) by @shaunco, [JWKS docs updated](https://centrifugal.dev/docs/server/authentication#json-web-key-support)
* Experimental GRPC proxy subscription streams [#722](https://github.com/centrifugal/centrifugo/pull/722) - this is like [Websocketd](https://github.com/joewalnes/websocketd) but on network steroids 🔥. Streaming request semantics - both unidirectional and bidirectional – is now super-simple to achieve with Centrifugo and GRPC. See additional details about motivation, design, scalability concerns and basic examples in [docs](https://centrifugal.dev/docs/server/proxy_streams)
* Transport error mode for server HTTP and GRPC APIs [#690](https://github.com/centrifugal/centrifugo/pull/690) - read [more in docs](https://centrifugal.dev/docs/server/server_api#transport-error-mode)
* Support GRPC gzip compression [#723](https://github.com/centrifugal/centrifugo/pull/723). GRPC servers Centrifugo has now recognize gzip compression, proxy requests can optionally use compression for calls (see [updated proxy docs](https://centrifugal.dev/docs/server/proxy)).

### Misc

* Release is built with Go 1.21.3
* Dependencies updated (crypto, otel, msgpack, etc)

v5.0.4
======

### Improvements

* Support `expire_at` field of SubscribeResult from Subscribe Proxy [#707](https://github.com/centrifugal/centrifugo/pull/707)
* Option to skip client token signature verification [#708](https://github.com/centrifugal/centrifugo/pull/708)

### Fixes

* Fix connecting to Redis server over unix socket - inherited from [centrifugal/centrifuge#318](https://github.com/centrifugal/centrifuge/pull/318) by @tie

### Misc

* Release is built with Go 1.21.1
* Dependencies updated (centrifuge, quic-go, grpc, and others)

v5.0.3
======

### Improvements

* Add support for GRPC exporter protocol in opentelemetry tracing, by @SinimaWath in [#691](https://github.com/centrifugal/centrifugo/pull/691)

### Misc

* Release is built with Go 1.20.7
* Dependencies updated (rueidis, quic-go, opentelemetry, etc)

v5.0.2
======

### Improvements

* Quiet mode and no expiration for gentoken/gensubtoken cli commands [#681](https://github.com/centrifugal/centrifugo/pull/681) - so token generation using cli helpers is more flexible now
* Add `proxy_static_http_headers` option and `static_http_headers` key for granular proxy [#687](https://github.com/centrifugal/centrifugo/pull/687) - so it's possible to append custom headers to HTTP proxy requests.

### Fixes

* Suppress warnings about k8s env vars, see [issue](https://github.com/centrifugal/centrifugo/issues/678)

### Misc

* Release is built with Go 1.20.7
* Dependencies updated (rueidis, quic-go, crypto, etc)
* Replace `interface{}` with `any` in code base, [#682](https://github.com/centrifugal/centrifugo/pull/682)

v5.0.1
======

* Fix panic upon subscription token validation caused by nil interface comparison, [commit](https://github.com/centrifugal/centrifugo/commit/fe2a92da24d1e8a473e559224fc5c87895713f6a)

v5.0.0
======

In Centrifugo v5 we're phasing out old client protocol support, introducing a more intuitive HTTP API, adjusting token management behaviour in SDKs, improving configuration process, and refactoring the history meta ttl option. As the result you get a cleaner, more user-friendly, and optimized Centrifugo experience.

All the major details about the release may be found in [Centrifugo v5 release announcement](https://centrifugal.dev/blog/2023/06/29/centrifugo-v5-released) in our blog.

We also prepared [Centrifugo v5 migration guide](https://centrifugal.dev/docs/getting-started/migration_v5) which has more specific details about changes.

v4.1.3
======

### Improvements

* Dynamic JWKS endpoint based on iss and aud – implemented in [#638](https://github.com/centrifugal/centrifugo/pull/638), [documented here](https://centrifugal.dev/docs/server/authentication#dynamic-jwks-endpoint)
* Add [redis_force_resp2](https://centrifugal.dev/docs/server/engines#redis_force_resp2) option, [#641](https://github.com/centrifugal/centrifugo/pull/641)
* Document [client_stale_close_delay](https://centrifugal.dev/docs/server/configuration#client_stale_close_delay), make it 10 sec  instead of 25 sec by default, relates [#639](https://github.com/centrifugal/centrifugo/issues/639)

### Misc

* This release is built with Go 1.20.3

v4.1.2
======

### Fixes

* Fix decoding of large protocol messages. The bug was introduced by v4.1.1. See [bug report](https://github.com/centrifugal/centrifugo/issues/603)

v4.1.1
======

### Improvements

* Possibility to disable client protocol v1 using `disable_client_protocol_v1` boolean option. To remind you about client protocol v1 vs v2 migration in Centrifugo v4 take a look at [v3 to v4 migration guide](https://centrifugal.dev/docs/getting-started/migration_v4#client-sdk-migration). Centrifugo v4 uses client protocol v2 by default, all our recent SDKs only support client protocol v2. So if you are using modern stack then you can disable clients to use outdated protocol v1 right now. In Centrifugo v5 support for client protocol v1 will be completely removed, see [Centrifugo v5 roadmap](https://github.com/centrifugal/centrifugo/issues/599).
* New boolean option `disallow_anonymous_connection_tokens`. When the option is set Centrifugo won't accept connections from anonymous users even if they provided a valid JWT. See [#591](https://github.com/centrifugal/centrifugo/issues/591)
* New option `client_connection_rate_limit` to limit the number of new real-time connections Centrifugo may accept per second, see [docs](https://centrifugal.dev/docs/server/configuration#client_connection_rate_limit)
* Implement `sub_refresh` proxy to periodically validate expiring subscriptions over the call from Centrifugo to the backend endpoint, see [#592](https://github.com/centrifugal/centrifugo/issues/592) and [docs](https://centrifugal.dev/docs/server/proxy#sub-refresh-proxy)
* More human-readable tracing logging output (especially in Protobuf protocol case). On the other hand, tracing log level is much more expensive now. We never assumed it will be used in production – so seems an acceptable trade-off.
* Several internal optimizations in client protocol to reduce memory allocations.
* More strict client protocol: only allow one pong message from client to server after receiving ping, disable sending commands over the connection which returned an error to the Connect command

### Fixes

* Fix: slow down subscription dissolver workers while Redis PUB/SUB is unavailable. This solves a CPU usage spike which may happen while Redis PUB/SUB is unavailable and last client unsubscribes from some channel.
* Relative static paths in Centrifugo admin web UI (to fix work behind reverse proxy on sub-path)

v4.1.0
======

### Improvements

* Fully rewritten Redis engine using [rueian/rueidis](https://github.com/rueian/rueidis) library. Many thanks to [@j178](https://github.com/j178) and [@rueian](https://github.com/rueian) for the help. Check out details in our blog post [Improving Centrifugo Redis Engine throughput and allocation efficiency with Rueidis Go library](https://centrifugal.dev/blog/2022/12/20/improving-redis-engine-performance). We expect that new implementation is backwards compatible with the previous one except some timeout options which were not documented, please report issues if any.
* Extended TLS configuration for Redis – it's now possible to set CA root cert, client TLS certs, set custom server name for TLS. See more details in the [updated Redis Engine option docs](https://centrifugal.dev/docs/server/engines#redis-engine-options). Also, it's now possible to provide certificates as strings.

v4.0.5
======

### Fixes

* Fix non-working bidirectional emulation in multi-node case [#590](https://github.com/centrifugal/centrifugo/issues/590)
* Process client channels for no-credentials case also, see issue [#581](https://github.com/centrifugal/centrifugo/issues/581)
* Fix setting `allow_positioning` for top-level namespace, [commit](https://github.com/centrifugal/centrifugo/commit/dbaf01776ff294ee6731cd5422146c0f23107cce)

### Misc

* This release is built with Go 1.19.4

v4.0.4
======

This release contains an important fix of Centrifugo memory leak. The leak happens in all setups which use Centrifugo v4.0.2 or v4.0.3.

### Fixes

* Fix goroutine leak on connection close introduced by v4.0.2, [commit](https://github.com/centrifugal/centrifuge/commit/82107b38a42561ca022d50f7ee2ca038a6f120e9)

### Misc

* This release is built with Go 1.19.3

v4.0.3
======

### Fixes

* Fix insensitive case match for granular proxy headers, [#572](https://github.com/centrifugal/centrifugo/issues/572)

v4.0.2
======

This release contains one more fix of v4 degradation (not respecting `force_push_join_leave` option for top-level namespace), comes with updated admin web UI and other improvements.

### Fixes

* Handle `force_push_join_leave` option set for top-level namespace – it was ignored so join/leave messages were not delivered to clients, [commit](https://github.com/centrifugal/centrifugo/commit/a2409fb7465e348275d87a9d94db5bea5bae357d)
* Properly handle `b64data` in server publish API, [commit](https://github.com/centrifugal/centrifugo/commit/e205bca549c6104b608273e2a9c8a777f0083d92)

### Improvements

* Updated admin web UI. It now uses modern React stack, fresh look based on Material UI and several other small improvements. See [#566](https://github.com/centrifugal/centrifugo/pull/566) for more details
* Case-insensitive http proxy header configuration [#558](https://github.com/centrifugal/centrifugo/pull/558)
* Use Alpine 3.16 instead of 3.13 for Docker builds, [commit](https://github.com/centrifugal/centrifugo/commit/0c1332ffb335266ce4ff88750985c27263b13de2)
* Add missing empty object results to API command responses, [commit](https://github.com/centrifugal/centrifugo/commit/bdfdc1eeadd99eb4e30162c70accb02b1b1e32d2)
* Disconnect clients in case of inappropriate protocol [centrifugal/centrifuge#256](https://github.com/centrifugal/centrifuge/pull/256)

### Misc

* This release is built with Go 1.19.2

v4.0.1
======

This release contains an important fix of v4 degradation (proxying user limited channel) and comes with several nice improvements.

### Fixes

* Avoid proxying user limited channel [#550](https://github.com/centrifugal/centrifugo/pull/550)
* Look at subscription source to handle token subs change [#545](https://github.com/centrifugal/centrifugo/pull/545)

### Improvements

* Configure server-to-client ping/pong intervals [#551](https://github.com/centrifugal/centrifugo/pull/551), [docs](https://centrifugal.dev/docs/transports/overview#pingpong-behavior)
* Option `client_connection_limit` to set client connection limit for a single Centrifugo node [#546](https://github.com/centrifugal/centrifugo/pull/546), [docs](https://centrifugal.dev/docs/server/configuration#client_connection_limit)
* Option `api_external` to expose API handler on external port [#536](https://github.com/centrifugal/centrifugo/issues/536)
* Use `go.uber.org/automaxprocs` to set GOMAXPROCS [#528](https://github.com/centrifugal/centrifugo/pull/528), this may help to automatically improve Centrifugo performance when it's running in an environment with cgroup-restricted CPU resources (Docker, Kubernetes).
* Nats broker: use push format from client protocol v2 [#542](https://github.com/centrifugal/centrifugo/pull/542)

### Misc

* While working on [Centrifuge](https://github.com/centrifugal/centrifuge) lib [@j178](https://github.com/j178) found a scenario where connection to Redis could leak, this was not observed and reported in Centrifugo outside the test suite, but it seems that theoretically connections to Redis from Centrifugo could leak with time if the network between Centrifugo and Redis is unstable. This release contains an updated Redis engine which eliminates this.
* This release is built with Go 1.18.5

v4.0.0
======

New v4 release puts Centrifugo to the next level in terms of client protocol performance, WebSocket fallback simplicity, SDK ecosystem and channel security model. This is a major release with breaking changes according to our [Centrifugo v4 roadmap](https://github.com/centrifugal/centrifugo/issues/500).

Several important documents we have at this point can help you get started with Centrifugo v4:

* Centrifugo v4 [release blog post](https://centrifugal.dev/blog/2022/07/10/centrifugo-v4-released)
* Centrifugo v3 -> v4 [migration guide](https://centrifugal.dev/docs/getting-started/migration_v4)
* Client SDK API [specification](https://centrifugal.dev/docs/transports/client_api)
* Updated [quickstart tutorial](https://centrifugal.dev/docs/getting-started/quickstart)

### Highlights

* New client protocol iteration and unified client SDK API. See client SDK API [specification](https://centrifugal.dev/docs/transports/client_api).
* All SDKs now support all the core features of Centrifugo - see [feature matrix](https://centrifugal.dev/docs/transports/client_sdk#sdk-feature-matrix)
* Our own WebSocket bidirectional emulation layer based on HTTP-streaming and SSE (EventSource). Without sticky session requirement for a distributed case. See details in release post and [centrifuge-js README](https://github.com/centrifugal/centrifuge-js/tree/master#bidirectional-emulation)
* SockJS is still supported but DEPRECATED
* Redesigned, more efficient PING-PONG – see details in release post
* Optimistic subscriptions support (implemented in `centrifuge-js` only at this point) – see details in release post
* Secure by default channel namespaces – see details in release post
* Private channel and subscription JWT concepts revised – see details in release post
* Possibility to enable join/leave, recovery and positioning from the client-side
* Experimental HTTP/3 support - see details in release post
* Experimental WebTransport support - see details in release post
* Avoid sending JSON in WebSocket Close frame reason
* Temporary flag for errors, allows resilient behavior of Subscriptions
* `gensubtoken` and `checksubtoken` helper cli commands as subscription JWT now behaves similar to connection JWT
* Legacy options removed, some options renamed, see [migration guide](https://centrifugal.dev/docs/getting-started/migration_v4) for details.
* `meta` attached to a connection now updated upon connection refresh
* `centrifuge-js` migrated to Typescript
* The docs on [centrifugal.dev](https://centrifugal.dev/) were updated for v4, docs for v3 are still there but under version switch widget.
* Use constant time compare function to compare admin_password and api_key [#527](https://github.com/centrifugal/centrifugo/pull/527)

### Misc

* This release is built with Go 1.18.4

v3.2.3
======

No backwards incompatible changes here.

### Improvements

* Support Debian bullseye DEB package release, drop Debian jessie, [#520](https://github.com/centrifugal/centrifugo/issues/520)

### Fixes

* Fix emitting Join message in dynamic server subscribe case (when calling subscribe server API), [centrifugal/centrifuge#231](https://github.com/centrifugal/centrifuge/issues/231).

v3.2.2
======

No backwards incompatible changes here.

### Fixes

* Fix top-level granular subscribe and publish proxies [#517](https://github.com/centrifugal/centrifugo/issues/517).

### Misc

* This release is built with Go 1.17.11.

v3.2.1
======

No backwards incompatible changes here.

### Improvements

* Centrifugo now periodically sends anonymous usage information (once in 24 hours). That information is impersonal and does not include sensitive data, passwords, IP addresses, hostnames, etc. Only counters to estimate version and installation size distribution, and feature usage. See implementation in [#516](https://github.com/centrifugal/centrifugo/pull/516). Please do not disable usage stats sending without reason. If you depend on Centrifugo – sure you are interested in further project improvements. Usage stats help us understand Centrifugo use cases better, concentrate on widely-used features, and be confident we are moving in the right direction. Developing in the dark is hard, and decisions may be non-optimal. See [docs](https://centrifugal.dev/docs/next/server/configuration#anonymous-usage-stats) for more details.

### Misc

* We continue working on Centrifugo v4, look at our [v4 roadmap](https://github.com/centrifugal/centrifugo/issues/500) where the latest updates are shared. BTW Centrifugo v3 already has code to work over new protocol which we aim to make default in v4. It's already possible to try out our own bidirectional emulation layer with HTTP-streaming and Eventsource transports. Don't hesitate reaching out if you depend on Centrifugo and want to understand more what's coming in next major release. We are actively collecting feedback at the moment.
* This release is built with Go 1.17.10.

v3.2.0
======

This release **contains backwards incompatible changes in experimental Tarantool engine** (see details below).

### Improvements

* Support checking `aud` and `iss` JWT claims [#496](https://github.com/centrifugal/centrifugo/pull/496). See more details in docs: [aud](https://centrifugal.dev/docs/server/authentication#aud), [iss](https://centrifugal.dev/docs/server/authentication#iss).
* Channel Publication now has `tags` field (`map[string]string`) – this is a map with arbitrary keys and values which travels with publications. It may help to put some useful info into publication without modifying payload. It can also help to avoid processing payload in some scenarios. Publish and broadcast server APIs got support for setting `tags`. Though supporting this field throughout our ecosystem (for example expose it in all our client SDKs) may take some time. Server API docs for [publish](https://centrifugal.dev/docs/server/server_api#publish) and [broadcast](https://centrifugal.dev/docs/server/server_api#broadcast) commands have been updated.
* Support setting user for Redis ACL-based auth, for Redis itself and for Sentinel. See [in docs](https://centrifugal.dev/docs/server/engines#redis_user).
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

v3.1.1
======

No backwards incompatible changes here.

Improvements:

* Massive JSON client protocol performance improvements in decoding multiple commands in a single frame. See [#215](https://github.com/centrifugal/centrifuge/pull/215) for details.
* General JSON client protocol performance improvements for unmarshalling messages (~8-10% according to [#215](https://github.com/centrifugal/centrifuge/pull/215))
* Subscribe proxy can now proxy custom `data` from a client passed in a subscribe command.

This release is built with Go 1.17.4.

v3.1.0
======

No backwards incompatible changes here.

Improvements:

* Introducing a [granular proxy mode](https://centrifugal.dev/docs/server/proxy#granular-proxy-mode) for a fine-grained proxy configuration. Some background can be found in [#477](https://github.com/centrifugal/centrifugo/issues/477).

Also check out new tutorials in our blog (both examples can be run with single `docker compose up` command):

* [Centrifugo integration with NodeJS tutorial](https://centrifugal.dev/blog/2021/10/18/integrating-with-nodejs)
* [Centrifugo integration with Django – building a basic chat application](https://centrifugal.dev/blog/2021/11/04/integrating-with-django-building-chat-application)

Centrifugo [dashboard for Grafana](https://grafana.com/grafana/dashboards/13039) was updated and now uses [$__rate_interval](https://grafana.com/blog/2020/09/28/new-in-grafana-7.2-__rate_interval-for-prometheus-rate-queries-that-just-work/) function of Grafana.

This release is built with Go 1.17.3.

v3.0.5
======

No backwards incompatible changes here.

Fixes:

* Fix subscription cleanup on client close. Addresses one more problem found in [this report](https://github.com/centrifugal/centrifugo/issues/486).

v3.0.4
======

No backwards incompatible changes here.

Fixes:

* Fix deadlock during PUB/SUB sync in channels with recovery. Addresses [this report](https://github.com/centrifugal/centrifugo/issues/486).
* Fix `redis_db` option: was ignored previously – [#487](https://github.com/centrifugal/centrifugo/issues/487).

v3.0.3
======

No backwards incompatible changes here.

Fixes:

* Fix passing `data` from subscribe proxy result towards client connection.

This release is built with Go 1.17.2.

v3.0.2
======

No backwards incompatible changes here.

Fixes:

* Fix SockJS data escaping on EventSource fallback. See [igm/sockjs-go#100](https://github.com/igm/sockjs-go/issues/100) for more information. In short – this bug could prevent a message with `%` symbol inside be properly parsed by a SockJS Javascript client – thus not processed by a frontend at all.
* Fix panic on concurrent subscribe to the same channels with recovery feature on. More details in [centrifugal/centrifuge#207](https://github.com/centrifugal/centrifuge/pull/207)

v3.0.1
======

No backwards incompatible changes here.

Fixes:

* Fix proxy behavior for disconnected clients, should be now consistent between HTTP and GRPC proxy types.
* Fix `bufio: buffer full` error when unmarshalling large client protocol JSON messages.
* Fix `unexpected end of JSON input` errors in Javascript client with Centrifugo v3.0.0 when publishing formatted JSON (with new lines).

This release uses Go 1.17.1. We also added more tests for proxy package, thanks to [@silischev](https://github.com/silischev).

v3.0.0
======

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

v2.8.6
======

No backwards incompatible changes here.

Improvements:

* RPM and DEB packages now additionally added to release assets

Fixes:

* Fixes accidentally pushed Docker latest tag from Centrifugo v3 PRO beta.

Centrifugo v2.8.6 based on latest Go 1.16.6

v2.8.5
======

No backwards incompatible changes here.

Improvements:

* Possibility to modify data in publish proxy – see [#439](https://github.com/centrifugal/centrifugo/issues/439) and [updated docs for publish proxy](https://centrifugal.github.io/centrifugo/server/proxy/#publish-proxy)

Fixes:

* Use default timeouts for subscribe and publish proxy (1 second). Previously these proxy had no default timeout at all.

Centrifugo v2.8.5 based on Go 1.16.4

v2.8.4
======

No backwards incompatible changes here.

Improvements:

* New subcommand `serve` to quickly run a static file server

Fixes:

* Fix panic when using connect proxy with a personal channel feature on. See [#436](https://github.com/centrifugal/centrifugo/issues/436)

Centrifugo v2.8.4 based on Go 1.16.3

v2.8.3
======

**Security warning**: take a closer look at new option `allowed_origins` **if you are using connect proxy feature**.

No backwards incompatible changes here.

Improvements:

* Possibility to set `allowed_origins` option ([#431](https://github.com/centrifugal/centrifugo/pull/431)). This option allows setting an array of allowed origin patterns (array of strings) for WebSocket and SockJS endpoints to prevent [Cross site request forgery](https://en.wikipedia.org/wiki/Cross-site_request_forgery) attack. This can be especially important when using [connect proxy](https://centrifugal.github.io/centrifugo/server/proxy/#connect-proxy) feature. If you are using JWT authentication then you should be safe. Note, that since you get an origin header as part of proxy request from Centrifugo it's possible to check allowed origins without upgrading to Centrifugo v2.8.3. See [docs](https://centrifugal.github.io/centrifugo/server/configuration/#allowed_origins) for more details about this new option
* Multi-arch Docker build support - at the moment for `linux/amd64` and `linux/arm64`. See [#433](https://github.com/centrifugal/centrifugo/pull/433)

Centrifugo v2.8.3 based on latest Go 1.16.2, Centrifugo does not vendor its dependencies anymore.

v2.8.2
======

No backwards incompatible changes here.

Improvements:

* [JSON Web Key](https://tools.ietf.org/html/rfc7517) support - see [pull request #410](https://github.com/centrifugal/centrifugo/pull/410) and [description in docs](https://centrifugal.github.io/centrifugo/server/authentication/#json-web-key-support)
* Support ECDSA algorithm for verifying JWT - see [pull request #420](https://github.com/centrifugal/centrifugo/pull/420) and updated [authentication docs chapter](https://centrifugal.github.io/centrifugo/server/authentication/)
* Various documentation clarifications - did you know that you can [use subscribe proxy instead of private channels](https://centrifugal.github.io/centrifugo/server/proxy/#subscribe-proxy) for example?

Fixes:

* Use more strict file permissions for a log file created (when using `log_file` option): `0666` -> `0644`
* Fix [issue](https://github.com/centrifugal/web/issues/36) with opening admin web UI menu on small screens

Other:

* Centrifugo repo [migrated from Travis CI to GH actions](https://github.com/centrifugal/centrifugo/issues/414), `golangci-lint` now works in CI
* Check out [a new community package](https://github.com/denis660/laravel-centrifugo) for Laravel that works with the latest version of framework

v2.8.1
======

No backwards incompatible changes here.

Fixes:

* fix concurrent map access which could result in runtime crash when using presence feature.

v2.8.0
======

Minor backwards incompatible changes here when using `client_user_connection_limit` option – see below.

Centrifugo v2.8.0 has many internal changes that could affect overall performance and latency. In general, we expect better latency between a client and a server, but servers under heavy load can notice a small regression in CPU usage.

Improvements:

* Centrifugo can now maintain a single connection from a user when personal server-side channel used. See [#396](https://github.com/centrifugal/centrifugo/issues/396) and [docs](https://centrifugal.github.io/centrifugo/server/server_subs/#maintain-single-user-connection)
* New option `client_concurrency`. This option allows processing client commands concurrently. Depending on your use case this option has potential to radically reduce latency between a client and Centrifugo. See [detailed description in docs](https://centrifugal.github.io/centrifugo/server/configuration/#client_concurrency)
* When using `client_user_connection_limit` and user reaches max amount of connections Centrifugo will now disconnect client with `connection limit` reason instead of returning `limit exceeded` error. Centrifugo will give a client advice to not reconnect.

Centrifugo v2.8.0 based on latest Go 1.15.5

v2.7.2
======

No backwards incompatible changes here.

Fixes:

* Fix client reconnects due to `InsufficientState` errors. There were two scenarios when this could happen. The first one is using Redis engine with `seq`/`gen` legacy fields (i.e. not using **v3_use_offset** option). The second when publishing a lot of messages in parallel with Memory engine. Both scenarios should be fixed now.
* Fix non-working SockJS transport close with custom disconnect code: this is a regression introduced by v2.6.2

v2.7.1
======

No backwards incompatible changes here.

Fixes:

* Fix non-working websocket close with custom disconnect code: this is a regression introduced by v2.6.2

v2.7.0
======

This release has minor backwards incompatible changes in some Prometheus/Graphite metric names. This means that you may need to adapt your monitoring dashboards a bit. See details below.

Improvements:

* Previously metrics exposed by Centrifuge library (which Centrifugo is built on top of) belonged to `centrifuge` Prometheus namespace. This lead to a situation where part of Centrifugo metrics belonged to `centrifugo` and part to `centrifuge` Prometheus namespaces. Starting from v2.7.0 Centrifuge library specific metrics also belong to `centrifugo` namespace. So the rule to migrate is simple: if see `centrifuge` word in a metric name – change it to `centrifugo`. 
* Refreshed login screen of admin web interface with moving Centrifugo logo on canvas – just check it out!
* New gauge that shows amount of running Centrifugo nodes
* Centrifugal organization just got [the first baker on Opencollective](https://opencollective.com/centrifugal) ❤️. This is a nice first step in making Centrifugo development sustainable.

Fixes:

* Fix `messages_sent_count` counter which did not show control, join and leave messages

**Coming soon 🔥:**

* Official Grafana Dashboard for Prometheus storage is on its way to Centrifugo users. [Track this issue](https://github.com/centrifugal/centrifugo/issues/383) for a status, the work almost finished. 
* Official Centrifugo Helm Chart for Kubernetes. [Track this issue](https://github.com/centrifugal/centrifugo/issues/385) for a status, the work almost finished.

v2.6.2
======

No backwards incompatible changes here.

Improvements:

* Internal refactoring of WebSocket graceful close, should make things a bit more performant (though only in apps which read lots of messages from WebSocket connections)
* Disconnect code is now `uint32` internally
* A bit more performant permission checks for publish, history and presence ops 
* Connect proxy request payload can optionally contain `name` and `version` of client if set on client side, see updated [connect proxy docs](https://centrifugal.github.io/centrifugo/server/proxy/#connect-proxy)
* New blog post [Experimenting with QUIC and WebTransport in Go](https://centrifugal.github.io/centrifugo/blog/quic_web_transport/) in Centrifugo blog

Fixes:

* fix panic on connect in 32-bit ARM builds, see [#387](https://github.com/centrifugal/centrifugo/issues/387)

v2.6.1
======

No backwards incompatible changes here.

Improvements:

* Add `grpc_api_key` option, see [in docs](https://centrifugal.github.io/centrifugo/server/grpc_api/#api-key-authorization)

Fixes:

* Fix Redis Engine errors related to missing epoch in Redis HASH. If you see errors in servers logs, like `wrong Redis reply epoch` or `redigo: nil returned`, then those should be fixed here. Also take a look at v2.5.2 release which contains backport of this fix if you are on v2.5.x release branch.

v2.6.0
======

No backwards incompatible changes here.

Improvements:

* New section in docs – Centrifugo dev blog, check out [first post about scaling WebSocket](https://centrifugal.github.io/centrifugo/blog/scaling_websocket/)
* Possibility to [scale with Nats server](https://centrifugal.github.io/centrifugo/server/engines/#nats-broker) for **unreliable at most once PUB/SUB**
* Subscribe HTTP proxy, [see docs](https://centrifugal.github.io/centrifugo/server/proxy/#subscribe-proxy)
* Publish HTTP proxy, [see docs](https://centrifugal.github.io/centrifugo/server/proxy/#publish-proxy)
* Support for setting Redis Sentinel password, [see in docs](https://centrifugal.github.io/centrifugo/server/engines/#redis-sentinel-for-high-availability)
* Various documentation improvements since previous release

This release based on massively updated [Centrifuge](https://github.com/centrifugal/centrifuge) library, we don't expect problems but since many things were refactored – we suggest to carefully test your app.

v2.5.1
======

No backwards incompatible changes here.

Improvements:

* refreshed [documentation design](https://centrifugal.github.io/centrifugo/)
* new [Quick start](https://centrifugal.github.io/centrifugo/quick_start/) chapter for those who just start working with Centrifugo 
* faster marshal of disconnect messages into close frame texts, significantly reduces amount of memory allocations during server graceful shutdown in deployments with many connections
* one beautiful Centrifugo integration with Symfony framework from our community - [check it out](https://github.com/fre5h/CentrifugoBundle)

Fixes:

* add `Content-Type: application/json` header to outgoing HTTP proxy requests to app backend for better integration with some frameworks. [#368](https://github.com/centrifugal/centrifugo/issues/368)
* fix wrong channel name in Join messages sent to client in case of server-side subscription to many channels
* fix disconnect code unmarshalling after receiving response from HTTP proxy requests, it was ignored previously

v2.5.0
======

No backwards incompatible changes here.

Starting from this release we begin migration to new `offset` `uint64` client-server protocol field for Publication position inside history stream instead of currently used `seq` and `gen` (both `uint32`) fields. This `offset` field will be used in Centrifugo v3 by default. This change required to simplify working with history API, and due to this change history API can be later extended with pagination features.

Our client libraries `centrifuge-js`, `centrifuge-go` and `centrifuge-mobile` were updated to support `offset` field. If you are using these libraries then you can update `centrifuge-js` to at least `2.6.0`, `centrifuge-go` to at least `0.5.0` and `centrifuge-mobile` to at least `0.5.0` to work with the newest client-server protocol. As soon as you upgraded mentioned libraries you can enable `offset` support without waiting for Centrifugo v3 release with `v3_use_offset` option:

```json
{
  "v3_use_offset": true
}
```

All other client libraries except `centrifuge-js`, `centrifuge-go` and `centrifuge-mobile` do not support recovery at this moment and will only work with `offset` field in the future.

It's important to mention that `centrifuge-js`, `centrifuge-go` and `centrifuge-mobile` will continue to work with a server which is using `seq` and `gen` fields for recovery until Centrifugo v3 release. With Centrifugo v3 release those libraries will be updated to only work with `offset` field.

Command `centrifugo genconfig` will now generate config file with `v3_use_offset` option enabled. Documentation has been updated to suggest turning on this option for fresh installations.

Improvements:

* support [Redis Streams](https://redis.io/topics/streams-intro) - radically reduces amount of memory allocations during recovery in large history streams. This also opens a road to paginate over history stream in future releases, see description of new `redis_streams` option [in Redis engine docs](https://centrifugal.github.io/centrifugo/server/engines/#redis-streams)
* support [Redis Cluster](https://redis.io/topics/cluster-tutorial), client-side sharding between different Redis Clusters also works, see more [in docs](https://centrifugal.github.io/centrifugo/server/engines/#redis-cluster)
* faster HMAC-based JWT parsing
* faster Memory engine, possibility to expire history stream metadata (more [in docs](https://centrifugal.github.io/centrifugo/server/engines/#memory-engine))
* releases for Centos 8, Debian Buster, Ubuntu Focal Fossa
* new cli-command `centrifugo gentoken` to quickly generate HMAC SHA256 based connection JWT, [see docs](https://centrifugal.github.io/centrifugo/server/configuration/#gentoken-command)
* new cli-command `centrifugo checktoken` to quickly validate connection JWT while developing application, [see docs](https://centrifugal.github.io/centrifugo/server/configuration/#checktoken-command)

Fixes:

* fix server side subscriptions to private channels (were ignored before)
* fix `channels` counter update frequency in server `info` – this includes how fast `channels` counter updated in admin web interface (previously `num clients` and `num users` updated once in 3 seconds while `num channels` only once in a minute, now `num channels` updated once in 3 seconds too)

This release based on Go 1.14.x

v2.4.0
======

This release is a step towards new interesting possibilities with Centrifugo. It adds server-side subscriptions support and some sugar on top of it. With server-side subscriptions you don't need to call `Subscribe` method on client side at all. Follow release notes to know more.

No backwards incompatible changes here.

Improvements:

* Server-side subscriptions, this functionality requires updating client code so at moment usage is limited to `centrifuge-js`. Also there is a possibility to automatically subscribe user connection to personal notifications channel. More info in [new documentation chapter](https://centrifugal.github.io/centrifugo/server/server_subs/)
* New private subscription JWT `eto` claim - see [its description in docs](https://centrifugal.github.io/centrifugo/server/private_channels/#eto)
* Options to disable WebSocket, SockJS and API handlers – [see docs](https://centrifugal.github.io/centrifugo/server/configuration/#disable-default-endpoints)
* New option `websocket_use_write_buffer_pool` – [see docs](https://centrifugal.github.io/centrifugo/transports/websocket/)
* Metrics now include histograms of requests durations - [pull request](https://github.com/centrifugal/centrifugo/pull/337)
* Add Linux ARM binary release

Fixes:

* Fix unreliable unsubscriptions from Redis PUB/SUB channels under load, now we unsubscribe nodes from PUB/SUB channels over in-memory queue
* Fix `tls_external` option regression

v2.3.1
======

This release contains several improvements to proxy feature introduced in v2.3.0, no backwards incompatible changes here.

Improvements:

* With `proxy_extra_http_headers` configuration option it's now possible to set a list of extra headers that should be copied from original client request to proxied HTTP request - see [#334](https://github.com/centrifugal/centrifugo/issues/334) for motivation and [updated proxy docs](https://centrifugal.github.io/centrifugo/server/proxy/)
* You can pass custom data in response to connect event and this data will be available in `connect` event callback context on client side. See [#332](https://github.com/centrifugal/centrifugo/issues/332) for more details
* Starting from this release `Origin` header is proxied to your backend by default - see [full list in docs](https://centrifugal.github.io/centrifugo/server/proxy/#proxy-headers)

v2.3.0
======

This release is a big shift in Centrifugo possibilities due to HTTP request proxy feature. It was a pretty long term work but the final result opens a new commucation direction: from client to server – see details below.

Release has some internal backwards incompatible changes in Redis engine and deprecations. **Migration must be smooth but we strongly suggest to test your functionality** before running new version in production. Read release notes below for more information.

Improvements:

* It's now possible to proxy some client connection events over HTTP to application backend and react to them in a way you need. For example you can authenticate connection via request from Centrifugo to your app backend, refresh client sessions and answer to RPC calls sent by client over WebSocket or SockJS connections. More information in [new documentation chapter](https://centrifugal.github.io/centrifugo/server/proxy/)
* Centrifugo now supports RSA-based JWT. You can enable this by setting `token_rsa_public_key` option. See [updated authentication chapter](https://centrifugal.github.io/centrifugo/server/authentication/) in docs for more details. Due to this addition we also renamed `secret` option to `token_hmac_secret_key` so it's much more meaningful in modern context. But don't worry - old `secret` option will work and continue to set token HMAC secret key until Centrifugo v3 release (which is not even planned yet). But we adjusted docs and `genconfig` command to use new naming
* New option `redis_sequence_ttl` for Redis engine. It allows to expire internal keys related to history sequnce meta data in Redis – current sequence number in channel and epoch value. See more motivation behind this option in [its description in Redis Engine docs](https://centrifugal.github.io/centrifugo/server/engines/#redis-engine). While adding this feature we changed how sequence and epoch values are stored in Redis - both are now fields of single Redis HASH key. This means that after updating to this version your clients won't recover missed messages - but your frontend application will receive `recovered: false` in subscription context so it should tolerate this loss gracefully recovering state from your main database (if everything done right on your client side of course)
* More validation of configuration file is now performed. Specifically we now check history recovery configuration - see [this issue](https://github.com/centrifugal/centrifuge-js/issues/99) to see how absence of such misconfiguration check resulted in confused Centrifugo behaviour - no messages were received by subscribers
* Go internal logs from HTTP server are now wrapped in our structured logging mechanism - those errors will look as warns in Centrifugo logs now
* Alpine 3.10 instead of Alpine 3.8 as Centrifugo docker image base

v2.2.7
======

Improvements:

* Support passing `api_key` over URL param, see [#317](https://github.com/centrifugal/centrifugo/issues/317) for reasoning behind this feature

v2.2.6
======

This is a quick fix release. Fixes an error on start when `namespaces` not set in configuration file,the bug was introduced in v2.2.5, see [#319](https://github.com/centrifugal/centrifugo/issues/319) for details.

v2.2.5
======

Centrifugo now uses `https://cdn.jsdelivr.net/npm/sockjs-client@1/dist/sockjs.min.js` as default SockJS url. This allows Centrifugo to always be in sync with recent v1 SockJS client. This is important to note because SockJS requires client versions to exactly match on server and client sides when using transports involving iframe. Don't forget that Centrifugo has `sockjs_url` option to set custom SockJS URL to use on server side.

Improvements:

* support setting all configuration options over environment variables in format `CENTRIFUGO_<OPTION_NAME>`. This was available before but starting from this release we will support setting **all** options over env
* show HTTP status code in logs when debug log level on
* option to customize HTTP handler endpoints, see [docs](https://centrifugal.github.io/centrifugo/server/configuration/#customize-handler-endpoints)
* possibility to provide custom key and cert files for GRPC API server TLS, see `centrifugo -h` for a bunch of new options 

Fixes:

* Fix setting `presence_disable_for_client` and `history_disable_for_client` on config top level
* Fix Let's Encrypt integration by updating to ACMEv2 / RFC 8555 compilant acme library, see [#311](https://github.com/centrifugal/centrifugo/issues/311)


v2.2.4
======

No backwards incompatible changes here.

Improvements:

* Improve web interface: show total client information (sum of all client connections on all running nodes)

Fixes:

* Fixes SockJS WebSocket 403 response for cross domain requests: this is a regression in v2.2.3


v2.2.3
======

No backwards incompatible changes here.

Improvements:

* New chapter in docs: [Benchmarking server](https://centrifugal.github.io/centrifugo/misc/benchmark/). This chapter contains information about test stand inside Kubernetes with million WebSocket connections to a server based on Centrifuge library (the core of Centrifugo). It gives some numbers and insights about hardware requirements and scalability of Centrifugo
* New channel and channel namespace options: `presence_disable_for_client` and `history_disable_for_client`. `presence_disable_for_client` allows to make presence available only for server side API. `history_disable_for_client` allows to make history available only for server side API. Previously when enabled presence and history were available for both client and server APIs. Now you can disable for client side. History recovery mechanism if enabled will continue to work for clients anyway even if `history_disable_for_client` is on
* Wait for close handshake completion before terminating WebSocket connection from server side. This allows to gracefully shutdown WebSocket sessions

Fixes:

* Fix crash due to race condition, race reproduced when history recover option was on. See [commit](https://github.com/centrifugal/centrifuge/pull/73/files) with fix details
* Fix lack of `client_anonymous` option. See [#304](https://github.com/centrifugal/centrifugo/issues/304)

This release based on Go 1.13.x

v2.2.2
======

No backwards incompatible changes here.

Improvements:

* Support for `tls-alpn-01` ACME challenge, see [#283](https://github.com/centrifugal/centrifugo/issues/283)

Fixes:

* fix running HTTP server several times when http-01 ACME challenge used, see [#288](https://github.com/centrifugal/centrifugo/issues/288)


v2.2.1
======

This release fixes two regressions introduced by v2.2.0.

Improvements:

* [New documentation chapter](https://centrifugal.github.io/centrifugo/server/grpc_api/) about GRPC API in Centrifugo.

Fixes:

* Fix client disconnect in channels with enabled history but disabled recovery
* Fix wrong Push type sent in Redis engine: Leave message was used where Join required

v2.2.0
======

This release based on latest refactoring of Centrifuge library. The refactoring opens a road for possible interesting improvements in Centrifugo – such as possibility to use any PUB/SUB broker instead of Redis (here is an [example of possible integration](https://github.com/centrifugal/centrifugo/pull/273) with Nats server), or even combine another broker with existing Redis engine features to still have recovery and presence features. Though these ideas are not implemented in Centrifugo yet. Performance of broadcast operations can be slightly decreased due to some internal changes in Centrifuge library. Also take a close look at backwards incompatible changes section below for one breaking change.

Improvements:

* Track client position in channels with `history_recover` option enabled and disconnect in case of insufficient state. This resolves an edge case when messages could be lost in channels with `history_recover` option enabled after node reconnect to Redis (imagine situation when Redis was unavailable for some time but before Centrifugo node reconnects publisher was able to successfully send a message to channel on another node which reconnected to Redis faster). With new mechanism client won't miss messages though can receive them with some delay. As such situations should be pretty rare on practice it should be a reasonable compromise for applications. New mechanism adds more load on Redis as Centrifugo node periodically polls channel history state. The load is linearly proportional to amount of active channels with `history_recover` option on. By default Centrifugo will check client position in channel stream not often than once in 40 seconds so an additional load on Redis should not be too high
* New options for more flexible conrol over exposed endpoint interfaces and ports: `internal_address`, `tls_external`, `admin_external`. See description calling `centrifugo -h`. [#262](https://github.com/centrifugal/centrifugo/pull/262), [#264](https://github.com/centrifugal/centrifugo/pull/264)
* Small optimizations in Websocket and SockjS transports writes
* Server initiated disconnect number metrics labeled with disconnect code 

Backwards incompatible changes:

* This release removes a possibility to set `uid` to Publication over API. This feature was not documented in [API reference](https://centrifugal.github.io/centrifugo/server/api/) and `uid` field does not make sense to be kept on client protocol top level as in Centrifugo v2 it does not serve any internal protocol purpose. This is just an application specific information that can be put into `data` payload

Release based on Go 1.12.x

v2.1.0
======

This release contains changes in metric paths exported to Graphite, you may need to fix your dashboard when upgrading. Otherwise everything in backwards compatible.

Improvements:

* Refactored export to Graphite, you can now control aggregation interval using `graphite_interval` option (in seconds), during refactoring some magical path transformation was removed so now we have more predictable path generation. Though Graphite paths changed with this refactoring
* Web interface rewritten using modern Javascript stack - latest React, Webpack instead of Gulp, ES6 syntax 
* Aggregated metrics also added to `info` command reply. This makes it possible to look at metrics in admin panel too when calling `info` command
* More options can be set over environment variables – see [#254](https://github.com/centrifugal/centrifugo/issues/254)
* Healthcheck endpoint – see [#252](https://github.com/centrifugal/centrifugo/issues/252)
* New important chapter in docs – [integration guide](https://centrifugal.github.io/centrifugo/guide/)
* Support setting `api_key` when using `Deploy on Heroku` button
* Better timeout handling in Redis engine – client timeout is now bigger than default `redis_read_timeout` so application can more reliably handle errors

Fixes:

* Dockerfile had no correct `WORKDIR` set so it was only possible to use absolute config file path, now this is fixed in [this commit](https://github.com/centrifugal/centrifugo/commit/08be85223aa849d9996c16971f9d049125ade50c) 
* Show node version in admin web panel
* Fix possible goroutine leak on client connection close, [commit](https://github.com/centrifugal/centrifuge/commit/a70909c2a2677932fcef0910525ea9497ff9acf2)

v2.0.2
======

**Important** If you are using `rpm` or `deb` packages from packagecloud.io then you have to re-run the [installation method of your choice](https://packagecloud.io/FZambia/centrifugo/install) for Centrifugo repository. This is required to update GPG key used. This is a standard process that all packages hosted on packagecloud should do. 

Improvements:

* Redis TLS connection support - see [issue in Centrifuge lib](https://github.com/centrifugal/centrifuge/issues/23) and [updated docs](https://centrifugal.github.io/centrifugo/server/engines/#redis-engine)
* Do not send Authorization header in admin web interface when insecure admin mode enabled - helps to protect admin interface with basic authorization (see [#240](https://github.com/centrifugal/centrifugo/issues/240))

Fixes:

* Resubscribe only to shard subset of channels after reconnect to Redis ([issue](https://github.com/centrifugal/centrifuge/issues/25))


v2.0.1
======

This release has several fixes and performance improvements

Improvements:

* Use latest SockJS url (SockJS version 1.3) for iframe transports
* Improve performance of massive subscriptions to different channels
* Allow dot in namespace names

Fixes:

* Fix of possible deadlock in Redis Engine when subscribe operation fails
* Fix admin web interface [logout issue](https://github.com/centrifugal/web/issues/14) when session expired
* Fix io timeout error when using Redis Engine with sharding enabled
* Fix `checkconfig` command
* Fix typo in metric name - see [#233](https://github.com/centrifugal/centrifugo/pull/233)

v2.0.0
======

This is a new major version of Centrifugo. New version has some important changes and useful features.

Centrifugo v2 serves the same purpose as Centrifugo v1. Centrifugo v2 is not backwards compatible with v1 – migration to it will require adapting both backend and frontend sides of your application (of course if you decide to migrate).

Centrifugo is now based on new library [centrifuge](https://github.com/centrifugal/centrifuge) for Go language. That library can be used standalone to get even more than Centrifugo server provides – like custom authentication, your own permission management, asynchronous message passing, RPC calls etc.

Highlights of v2:

* Cleaner and more structured client-server protocol defined in protobuf schema. Protocol is more compact because some fields with default values that were sent previously now omitted
* Binary Websocket support (Protobuf). Protobuf allows to transfer data in much more compact and performant way than before. Of course JSON is still the main serialization format
* JWT for authentication and private channel authorization instead of hand-crafted HMAC sign. This means that there is no need in extra libraries to generate connection and subscription tokens. There are [plenty of JWT libraries](https://jwt.io/) for all languages
* Prometheus integration and automatic export of stats to Graphite. Now Centrifugo easily integrates in modern monitoring stack – no need to manually export stats
* Refactored [Javascript](https://github.com/centrifugal/centrifuge-js) (ES6), [Go](https://github.com/centrifugal/centrifuge-go) and [gomobile client](https://github.com/centrifugal/centrifuge-mobile) libraries
* Simplified HTTP API authentication (no request body signing anymore)
* GRPC for server API
* New `presence_stats` API command to get compact presence information - how many clients and unique users in channel
* Structured logging with coloured output during development
* Mechanism to automatically merge several Websocket messages into one to reduce syscall amount thus be more performant under heavy load
* Better recovery algorithm to fix several `recovered` flag false positives
* Goreleaser for automatic releases to Github

Some things were removed from Centrifugo in v2 release:

* Publishing over Redis queue
* Admin websocket endpoint
* Client limited channels
* `history_drop_inactive` channel option now gone
* Websocket prepared message support (though this one can be pushed back at some point).

[New documentation](https://centrifugal.github.io/centrifugo/) contains actual information and tips about migration from v1.

As mentioned above new version uses JWT tokens for authentication and private channel authorization. And there is no API request body signing anymore. This all means that using API clients (like `cent`, `phpcent`, `jscent`, `rubycent`, `gocent` before) is not necessary anymore – you can use any JWT library for your language and just send commands from your code – this is just simple JSON objects. Though API libraries still make sense to simplify integration a bit.

At moment there are no native mobile clients. I.e. `centrifuge-ios` and `centrifuge-android` have not been updated to Centrifugo v2 yet.

v1.8.0
======

No backwards incompatible changes here.

### Features

* package for Ubuntu 18.04
* add Centrifugo `version` to stats output. Also add rusage stime and utime values to metrics. See [#222](https://github.com/centrifugal/centrifugo/issues/222) for details. Thanks to @Sannis for contributions
* expose more configuration options to be set over environment variables. See [commit](https://github.com/centrifugal/centrifugo/commit/bf8655914ef94aaa4b2579d943b64fc63e7b9b08) and [related issue](https://github.com/centrifugal/centrifugo/issues/223)
* more context in debug logs regarding to client connection. See [#201](https://github.com/centrifugal/centrifugo/issues/201)
* fix deb package upgrade - see [#219](https://github.com/centrifugal/centrifugo/issues/219) for details

### Internal

* using Go 1.10.3 for builds

v1.7.9
======

No backwards incompatible changes here.

### Fixes

* fix malformed JSON when using empty `info` in connection refresh request - see [#214](https://github.com/centrifugal/centrifugo/issues/214).

### Features

* support ACME http_01 challenge using new `ssl_autocert_http` boolean option. Centrifugo will serve http_01 ACME challenge on port 80. See [#210](https://github.com/centrifugal/centrifugo/issues/210) for more details. 

### Internal

* using Go 1.10.1 for builds

v1.7.8
======

No backwards incompatible changes here.

### Fixes

* the fix of goroutine leak in 1.7.7 was incomplete - looks like in this release the problem described in [#207](https://github.com/centrifugal/centrifugo/issues/207) gone away.

v1.7.7
======

No backwards incompatible changes here.

### Fixes

* fix goroutine leak due to deadlock, see [#207](https://github.com/centrifugal/centrifugo/issues/207)

### Features

* possibility to set message `uid` via API request - see [#205](https://github.com/centrifugal/centrifugo/pull/205)

### Internal

* do not send `unsubscribe` messages to client on shutdown - it will unsubscribe automatically on disconnect on client side
* using Go 1.10 for builds

v1.7.6
======

No backwards incompatible changes here.

### Fixes

* fix setting config via environment vars - `CENTRIFUGO_` prefix did not work since 1.7.4  

v1.7.5
======

No backwards incompatible changes here.

The only change is using new version of Go for builds (Go 1.9.2). This will allow to analize performance profiles more easily without having to use binaries. See [this new wiki page](https://github.com/centrifugal/centrifugo/wiki/Investigating-performance-issues) about investigating performance issues.

v1.7.4
======

No backwards incompatible changes here.

This release is centered around internal refactoring to detach node from server - see more details in [#186](https://github.com/centrifugal/centrifugo/pull/186).

### Features

* optionally create PID file using `--pid_file` command line option.
* create connections in separate goroutines to slightly improve GC (and therefore reduce memory usage).

### Internal (for developers/contributors)

* Using Go 1.8.3 for builds

v1.7.3
======

No backwards incompatible changes here.

This release built using new version of Go - 1.8.1, previously Centrifugo used Go 1.7.5, so here we benefit from Go evolution improvements - the most notable is improvements in GC pauses which should in turn improve Centrifugo latency. It also reduces memory usage by about 15-20% when websocket compression enabled.

v1.7.2
======

No backwards incompatible changes here.

### Fixes

* fix reusing read and write buffers returned from connection hijack. This was added in previous release but due to the bug in configuration the feature did not work.

v1.7.1
======

No backwards incompatible changes here.

### Fixes

* fix mass resubscribe after several Redis disconnects in a row - more details in [#163](https://github.com/centrifugal/centrifugo/pull/163)

### Features

* update Gorilla Websocket lib - it now tries to reuse buffers returned from Go http library `hijack` method. We adapted Centrifugo default websocket buffer options to utilize this feature (`websocket_read_buffer_size` and `websocket_write_buffer_size` now `0` by default).


v1.7.0
======

This release changes two important aspects of Centrifugo. We expect that it will be fully backwards compatible with previous one in most scenarios until you were using `timestamp` message field somehow.

### What's changed

* integration with Gorilla Websocket [PreparedMessage](https://godoc.org/github.com/gorilla/websocket#PreparedMessage) for raw websocket. We expect it to drastically improve websocket compression case - reducing both memory and CPU in large fan-out scenarios. This change does not affect SockJS in any way.
* `timestamp` field removed from message. See [#147](https://github.com/centrifugal/centrifugo/issues/147) for motivation.
* Several new memory metrics - `node_memory_heap_sys`, `node_memory_heap_alloc`, `node_memory_stack_inuse`

v1.6.5
======

No backwards incompatible changes here.

### Features

* resolve `history_drop_inactive` option edge case (described in [#50](https://github.com/centrifugal/centrifugo/issues/50))
* two new options for autocert: `ssl_autocert_force_rsa` and `ssl_autocert_server_name`. See [docs](https://fzambia.gitbooks.io/centrifugal/content/deploy/certificates.html#automatic-certificates) for description 

### Fixes

* update web interface - in new version we don't show connection endpoints on main page as we can't show them reliably. Final endpoints depend on your production proxy/firewall politics (and port configuration) so we don't try to guess.


v1.6.4
======

No backwards incompatible changes here.

We **consider removing** `timestamp` field from message as it's seems useless and never used by Centrifugo users. Applications that need timestamp for some reason can include it into message JSON payload. If you have any objections please look at [issue #147](https://github.com/centrifugal/centrifugo/issues/147) and write your thoughts against removing this field.

### Features

* configurable websocket compression level - see [updated docs](https://fzambia.gitbooks.io/centrifugal/content/mixed/websocket_compression.html). Bear in mind that compression is still very CPU and memory expensive
* new metric `node_uptime_seconds` - see [updated docs](https://fzambia.gitbooks.io/centrifugal/content/server/stats.html) for stats

### Fixes

* fixes crash when using builtin TLS server - see [#145](https://github.com/centrifugal/centrifugo/issues/145)
* redirect Go std lib logging into our INFO logger

### Internal (for developers/contributors)

* Using Go 1.7.5 for builds
* As soon as Go 1.8 out we will be able to remove `x/net/http2` dependency as standard lib will contain fix for [#145](https://github.com/centrifugal/centrifugo/issues/145)


v1.6.3
======

This release fixes wrong decision made in 1.6.x related to pings. We don't rely on client to server 
pings to disconnect clients anymore, we also moved back SockJS heartbeat frames - i.e. sending them 
from server to client every 25 seconds as before (in Centrifugo < 1.6.0). Recent changes in `centrifuge-js` (version 1.4.2) allowed us to not introduce addition reconnects for SockJS polling 
transports when sending client to server automatic ping. We also updated documentation [chapter about 
pings](https://fzambia.gitbooks.io/centrifugal/content/mixed/ping.html) a bit.

### Fixes

* Random disconnects from Centrifugo when using automatic client to server pings. This is a default 
behaviour so it affects almost everyone who using Centrifugo 1.6.x, fixes https://github.com/centrifugal/centrifugo/issues/142
* Fix writing headers after headers already written in raw websocket endpoint - this remove annoying log line appearing after client can't upgrade connection to Websocket.


v1.6.2
======

### Features

* Use Redis pipelining and single connection for presence/history/channels operations. This increases performance of those operations especially on systems with many CPU cores.
* Homebrew formula to install Centrifugo on MacOS, see README for instructions.
* Update gorilla websocket library - there is one more update for websocket compression: pool flate readers which should increase compression performance.

### Fixes

* Fix calling presence remove for every channel (not only channels with presence option enabled).
* Change subscribing/unsubscribing algorithm to Redis channels - it fixes theoretical possibility of wrong subscribing state in Redis.

### Internal (for developers/contributors)

* We don't use `disconnect` message before closing client connections anymore - we rely on websocket/SockJS close reason now (which is JSON encoded `DisconnectAdvice`). Our js client already handles that reason, so no breaking changes there. Some work required in other clients though to support `reconnect: false` in advice.

v1.6.1
======

This release fixes some configuration problems introduced by v1.6.0 and adds Let's Encrypt support.

### Features

* automatic TLS certificates from Let's Encrypt - see [#133](https://github.com/centrifugal/centrifugo/issues/133) and new [dedicated documentation chapter](https://fzambia.gitbooks.io/centrifugal/content/deploy/certificates.html)
* websocket compression performance improvement (due to Gorilla Websocket library update)

### Fixes

* fix SSL/TLS certificates file option names - see [#132](https://github.com/centrifugal/centrifugo/issues/132)
* fix `web` option that must enable admin socket automatically - see [#136](https://github.com/centrifugal/centrifugo/issues/136)

v1.6.0
======

This Centrifugo release is a massive 4-months refactoring of internals with the goal to separate code of different components such as engine, server, metrics, clients to own packages with well-defined API to communicate between them. The code layout changed dramatically. Look at `libcentrifugo` folder [before](https://github.com/centrifugal/centrifugo/tree/v1.5.1/libcentrifugo) and [after](https://github.com/centrifugal/centrifugo/tree/master/libcentrifugo)! Unfortunately there are backwards incompatibilities with previous release - see notes below. The most significant one is changed metrics format in `stats` and `node` API command responses.

With new code layout it's much more simple to create custom engines or servers – each with own metrics and configuration options. **We can not guarantee** though that we will keep `libcentrifugo` packages API stable – **our primary goal is still building Centrifugo standalone server**. So if we find something that must be fixed or improved internally - we will fix/improve it even if this could result in packages API changes.

As Centrifugo written in Go the only performant way to write plugins is to import them in `main.go` file and build Centrifugo with them. So if you want to create custom build with custom server or custom engine you will need to change `main.go` file and build Centrifugo yourself. But at least it's easier than supporting full Centrifugo fork.

### Release highlights:

* New metrics. Several useful new metrics have been added. For example HTTP API and client request HDR histograms. See updated documentation for complete list. Refactoring resulted in backwards incompatible issue when working with Centrifugo metrics (see below). [Here is a docs chapter](https://fzambia.gitbooks.io/centrifugal/content/server/stats.html) about metrics.
* Optimizations for client side ping, `centrifuge-js` now automatically sends periodic `ping` commands to server. Centrifugo checks client's last activity time and closes stale connections. Builtin SockJS server won't send heartbeat frames to SockJS clients by default. You can restore the old behaviour though: setting `ping: false` on client side and `sockjs_heartbeat_delay: 25` option in Centrifugo configuration. This all means that you better update `centrifuge-js` client to latest version (`1.4.0`). Read [more about pings in docs](https://fzambia.gitbooks.io/centrifugal/content/mixed/ping.html).
* Experimental websocket compression support for raw websockets - see [#115](https://github.com/centrifugal/centrifugo/issues/115). Read more details how to enable it [in docs](https://fzambia.gitbooks.io/centrifugal/content/mixed/websocket_compression.html). Keep in mind that enabling websocket compression can result in slower Centrifugo performance - depending on your load this can be noticeable.
* Serious improvements in Redis API queue consuming. There was a bottleneck as we used BLPOP command to get every message from Redis which resulted in extra RTT. Now it's fixed and we can get several API messages from queue at once and process them. The format of Redis API queue changed - see new format description [in docs](https://fzambia.gitbooks.io/centrifugal/content/server/engines.html). Actually it's now the same as single HTTP API command - so we believe you should be comfortable with it. Old format is still supported but **DEPRECATED** and will be removed in next releases.
* Redis sharding support. See more details [in docs](https://fzambia.gitbooks.io/centrifugal/content/server/scaling.html). This resolves some fears about Redis being bottleneck on some large Centrifugo setups. Though we have not heard such stories yet. Redis is single-threaded server, it's insanely fast but if your Redis approaches 100% CPU usage then this sharding feature is what can help your application to scale. 
* Many minor internal improvements.

### Fixes:

* This release fixes crash when `jsonp` transport was used by SockJS client - this was fixed recently in sockjs-go library.
* Memory bursts fixed on node shutdown when we gracefully disconnect many connected clients.

### Backwards incompatible changes:

* `stats` and `node` command response body format changed – metrics now represented as map containing string keys and integer values. So you may need to update your monitoring scripts.
* Default log level now is `info` - `debug` level is too chatty for production logs as there are tons of API requests per second, tons of client connect/disconnect events. If you still want to see logs about all connections and API requests - set `log_level` option to `debug`.
* `channel_prefix` option renamed to `redis_prefix`.
* `web_password` and `web_secret` option aliases not supported anymore. Use `admin_password` and `admin_secret`.
* `insecure_web` option removed – web interface now available without any password if `insecure_admin` option enabled. Of course in this case you should remember about protecting your admin endpoints with firewall rules. Btw we recommend to do this even if you are using admin password.
* Admin `info` response format changed a bit - but this most possibly will not affect anyone as it was mostly used by embedded web interface only.
* Some internal undocumented options have been changed. Btw [we documented several useful options](https://fzambia.gitbooks.io/centrifugal/content/server/advanced_configuration.html) which can be helpful in some cases.

### Several internal highlights (mostly for Go developers):

* Code base is more simple and readable now.
* Protobuf v3 for message schema (using gogoprotobuf library). proto2 and proto3 are wire compatible.
* Client transport now abstracted away - so it would be much easier in future to add new transport in addition/replacement to Websocket/SockJS.
* API abstracted away from protocol - it would be easier in future to add new API requests source.
* No performance penalty was introduced during this refactoring.
* We process PUB/SUB messages in several goroutines, preserving message order.
* We use `statik` (https://github.com/rakyll/statik) to embed web interface now.
* go1.7.4 used for builds.


v1.5.1
======

* Fixes [#94](https://github.com/centrifugal/centrifugo/issues).


v1.5.0
======

Some upgrade steps needed to migrate to this release, see below.

This release is a major refactoring of Centrifugo internal engine. Centrifugo now uses `protobuf` format while transferring data between nodes and serializing history/presence data in Redis. We expect that all those changes **should not affect your working application code** though as new serialization format used **you may need to run** `FLUSHDB` command if using Redis engine to remove presence/history data encoded as JSON (if you don't use presence and history then no need to do this). If you have any problems with new serialization format used by Centrifugo internally – we can consider making old JSON encoding optional in future releases. Depending on how you use Centrifugo with Redis Engine those internal changes can result in nearly the same performance (if real-time content mostly generated by users online, so messages coming to Centrifugo have to be delivered to at least one client) or up to 2x API request speed up (if your real-time mostly generated by application backend itself so some messages published in channel with no active subscribers and there is no need to do JSON encoding work).

* Using SockJS 1.1 by default (actually as by default we use jsdelivr.net CDN version it will be latest minor release - at moment of writing 1.1.1) – this means that you need to also upgrade SockJS on client side to the same version (because **some iframe-based SockJS transports require exact version match on client and server**). Note that you can set desired SockJS version used by Centrifugo server via `sockjs_url` option (which is now by default `//cdn.jsdelivr.net/sockjs/1.1/sockjs.min.js`). So if you don't want to upgrade SockJS on client side or just want to fix version used or use your own hosted SockJS library (which is good if you think about old browser support and don't want to be affected by minor SockJS library releases) - use that option.
* `centrifugo version` now shows Go language version used to build binary.
* Performance and memory allocation improvements.


v1.4.5
======

No backwards incompatible changes here. This release uses go1.6.2

* HTTP/2 support. This means that Centrifugo can utilize HTTP/2 protocol automatically - this is especially useful for HTTP-based SockJS transports (no limits for open connections per domain anymore). Note that HTTP/2 will work only when your setup utilizes `https`. Also HTTP/2 does not affect websockets because of missing protocol upgrade possibilities in HTTP/2 protocol - so websockets will work in the same way as before. Also if you have any proxy before Centrifugo then depending on your setup some reconfiguration may be required to make HTTP/2 work.

Just to remember how to test Centrifugo with SSL: [follow instructions](https://devcenter.heroku.com/articles/ssl-certificate-self) from Heroku article to generate self-signed certificate files. Then start Centrifugo like this:

```
./centrifugo --config=config.json --web --ssl --ssl_key=server.key --ssl_cert=server.crt
```

Go to https://localhost:8000/ and confirm that you trust certificate of this site (this is because of self-signed certificate, in case of valid certificate you don't need this step). Then you can test secure endpoint connections.


v1.4.4
======

One more fix for v1.4.2 release here

* proper aliasing of `admin_password` and `admin_secret` configuration options. See [#88](https://github.com/centrifugal/centrifugo/issues/88) 


v1.4.3
======

**Fix of security vulnerability introduced in v1.4.2**, see below.

* If you are using Centrifugo v1.4.2 (previous versions not affected) with admin socket enabled (with `--admin` or `--web` options) and your admin endpoint not protected by firewall somehow then you must update to this version. Otherwise it's possible to connect to admin websocket endpoint and run any command without authentication. It's recommended to update your secret key after upgrade. So sorry for this.


v1.4.2
======

* Redis Sentinel support for Redis high availability setup. [Docs](https://fzambia.gitbooks.io/centrifugal/content/deploy/sentinel.html)
* Redis Engine now uses Redis pipeline for batching publish operations - this results in latency and throughput improvments when publish rate is high.
* Refactored admin websocket. New option `admin` to enable admin websocket. New option `insecure_admin` to make this endpoint insecure (useful when admin websocket endpoint/port protected by firewall rules). `web_password` option renamed to `admin_password`, `web_secret` option renamed to `admin_secret`, `insecure_web` renamed to `insecure_admin`. **But all old option names still supported to not break things in existing setups**. Also note, that when you run Centrifugo with `web` interface enabled - you also make admin websocket available, because web interface uses it. A little more info [in pull request](https://github.com/centrifugal/centrifugo/pull/83).
* Presence Redis Engine methods rewritten to lua to be atomic.
* Some Redis connection params now can be set over environment variables. See [#81](https://github.com/centrifugal/centrifugo/issues/81)
* Fix busy loop when attempting to reconnect to Redis. Fixes large CPU usage while reconnecting.
* Shorter message `uid`s (22 bytes instead of 36). This was made in order to get some performance improvements.


v1.4.1
======

* fix server crash on 32-bit architectures (due to [this](https://golang.org/src/sync/atomic/doc.go?s=1207:1656#L36)), see more details in [#74](https://github.com/centrifugal/centrifugo/issues/74).
* fix compatibility with gocent introduced in v1.4.0


v1.4.0
======

No backwards incompatible changes here for most usage scenarios, but look carefully on notes below.

* Timers in metrics marked as deprecated. `time_api_mean`, `time_client_mean`, `time_api_max`, `time_client_max` now return 0. This was made because timer's implementation used `Timer` from `go-metrics` library that does not suit very well for Centrifugo needs - so values were mostly useless in practice. So we decided to get rid of them for now to not confuse our users.
* New `node` API method to get information from single node. That information will contain counters without aggregation over minute interval (what `stats` method does by default). So it can be useful if your metric aggregation system can deal with non-aggregated counters over time period itself. Also note that to use this method you should send API request to each Centrifugo node separately - as this method returns current raw statistics about one node. See [issue](https://github.com/centrifugal/centrifugo/issues/68) for motivation description.
* Centrifugo now handles SIGTERM in addition to SIGINT and makes `shutdown` when this signal received. During shutdown Centrifugo returns 503 status code on requests to handlers and closes client connections so clients will reconnect. If shutdown finished without errors in 10 seconds interval then Centrifugo exits with status code 0 (instead of 130 before, this fixes behaviour behind `systemd` after SIGTERM received).
* Maximum limit in bytes for client request was added. It can be changed using `client_request_max_size` config option. By default 65536 bytes (64kb).
* Packages for 64-bit Debian, Centos and Ubuntu [hosted on packagecloud.io](https://packagecloud.io/FZambia/centrifugo). If you are using Debian 7 or 8, Centos 6 or 7, Ubuntu 14.04 or Ubuntu 16.04 - you can find packages for those linux distribution following to packagecloud. Packages will be created every time we release new Centrifugo version.


v1.3.3
======

No backwards incompatible changes here

* fix automatic presence expire in Redis engine - could lead to small memory leaks in Redis when using presence. Also could result in wrong presence information after non-graceful Centrifugo node shutdown.
* configurable limit for amount of channels each client can subscribe to. Default `100`. Can be changed using `client_channel_limit` configuration option.


v1.3.2
======

This release built using go 1.5.3 and [includes security fix in Go lang](https://groups.google.com/forum/#!topic/golang-announce/MEATuOi_ei4)

* empty errors not included in client response (**this requires using Javascript client >= 1.1.0**)
* optimization in Redis engine when using history - one round trip to Redis to publish message and save it into history instead of two. This was done over registering lua script on server start.
* client errors improvements - include error advice when error occurred (fix or retry at moment)

Also note that Javascript client will be fully refreshed soon. See [this pull request](https://github.com/centrifugal/centrifuge-js/pull/7)

v1.3.1
======

* fix port configuration introduced in v1.3.0: `--port` should override default values for `admin_port` and `api_port`
* use the same (http/https) scheme for Sockjs default iframe script source.
* fix possible deadlock in shutdown

v1.3.0
======

Possible backwards incompatibility here (in client side code) - see first point.

* omit fields in message JSON if field contains empty value: `client` on top level, `info` on top level, `default_info` in `info` object, `channel_info` in `info` object. This also affects top level data in join/leave messages and presence data – i.e. `default_info` and `channel_info` keys not included in JSON when empty. This can require adapting your client side code a bit if you rely on these keys but for most cases this should not affect your application. But we strongly suggest to test before updating. This change allows to reduce message size. See migration notes below for more details.
* new option `--admin_port` to bind admin websocket and web interface to separate port. [#44](https://github.com/centrifugal/centrifugo/issues/44)
* new option `--api_port` to bind API endpoint to separate port. [#44](https://github.com/centrifugal/centrifugo/issues/44)
* new option `--insecure_web` to use web interface without setting `web_password` and `web_secret` (for use in development or when you protected web interface by firewall rules). [#44](https://github.com/centrifugal/centrifugo/issues/44)
* new channel option `history_drop_inactive` to drastically reduce resource usage (engine memory, messages travelling around) when you use message history. See [#50](https://github.com/centrifugal/centrifugo/issues/50)
* new Redis engine option `--redis_api_num_shards`. This option sets a number of Redis shard queues Centrifugo will use in addition to standard `centrifugo.api` queue. This allows to increase amount of messages you can publish into Centrifugo and preserve message order in channels. See [#52](https://github.com/centrifugal/centrifugo/issues/52) and [documentation](https://fzambia.gitbooks.io/centrifugal/content/server/engines.html) for more details.
* fix race condition resulting in client disconnections on high channel subscribe/unsubscribe rate. [#54](https://github.com/centrifugal/centrifugo/issues/54)
* refactor `last_event_id` related stuff to prevent memory leaks on large amount of channels. [#48](https://github.com/centrifugal/centrifugo/issues/48)
* send special disconnect message to client when we don't want it to reconnect to Centrifugo (at moment to client sending malformed message). 
* pong wait handler for raw websocket to detect non responding clients.

Also it's recommended to update javascipt client to latest version as it has some useful changes (see its changelog).

How to migrate
--------------

Message before:

```json
{
	"uid":"442586d4-688c-4a0d-52ad-d0a13d201dfc",
	"timestamp":"1450817253",
	"info": null,
	"channel":"$public:chat",
	"data":{"input":"1"},
	"client":""
}
```

Message now:

```json
{
	"uid":"442586d4-688c-4a0d-52ad-d0a13d201dfc",
	"timestamp":"1450817253",
	"channel":"$public:chat",
	"data":{"input":"1"}
}
```

I.e. not using empty `client` and `info` keys. If those keys are non empty then they present in message.

Join message before:

```json
{
	"user":"2694",
	"client":"93615872-4e45-4da2-4733-55c955133436",
	"default_info": null,
	"channel_info":null
}
```

Join message now:

```json
{
	"user":"2694",
	"client":"93615872-4e45-4da2-4733-55c955133436"
}
```

If "default_info" or "channel_info" exist then they would be included:

```json
{
	"user":"2694",
	"client":"93615872-4e45-4da2-4733-55c955133436",
	"default_info": {"username": "FZambia"},
	"channel_info": {"extra": "some data here"}
}
```


v1.2.0
======

No backwards incompatible changes here.

* New `recover` option to automatically recover missed messages based on last message ID. See [pull request](https://github.com/centrifugal/centrifugo/pull/42) and [chapter in docs](https://fzambia.gitbooks.io/centrifugal/content/server/recover.html) for more information. Note that you need centrifuge-js >= v1.1.0 to use new `recover` option
* New `broadcast` API method to send the same data into many channels. See [issue](https://github.com/centrifugal/centrifugo/issues/41) and updated [API description in docs](https://fzambia.gitbooks.io/centrifugal/content/server/api.html)
* Dockerfile now checks SHA256 sum when downloading release zip archive.
* release built using Go 1.5.2


v1.1.0
======

No backwards incompatible changes here.

* support enabling web interface over environment variable CENTRIFUGO_WEB
* close client's connection after its message queue exceeds 10MB (default, can be modified using `max_client_queue_size` configuration file option)
* fix theoretical server crash on start when reading from redis API queue


v1.0.0
======

A bad and a good news here. Let's start with a good one. Centrifugo is still real-time messaging server and just got v1.0 release. The bad – it is not fully backwards compatible with previous versions. Actually there are three changes that ruin compatibility. If you don't use web interface and private channels then there is only one change. But it affects all stack - Centrifugo itself, client library and API library.

Starting from this release Centrifugo won't support multiple registered projects. It will work with only one application. You don't need to use `project key` anymore. Changes resulted in simplified
configuration file format. The only required option is `secret` now. See updated documentation 
to see how to set `secret`. Also changes opened a way for exporting Centrifugo node statistics via HTTP API `stats` command.

As this is v1 release we'll try to be more careful about backwards compatibility in future. But as we are trying to make a good software required changes will be done if needed.

Highlights of this release are:

* Centrifugo now works with single project only. No more `project key`. `secret` the only required configuration option.
* web interface is now embedded, this means that when downloading release you get possibility to run Centrifugo web interface just providing `--web` flag to `centrifugo` when starting process.
* when `secret` set via environment variable `CENTRIFUGO_SECRET` then configuration file is not required anymore. But note, that when Centrifugo configured via environment variables it's not possible to reload configuration sending HUP signal to process.
* new `stats` command added to export various node stats and metrics via HTTP API call. Look its response example [in docs chapter](https://fzambia.gitbooks.io/centrifugal/content/server/api.html).
* new `insecure_api` option to turn on insecure HTTP API mode. Read more [in docs chapter](https://fzambia.gitbooks.io/centrifugal/content/mixed/insecure_mode.html).
* minor clean-ups in client protocol. But as protocol incapsulated in javascript client library you only need to update centrifuge-js.
* release built using Go 1.5.1

[Documentation](https://fzambia.gitbooks.io/centrifugal/content/) was updated to fit all these release notes. Also all API and client libraries were updated – Javascript browser client (`centrifuge-js`), Python API client (`cent`), Django helper module (`adjacent`). API clients for Ruby (`centrifuge-ruby`) and PHP (`phpcent`) too. Admin web interface was also updated to support changes introduced here.

There are 2 new API libraries: [gocent](https://github.com/centrifugal/gocent) and [jscent](https://github.com/centrifugal/jscent). First for Go language. And second for NodeJS.

Also if you are interested take a look at [centrifuge-go](https://github.com/centrifugal/centrifuge-go) – experimental Centrifugo client for Go language. It allows to connect to Centrifugo from non-browser environment. Also it can be used as a reference to make a client in another language (still hoping that clients in Java/Objective-C/Swift to use from Android/IOS applications appear one day – but we need community help here).

How to migrate
--------------

* Use new versions of Centrifugal libraries - browser client and API client. Project key not needed in client connection parameters, in client token generation, in HTTP API client initialization.
* Another backwards incompatible change related to private channel subscriptions. Actually this is not related to Centrifugo but to Javascript client but it's better to write about it here. Centrifuge-js now sends JSON (`application/json`) request instead of `application/x-www-form-urlencoded` when client wants to subscribe on private channel. See [in docs](https://fzambia.gitbooks.io/centrifugal/content/mixed/private_channels.html) how to deal with JSON in this case.
* `--web` is now a boolean flag. Previously it was used to set path to admin web interface. Now it indicates whether or not Centrifugo must serve web interface. To provide path to custom web application directory use `--web_path` string option.

I.e. before v1 you started Centrifugo like this to use web interface:

```
centrifugo --config=config.json --web=/path/to/web/app
```

Now all you need to do is run:

```
centrifugo --config=config.json --web
```

And no need to download web interface repository at all! Just run command above and check http://localhost:8000.

If you don't want to use embedded web interface you can still specify path to your own web interface directory:

```
centrifugo --config=config.json --web --web_path=/path/to/web/app
```


v0.3.0
======

* new `channels` API command – allows to get list of active channnels in project at moment (with one or more subscribers).
* `message_send_timeout` option default value is now 0 (last default value was 60 seconds) i.e. send timeout is not used by default. This means that Centrifugo won't start lots of goroutines and timers for every message sent to client. This helps to drastically reduce memory allocations. But in this case it's recommended to keep Centrifugo behind properly configured reverse proxy like Nginx to deal with connection edge cases - slow reads, slow writes etc.
* Centrifugo now sends pings into pure Websocket connections. Default value is 25 seconds and can be adjusted using `ping_interval` configuration option. Note that this option also sets SockJS heartbeat messages interval. This opens a road to set reasonable value for Nginx `proxy_read_timeout` for `/connection` location to mimic behaviour of `message_send_timeout` which is now not used by default
* improvements in Redis Engine locking.
* tests now require Redis instance running to test Redis engine. Tests use Redis database 9 to run commands and if that database is not empty then tests will fail to prevent corrupting existing data.
* all dependencies now vendored.

v0.2.4
======

* HTTP API endpoint now can handle json requests. Used in client written in Go at moment. Old behaviour have not changed, so this is absolutely optional.
* dependency packages updated to latest versions - websocket, securecookie, sockjs-go.

v0.2.3
======

Critical bug fix for Redis Engine!

* fixed bug when entire server could unsubscribe from Redis channel when client closed its connection.


v0.2.2
======

* Add TLS support. New flags are:
  * `--ssl`                   - accept SSL connections. This requires an X509 certificate and a key file.
  * `--ssl_cert="file.cert"`  - path to X509 certificate file.
  * `--ssl_key="file.key"`    - path to X509 certificate key.
* Updated Dockerfile

v0.2.1
======

* set expire on presence hash and set keys in Redis Engine. This prevents staling presence keys in Redis.

v0.2.0
======

* add optional `client` field to publish API requests. `client` will be added on top level of 
	published message. This means that there is now a way to include `client` connection ID to 
	publish API request to Centrifugo (to get client connection ID call `centrifuge.getClientId()` in 
	javascript). This client will be included in a message as I said above and you can compare
	current client ID in javascript with `client` ID in message and if both values equal then in 
	some situations you will wish to drop this message as it was published by this client and 
	probably already processed (via optimistic optimization or after successful AJAX call to web 
	application backend initiated this message).
* client limited channels - like user limited channels but for client. Only client with ID used in
	channel name can subscribe on such channel. Default client channel boundary is `&`. If you have
	channels with `&` in its name - then you must adapt your channel names to not use `&` or run Centrifugo with another client channel boundary using `client_channel_boundary` configuration
	file option.
* fix for presence and history client calls - check subscription on channel before returning result 
	or return Permission Denied error if not subscribed.
* handle interrupts - unsubscribe clients from all channels. Many thanks again to Mr Klaus Post.	
* code refactoring, detach libcentrifugo real-time core from Centrifugo service.

v0.1.1
======

Lots of internal refactoring, no API changes. Thanks to Mr Klaus Post (@klauspost) and Mr Dmitry Chestnykh (@dchest)

v0.1.0
======

First release. New [documentation](http://fzambia.gitbooks.io/centrifugal/content/).
