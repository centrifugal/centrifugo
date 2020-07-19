No backwards incompatible changes here.

Improvements:

* New section in docs – Centrifugo dev blog, check out [first post about scaling WebSocket](https://centrifugal.github.io/centrifugo/blog/scaling_websocket/)
* Possibility to [scale with Nats server](https://centrifugal.github.io/centrifugo/server/engines/#nats-broker) for **unreliable at most once PUB/SUB**
* Subscribe HTTP proxy, [see docs](https://centrifugal.github.io/centrifugo/server/proxy/#subscribe-proxy)
* Publish HTTP proxy, [see docs](https://centrifugal.github.io/centrifugo/server/proxy/#publish-proxy)
* Support for setting Redis Sentinel password, [see in docs](https://centrifugal.github.io/centrifugo/server/engines/#redis-sentinel-for-high-availability)
* Various documentation improvements since previous release

This release based on massively updated [Centrifuge](https://github.com/centrifugal/centrifuge) library, we don't expect problems but since many things were refactored – we suggest to carefully test your app.
