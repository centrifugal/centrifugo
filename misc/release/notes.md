This is a new major version of Centrifugo. New version has some important changes and useful features.

Centrifugo v2 serves the same purpose as Centrifugo v1. Centrifugo v2 is not backwards compatible with v1 – migration to it will require adapting both backend and frontend sides of your application (of course if you decide to migrate).

Centrifugo is now based on new library [centrifuge](https://github.com/centrifugal/centrifuge) for Go language. That library can be used standalone to get even more than Centrifugo server provides – like custom authentication, your own permission management, asynchronous message passing, RPC calls etc.

Highlights of v2:

* Cleaner and more structured client-server protocol defined in protobuf schema. Protocol is more compact because some fields with default values that were sent previously now omitted
* Binary Websocket support (Protobuf). Protobuf allows to transfer data in much more compact and performant way than before. Of course JSON is still the main serialization format
* JWT for authentication and private channel authorization instead of hand-crafted HMAC sign. This means that there is no need in extra libraries to generate connection and subscription tokens. There are [plenty of JWT libraries](https://jwt.io/) for all languages
* Prometheus integration and automatic export of stats to Graphite. Now Centrifugo easily integrates in modern monitoring stack – no need to manually export stats
* Refactored [Javascript](https://github.com/centrifugal/centrifuge-js) (ES6), [Go](https://github.com/centrifugal/centrifuge-go) and [gomobile client](https://github.com/centrifugal/centrifuge-mobile) libraries
* Simplified HTTP API authentication (no request body signing anymore)
* GRPC for server API
* New `presence_stats` API command to get compact presence information - how many clients and unique users in channel
* Structured logging with coloured output during development
* Mechanism to automatically merge several Websocket messages into one to reduce syscall amount thus be more performant under heavy load
* Better recovery algorithm to fix several `recovered` flag false positives
* Goreleaser for automatic releases to Github

Some things were removed from Centrifugo in v2 release:

* Publishing over Redis queue
* Admin websocket endpoint
* Client limited channels
* `history_drop_inactive` channel option now gone
* Websocket prepared message support (though this one can be pushed back at some point).

[New documentation](https://centrifugal.github.io/centrifugo/) contains actual information and tips about migration from v1.

As mentioned above new version uses JWT tokens for authentication and private channel authorization. And there is no API request body signing anymore. This all means that using API clients (like `cent`, `phpcent`, `jscent`, `rubycent`, `gocent` before) is not necessary anymore – you can use any JWT library for your language and just send commands from your code – this is just simple JSON objects. Though API libraries still make sense to simplify integration a bit.

At moment there are no native mobile clients. I.e. `centrifuge-ios` and `centrifuge-android` have not been updated to Centrifugo v2 yet.
