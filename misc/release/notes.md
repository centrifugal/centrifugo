Centrifugo is an open-source scalable real-time messaging server. Centrifugo can instantly deliver messages to application online users connected over supported transports (WebSocket, HTTP-streaming, Server-Sent Events (SSE/EventSource), GRPC, WebTransport). Centrifugo has the concept of a channel – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Several official client SDKs for browser and mobile development wrap the bidirectional protocol. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev).

## What's changed

### Improvements

* Centrifugo now automatically detects Redis Cluster and no longer requires explicit Cluster configuration. See [#951](https://github.com/centrifugal/centrifugo/pull/951) for more background on this decision. TLDR: Modern cloud providers usually provide `redis://host:port` or `rediss://host:port` URLs to users, which may correspond to either standalone Redis or Redis Cluster under the hood. Things should now work out of the box for Centrifugo users when using this string as the Redis `address`, making Centrifugo more cloud-friendly.
* Added support for the `rediss` scheme (enabling TLS). Previously, this was possible via settings like `engine.redis.tls.enabled=true` or `tls_enabled=true` in the Redis URL. However, `rediss://` is part of Redis specification and very common. See [#951](https://github.com/centrifugal/centrifugo/pull/951).
* Centrifugo now uses Go 1.24.x (specifically 1.24.1 for this release), inheriting [performance optimizations](https://go.dev/blog/go1.24#performance-improvements) introduced in the latest Go version. If you follow our community channels, you may have seen that we tried Centrifugo's PUB/SUB throughput benchmark with Go 1.24 and observed nice improvements. The previous maximum throughput on an Apple M1 Pro (2021) was 1.62 million msgs/sec, which increased to 1.74 million msgs/sec with Go 1.24—without any code changes on our side.
* Centrifugo now adds the `Access-Control-Max-Age: 300` header for HTTP-based transport preflight responses and emulation endpoint preflight responses. This prevents browsers from sending preflight OPTIONS request before every POST request in cross-origin setups. As a result, bidirectional emulation in cross-origin environments is now more efficient, reducing latency for client-to-server emulation requests (since it requires 1 RTT instead of 2 RTTs) and reducing the number of HTTP requests. Note that you need to use the latest `centrifuge-js` (>=5.3.4) to benefit from this change.
* Added a DEB package for Ubuntu 24.04 Noble Numbat. Removed support for Ubuntu Bionic Beaver due to its EOL. Addresses [#946](https://github.com/centrifugal/centrifugo/issues/946).

### Fixes

* Downgraded the `rueidis` dependency to v1.0.53 to avoid AUTH errors when RESP2 is forced. See [redis/rueidis#788](https://github.com/redis/rueidis/issues/788).

### Miscellaneous

* Centrifugo v6 has been recently released. See details in the [Centrifugo v6 release blog post](https://centrifugal.dev/blog/2025/01/16/centrifugo-v6-released).
* This release is built with Go 1.24.1.
* See also the corresponding [Centrifugo PRO release](https://github.com/centrifugal/centrifugo-pro/releases/tag/v6.1.0).
