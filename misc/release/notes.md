This release is a step towards new interesting possibilities with Centrifugo. It adds server-side subscriptions support and some sugar on top of it. With server-side subscriptions you don't need to call `Subscribe` method on client side at all. Follow release notes to know more.

No backwards incompatible changes here.

Improvements:

* Server-side subscriptions, this functionality requires updating client code so at moment usage is limited to `centrifuge-js`. Also there is a possibility to automatically subscribe user connection to personal notifications channel. More info in [new documentation chapter](https://centrifugal.github.io/centrifugo/server/server_subs/)
* New private subscription JWT `eto` claim - see [its description in docs](https://centrifugal.github.io/centrifugo/server/private_channels/#eto)
* Options to disable WebSocket, SockJS and API handlers – [see docs](https://centrifugal.github.io/centrifugo/server/configuration/#disable-default-endpoints)
* New option `websocket_use_write_buffer_pool` – [see docs](https://centrifugal.github.io/centrifugo/transports/websocket/)
* Metrics now include histograms of requests durations - [pull request](https://github.com/centrifugal/centrifugo/pull/337)
* Add Linux ARM binary release

Fixes:

* Fix unreliable unsubscriptions from Redis PUB/SUB channels under load, now we unsubscribe nodes from PUB/SUB channels over in-memory queue
* Fix `tls_external` option regression
