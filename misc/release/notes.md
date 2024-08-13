Centrifugo is an open-source scalable real-time messaging server. Centrifugo can instantly deliver messages to application online users connected over supported transports (WebSocket, HTTP-streaming, SSE/EventSource, GRPC, WebTransport). Centrifugo has the concept of a channel – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Several official client SDKs for browser and mobile development wrap the bidirectional protocol. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev).

## What's changed

### Improvements

* Added an option to use Redis 7.4 hash field TTL for online presence [#872](https://github.com/centrifugal/centrifugo/pull/872). Redis 7.4 introduced the [per HASH field TTL](https://redis.io/blog/announcing-redis-community-edition-and-redis-stack-74/#:~:text=You%20can%20now%20set%20an%20expiration%20for%20hash%20fields.), which we now use for presence information by adding a new boolean option `redis_presence_hash_field_ttl`. One potential benefit is reduced memory usage in Redis for presence information, as fewer data needs to be stored (no separate ZSET structure to maintain). Depending on your presence data, you can achieve up to a 1.6x reduction in memory usage with this option. This also showed slightly better CPU utilization on the Redis side, as there are fewer keys to manage in LUA scripts during presence get, add, and remove operations. By default, Centrifugo will continue using the current presence implementation with ZSET, as the new option only works with Redis >= 7.4 and requires explicit configuration. See also the description of the new option in the [Centrifugo documentation](https://centrifugal.dev/docs/server/engines#redis_presence_hash_field_ttl).

### Fixes

* Process connect proxy result subs [#874](https://github.com/centrifugal/centrifugo/pull/874), fixes [#873](https://github.com/centrifugal/centrifugo/issues/873). Also, the documentation contained wrong description of subscribe options override object, this was fixed in terms of [centrifugal/centrifugal.dev#52](https://github.com/centrifugal/centrifugal.dev/pull/52).

### Miscellaneous

* Release is built with Go 1.22.6
* Check out [Centrifugo v6 roadmap](https://github.com/centrifugal/centrifugo/issues/832) issue. It outlines some important changes planned for the next major release. The date of the v6 release is not yet specified.
* See also the corresponding [Centrifugo PRO release](https://github.com/centrifugal/centrifugo-pro/releases/tag/v5.4.6).
