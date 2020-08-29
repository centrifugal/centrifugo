No backwards incompatible changes here.

Improvements:

* Internal refactoring of WebSocket graceful close, should make things a bit more performant (though only in apps which read lots of messages from WebSocket connections)
* Disconnect code is now `uint32` internally
* A bit more performant permission checks for publish, history and presence ops 
* Connect proxy request payload can optionally contain `name` and `version` of client if set on client side, see updated [connect proxy docs](https://centrifugal.github.io/centrifugo/server/proxy/#connect-proxy)
* New blog post [Experimenting with QUIC and WebTransport in Go](https://centrifugal.github.io/centrifugo/blog/quic_web_transport/) in Centrifugo blog

Fixes:

* fix panic on connect in 32-bit ARM builds, see [#387](https://github.com/centrifugal/centrifugo/issues/387)
