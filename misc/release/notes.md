Centrifugo is an open-source scalable real-time messaging server. Centrifugo can instantly deliver messages to application online users connected over supported transports (WebSocket, HTTP-streaming, SSE/EventSource, GRPC, SockJS, WebTransport). Centrifugo has the concept of a channel – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Several official client SDKs for browser and mobile development wrap the bidirectional protocol. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev).

## Release notes

This release contains an important fix of v4 degradation (proxying user limited channel) and comes with several nice improvements.

### Fixes

* Avoid proxying user limited channel [#550](https://github.com/centrifugal/centrifugo/pull/550)
* Look at subscription source to handle token subs change [#545](https://github.com/centrifugal/centrifugo/pull/545)

### Improvements

* Configure server-to-client ping/pong intervals [#551](https://github.com/centrifugal/centrifugo/pull/551)
* Option `client_connection_limit` to set client connection limit for a single Centrifugo node [#546](https://github.com/centrifugal/centrifugo/pull/546)
* Option `api_external` to expose API handler on external port [#536](https://github.com/centrifugal/centrifugo/issues/536)
* Use `go.uber.org/automaxprocs` to set GOMAXPROCS [#528](https://github.com/centrifugal/centrifugo/pull/528), this may help to automatically improve Centrifugo performance when it's running in an environment with cgroup-restricted CPU resources (Docker, Kubernetes).
* Nats broker: use push format from client protocol v2 [#542](https://github.com/centrifugal/centrifugo/pull/542)

### Misc

* While working on [Centrifuge](https://github.com/centrifugal/centrifuge) lib [@j178](https://github.com/j178) found a scenario where connection to Redis could leak, this was not observed and reported in Centrifugo outside the test suite, but it seems that theoretically connections to Redis from Centrifugo could leak with time if the network between Centrifugo and Redis is unstable. This release contains an updated Redis engine which eliminates this.
* This release is built with Go 1.18.5
