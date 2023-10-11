Centrifugo is an open-source scalable real-time messaging server. Centrifugo can instantly deliver messages to application online users connected over supported transports (WebSocket, HTTP-streaming, SSE/EventSource, GRPC, SockJS, WebTransport). Centrifugo has the concept of a channel – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Several official client SDKs for browser and mobile development wrap the bidirectional protocol. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev).

## What's changed

### Improvements

* Support for EC keys in JWK sets and EC JWTs when using JWKS [#720](https://github.com/centrifugal/centrifugo/pull/720) by @shaunco, [JWKS docs updated](https://centrifugal.dev/docs/server/authentication#json-web-key-support)
* Experimental GRPC proxy subscription streams [#722](https://github.com/centrifugal/centrifugo/pull/722) - this is like [Websocketd](https://github.com/joewalnes/websocketd) but on network steroids 🔥. Streaming request semantics - both unidirectional and bidirectional – is now super-simple to achieve with Centrifugo and GRPC. See additional details about motivation, design, scalability concerns and basic examples in [docs](https://centrifugal.dev/docs/server/proxy_streams)
* Transport error mode for server HTTP and GRPC APIs [#690](https://github.com/centrifugal/centrifugo/pull/690) - read [more in docs](https://centrifugal.dev/docs/server/server_api#transport-error-mode)
* Support GRPC gzip compression [#723](https://github.com/centrifugal/centrifugo/pull/723). GRPC servers Centrifugo has now recognize gzip compression, proxy requests can optionally use compression for calls (see [updated proxy docs](https://centrifugal.dev/docs/server/proxy)).

### Misc

* Release is built with Go 1.21.3
* Dependencies updated (crypto, otel, msgpack, etc)
