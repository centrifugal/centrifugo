Centrifugo is an open-source scalable real-time messaging server in a language-agnostic way. It can be a missing piece in your application infrastructure for introducing real-time features. Think chats, live comments, multiplayer games, streaming metrics – you'll be able to build amazing web and mobile real-time apps with a help of Centrifugo. Choose the approach you like:

* bidirectional communication over WebSocket or SockJS
* or unidirectional communication over WebSocket, EventSource (Server-Sent Events), HTTP-streaming, GRPC
* or... combine both!

See [centrifugal.dev](https://centrifugal.dev/) for more information.

## Release notes

No backwards incompatible changes here.

### Fixes

* Fix top-level granular subscribe and publish proxies [#517](https://github.com/centrifugal/centrifugo/issues/517).

### Misc

* This release is built with Go 1.17.11.
