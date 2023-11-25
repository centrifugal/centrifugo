Centrifugo is an open-source scalable real-time messaging server. Centrifugo can instantly deliver messages to application online users connected over supported transports (WebSocket, HTTP-streaming, SSE/EventSource, GRPC, SockJS, WebTransport). Centrifugo has the concept of a channel – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Several official client SDKs for browser and mobile development wrap the bidirectional protocol. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev).

## What's changed

### Improvements

* Avoid keeping zero offsets in history meta hash keys in Redis – slightly reduces memory consumption of Redis, see [centrifugal/centrifuge#332](https://github.com/centrifugal/centrifuge/pull/332)

### Fixes

* Centrifugo v5.1.1 fixed `Lua redis lib command arguments must be strings or integers script` error for new Centrifugo setups and new keys in Redis, but have not provided solution to existing keys. In [centrifugal/centrifuge/#331](https://github.com/centrifugal/centrifuge/pull/331) we fixed it.
* Updating `github.com/redis/rueidis` to v1.0.22 fixes unaligned atomics to run Centrifugo with Redis engine on 32-bit systems, [some details](https://github.com/centrifugal/centrifugo/pull/737) 

### Misc

* Release is built using Go v1.21.4
* Opentelemetry dependencies updated
* We now have a [bash script for quick local setup of Redis cluster](https://github.com/centrifugal/centrifugo/tree/master/misc/redis_cluster) - to simplify development
