Centrifugo is an open-source scalable real-time messaging server. Centrifugo can instantly deliver messages to application online users connected over supported transports (WebSocket, HTTP-streaming, SSE/EventSource, GRPC, WebTransport). Centrifugo has the concept of a channel – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Several official client SDKs for browser and mobile development wrap the bidirectional protocol. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev).

## What's changed

### Improvements

* Minor performance optimization – reuse connection timer, see [centrifugal/centrifuge#349](https://github.com/centrifugal/centrifuge/pull/349) 

### Fixes

* Fix WebSocket compression (`io: read/write on closed pipe` error) in JSON protocol case, see [commit in centrifugal/protocol](https://github.com/centrifugal/protocol/commit/a9e11df2c5fccf8c3f0397fea0321ac09555265c)
* Fix unmarshaling slice of objects in yaml config [#786](https://github.com/centrifugal/centrifugo/pull/786)

### Misc

* Release is built with Go 1.22.2
* All dependencies were updated to latest versions
* If you use `centrifuge-js` and have dynamic subscribe/unsubscribe calls – take a look at `centrifuge-js` [v5.1.0](https://github.com/centrifugal/centrifuge-js/releases/tag/5.1.0) – it contains changes for a proper processing of unsubscribe frames.
