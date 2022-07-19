Centrifugo is an open-source scalable real-time messaging server. Centrifugo can instantly deliver messages to application online users connected over supported transports (WebSocket, HTTP-streaming, SSE/EventSource, GRPC, SockJS, WebTransport). Centrifugo has the concept of a channel – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Several official client SDKs for browser and mobile development wrap the bidirectional protocol. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev).

New v4 release puts Centrifugo to the next level in terms of client protocol performance, WebSocket fallback simplicity, SDK ecosystem and channel security model. This is a major release with breaking changes according to our [Centrifugo v4 roadmap](https://github.com/centrifugal/centrifugo/issues/500).

Several important documents we have at this point can help you get started with Centrifugo v4:

* Centrifugo v4 [release blog post](https://centrifugal.dev/blog/2022/07/10/centrifugo-v4-released)
* Centrifugo v3 -> v4 [migration guide](https://centrifugal.dev/docs/getting-started/migration_v4)
* Client SDK API [specification](https://centrifugal.dev/docs/transports/client_api)
* Updated [quickstart tutorial](https://centrifugal.dev/docs/getting-started/quickstart)

### Highlights

* New client protocol iteration and unified client SDK API. See client SDK API [specification](https://centrifugal.dev/docs/transports/client_api).
* All SDKs now support all the core features of Centrifugo - see [feature matrix](https://centrifugal.dev/docs/transports/client_sdk#sdk-feature-matrix)
* Our own WebSocket bidirectional emulation layer based on HTTP-streaming and SSE (EventSource). Without sticky session requirement for a distributed case. See details in release post and [centrifuge-js README](https://github.com/centrifugal/centrifuge-js/tree/master#bidirectional-emulation)
* SockJS is still supported but DEPRECATED
* Redesigned, more efficient PING-PONG – see details in release post
* Optimistic subscriptions support (implemented in `centrifuge-js` only at this point) – see details in release post
* Secure by default channel namespaces – see details in release post
* Private channel and subscription JWT concepts revised – see details in release post
* Possibility to enable join/leave, recovery and positioning from the client-side
* Experimental HTTP/3 support - see details in release post
* Experimental WebTransport support - see details in release post
* Avoid sending JSON in WebSocket Close frame reason
* Temporary flag for errors, allows resilient behavior of Subscriptions
* `gensubtoken` and `checksubtoken` helper cli commands as subscription JWT now behaves similar to connection JWT
* Legacy options removed, some options renamed, see [migration guide](https://centrifugal.dev/docs/getting-started/migration_v4) for details.
* `meta` attached to a connection now updated upon connection refresh
* `centrifuge-js` migrated to Typescript
* The docs on [centrifugal.dev](https://centrifugal.dev/) were updated for v4, docs for v3 are still there but under version switch widget.

### Misc

* This release is built with Go 1.18.4
