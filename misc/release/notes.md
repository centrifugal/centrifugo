Centrifugo is a scalable soft **real-time messaging** server written in Go language. Centrifugo can instantly deliver messages to application online users connected over a **variety of real-time transports** (like WebSocket, HTTP-streaming, SSE/EventSource, GRPC, SockJS). Centrifugo has channel concept – so it's actually a **user-facing PUB/SUB server**. Centrifugo is **language-agnostic** and can be used to implement chat applications, live comments, multiplayer games, streaming metrics, etc. in conjunction with any backend. It's especially useful when the application backend does not have built-in concurrency support, or dealing with many persistent connections is a challenge you are not going to take part in. Several official client SDKs for **browser and mobile development** wrap bidirectional protocol, also Centrifugo supports unidirectional approach for simple zero-sdk-dependency use cases.

Proceed to [centrifugal.dev](https://centrifugal.dev/) for a more detailed description.

New v4 release puts Centrifugo to the next level in terms of client protocol performance, WebSocket fallback simplicity, SDK ecosystem and channel security model. This is a major release with breaking changes according to our [Centrifugo v4 roadmap](https://github.com/centrifugal/centrifugo/issues/500).

Several important documents we have at this point can help you get started with Centrifugo v4:

* Centrifugo v4 [release blog post](https://centrifugal.dev/blog/2022/07/10/centrifugo-v4-released)
* Centrifugo v3 -> v4 [migration guide](https://centrifugal.dev/docs/getting-started/migration_v4)
* Client SDK API [specification](https://centrifugal.dev/docs/transports/client_api)
* Updated [quickstart tutorial](https://centrifugal.dev/docs/getting-started/quickstart)

### Highlights

* New client protocol iteration and unified client SDK API
* All SDKs now support all the core features of Centrifugo
* Our own WebSocket bidirectional emulation layer based on HTTP-streaming and SSE (EventSource). Without sticky session requirement for a distributed case.
* SockJS is still supported but DEPRECATED
* Redesigned, more efficient PING-PONG
* Optimistic subscriptions support (implemented in `centrifuge-js` only at this point)
* Secure by default channel namespaces
* Private channel and subscription JWT concepts revised
* Avoid sending JSON in WebSocket Close frame reason
* Temporary flag for errors, allows resilient behavior of Subscriptions
* Possibility to enable recovery and positioning from the client-side
* Experimental HTTP/3 support
* `gensubtoken` and `checksubtoken` cli commands
* Legacy options removed, some options renamed, see [migration guide](https://centrifugal.dev/docs/getting-started/migration_v4) for details.
* `meta` attached to a connection now updated upon connection refresh
* `centrifuge-js` migrated to Typescript
* The docs on [centrifugal.dev](https://centrifugal.dev/) were updated for v4, docs for v3 are still there but under version switch widget.

### Misc

* This release is built with Go 1.18.3
