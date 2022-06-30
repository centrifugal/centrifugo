Centrifugo is an open-source scalable real-time messaging server in a language-agnostic way. It can be a missing piece in your application infrastructure for introducing real-time features. Think chats, live comments, multiplayer games, streaming metrics – you'll be able to build amazing web and mobile real-time apps with a help of Centrifugo. Choose the approach you like:

* bidirectional communication over WebSocket or SockJS
* or unidirectional communication over WebSocket, EventSource (Server-Sent Events), HTTP-streaming, GRPC
* or... combine both!

See [centrifugal.dev](https://centrifugal.dev/) for more information.

## Release notes

No backwards incompatible changes here.

### Improvements

* Support Debian bullseye DEB package release, drop Debian jessie, [#520](https://github.com/centrifugal/centrifugo/issues/520)

### Fixes

* Fix emitting Join message in dynamic server subscribe case (when calling subscribe server API), [centrifugal/centrifuge#231](https://github.com/centrifugal/centrifuge/issues/231).
