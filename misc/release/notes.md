Centrifugo is an open-source scalable real-time messaging server. Centrifugo can instantly deliver messages to application online users connected over supported transports (WebSocket, HTTP-streaming, Server-Sent Events (SSE/EventSource), GRPC, WebTransport). Centrifugo has the concept of a channel – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Several official client SDKs for browser and mobile development wrap the bidirectional protocol. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev).

## What's changed

### Improvements

* This release is the first built with Go 1.26. This version of the Go language includes a new garbage collector called the [Green Tea garbage collector](https://go.dev/doc/go1.26#new-garbage-collector). This may affect the performance of your Centrifugo installation; in most cases, we expect the impact to be positive. If you notice any performance changes in Centrifugo after upgrading to this release, please let us know in the community rooms. More information about the new GC can be found [here](https://go.dev/blog/greenteagc).
* Updated the Alpine image to 3.22 in the Dockerfile.
* Improve lint layout to improve local DX

### Fixes

* Updated the Go version and dependencies to inherit the latest updates and security fixes.

### Miscellaneous

* This release is built with Go 1.26.1
* See also the corresponding [Centrifugo PRO release](https://github.com/centrifugal/centrifugo-pro/releases/tag/v6.6.6).
