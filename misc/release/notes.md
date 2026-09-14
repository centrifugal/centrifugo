Centrifugo is an open-source scalable real-time messaging server. It instantly delivers messages to application online users connected over supported transports (WebSocket, HTTP-streaming, Server-Sent Events (SSE), GRPC, WebTransport). Centrifugo is built around channel subscriptions – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, AI streaming responses, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Official client SDKs are available for JavaScript (browser, Node.js, React Native), Dart/Flutter, Swift, Java, Python, Go, and .NET. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev). For runnable demos see [centrifugal/examples](https://github.com/centrifugal/examples).

## What's changed

### Fixes

* A client-side subscription refresh with an already expired subscription token was accepted. The subscription's expiration time was then cleared, so the subscription never expired. Now Centrifugo closes the connection with the `3006` (`subscription expired`) disconnect code. The client reconnects, gets a token expired error on resubscribe and requests a new token – all official SDKs already handle this ([centrifugal/centrifuge#627](https://github.com/centrifugal/centrifuge/pull/627)).
* Fossil delta compression with recovery: when a subscribe with recovery found no missed publications, the next publication was still sent as a delta. The client had no base data for it, so it could not decode that publication or any after it. Now the first publication after such a subscribe is sent with full data ([centrifugal/centrifuge#629](https://github.com/centrifugal/centrifuge/pull/629)).
* Align fossil deltas to UTF-8 character boundaries for JSON clients [centrifugal/centrifuge#630](https://github.com/centrifugal/centrifuge/pull/630). For a change inside a multi-byte character it copies the first bytes of the character from the previous data and inserts the rest. The inserted bytes aren't valid UTF-8, json.Escape replaces them with U+FFFD, and the client fails to apply the delta. The fix eliminates this.

### Miscellaneous

* This release is built with Go 1.26.8.
* Dependency updates.
* See also the corresponding [Centrifugo PRO release](https://github.com/centrifugal/centrifugo-pro/releases/tag/v6.9.6).
