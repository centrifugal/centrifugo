Centrifugo is an open-source scalable real-time messaging server. Centrifugo can instantly deliver messages to application online users connected over supported transports (WebSocket, HTTP-streaming, SSE/EventSource, GRPC, WebTransport). Centrifugo has the concept of a channel – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Several official client SDKs for browser and mobile development wrap the bidirectional protocol. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev).

## What's changed

Centrifugo v5.4.2 comes with useful improvements and important fix.

### Improvements

* [Raw mode for Nats broker](https://centrifugal.dev/docs/server/engines#nats-raw-mode) – in this mode Centrifugo just consumes core Nats topics and does not expect any Centrifugo internal wrapping.
* Option to [use wildcard subscriptions](https://centrifugal.dev/docs/server/engines#nats_allow_wildcards) with Nats broker. allows subscribing to [wildcard Nats subjects](https://docs.nats.io/nats-concepts/subjects#wildcards) (containing `*` and `>` symbols). This way client can receive messages from many channels while only having a single subscription.
* Support configuring [client TLS in GRPC proxy](https://centrifugal.dev/docs/server/proxy#proxy_grpc_tls), here we started migration to [unified TLS config object](https://centrifugal.dev/docs/server/tls#unified-tls-config-object) – using it here. See more details about revisiting TLS configuration in [this issue](https://github.com/centrifugal/centrifugo/issues/831). TLS object is also supported for [granular proxy configuration](https://centrifugal.dev/docs/server/proxy#defining-a-list-of-proxies).
* Support configuring [client TLS in Nats broker](https://centrifugal.dev/docs/server/engines#nats_tls) (for Nats client). Also uses unified TLS config object.
* [RPC ping extension](https://centrifugal.dev/docs/server/configuration#enable-rpc-ping-extension) to check if connection is alive at any point, measure RTT time.
* New histogram metric [centrifugo_client_ping_pong_duration_seconds](https://centrifugal.dev/docs/server/observability#centrifugo_client_ping_pong_duration_seconds) to track the duration of ping/pong – i.e. time between sending ping to client and receiving pong from client.

### Fixes

* Fix occasional deadlock leading to memory leak, the deadlock was introduced in Centrifugo v5.3.2, see [#856](https://github.com/centrifugal/centrifugo/issues/856)
* Fix non-working `allow_presence_for_subscriber` option to enable join/leave events when requested by client, see [#849](https://github.com/centrifugal/centrifugo/issues/849)

### Miscellaneous

* Release is built with Go 1.22.5
* All dependencies were updated to latest versions
* Check out [Centrifugo v6 roadmap](https://github.com/centrifugal/centrifugo/issues/856) issue. It outlines some important changes planned for the next major release. The date of the v6 release is not yet specified. 
