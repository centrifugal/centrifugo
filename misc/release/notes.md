Centrifugo is an open-source scalable real-time messaging server. It instantly delivers messages to application online users connected over supported transports (WebSocket, HTTP-streaming, Server-Sent Events (SSE), GRPC, WebTransport). Centrifugo is built around channel subscriptions – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, AI streaming responses, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Official client SDKs are available for JavaScript (browser, Node.js, React Native), Dart/Flutter, Swift, Java, Python, Go, and .NET. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev). For runnable demos see [centrifugal/examples](https://github.com/centrifugal/examples).

## What's changed

### Improvements

* Faster and less allocating decoding of client commands. The command decoders are on the hot path – one is taken from the pool for every frame a client sends. The JSON decoder now reuses its buffered reader instead of allocating a new 4KB buffer per frame, and returns commands without allocating a slice per command when they fit into the read buffer. The Protobuf decoder unmarshals small messages straight out of the buffered reader instead of copying them into a scratch buffer first. Decoding a frame with a single JSON command got 62% faster with 93% less memory allocated, a frame with 8 commands – 25% faster with 67% less memory; for Protobuf the win is around 7-10% in time ([centrifugal/protocol#41](https://github.com/centrifugal/protocol/pull/41)).
* The message size limit is now applied to every command in a JSON frame. Previously it was only checked reliably for the first command, so subsequent commands in the same frame could exceed the configured limit (up to roughly twice it). A command of exactly the limit is still accepted wherever it sits in the frame ([centrifugal/protocol#41](https://github.com/centrifugal/protocol/pull/41)).

### Fixes

* Fix `client.connection_rate_limit`: the configured value was used as the token bucket burst only, while the sustained rate was one connection per second. Centrifugo now accepts the configured number of new connections per second, keeping the configured value as the burst size too. Thanks to [@codeloperstz](https://github.com/codeloperstz) for the fix ([#1207](https://github.com/centrifugal/centrifugo/pull/1207)).
* Shared poll: `hmac_previous_secret_key_valid_until` was compared against the `iat` claim carried inside the track signature. That value is chosen by whoever mints the signature, so anyone retaining the rotated-out key could keep producing accepted signatures indefinitely by backdating `iat`. The cutoff is now enforced against server time before consulting the previous verifier – the same way it works for the previous JWT HMAC key – with the `iat` check kept as an additional constraint inside the grace period ([#1209](https://github.com/centrifugal/centrifugo/pull/1209)).
* Apply `publication_data_format` on all publication paths. It was only enforced for stream publishes (API publish, API broadcast, client publish), so map publish, shared poll publish and client map publish stored whatever bytes the caller sent. Data returned by publish, map publish and shared poll refresh proxies is now validated as well – a proxy can replace the data a client sent, and a shared poll refresh result reaches subscribers without ever passing through a client ([#1210](https://github.com/centrifugal/centrifugo/pull/1210)).

### Miscellaneous

* This release is built with Go 1.26.7.
* Dependency updates.
* See also the corresponding [Centrifugo PRO release](https://github.com/centrifugal/centrifugo-pro/releases/tag/v6.9.3).
