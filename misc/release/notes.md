This is a new major version of Centrifugo. New version has some important changes and useful features.

Centrifugo v2 serves the same purpose as Centrifugo v1. Centrifugo v2 is not backwards compatible with v1 – migration to it will require adapting both backend and frontend sides of your application (of course if you decide to migrate).

Centrifugo is now based on new library [centrifuge](https://github.com/centrifugal/centrifuge) for Go language. That library can be used standalone to get even more than Centrifugo server provides – like custom authentication, your own permission management, asynchronous message passing, RPC calls etc.

Highlights of v2:

* Cleaner and more structured client-server protocol defined in protobuf schema
* Binary Websocket support (Protobuf). Of course JSON is still the main serialization format
* JWT for authentication and private channel authorization instead of hand-crafted HMAC sign
* Prometheus integration and automatic export of stats to Graphite
* Refactored [Javascript](https://github.com/centrifugal/centrifuge-js) (ES6), [Go](https://github.com/centrifugal/centrifuge-go) and [gomobile client](https://github.com/centrifugal/centrifuge-mobile) libraries
* Simplified HTTP API authentication (no request body signing anymore)
* GRPC for server API
* New `presence_stats` API command
* Structured logging
* Mechanism to automatically merge several Websocket messages into one
* Better recovery algorithm to fix several `recovered` flag false positives
* Goreleaser for automatic releases to Github

Some things were removed from Centrifugo in v2 release:

* Publishing over Redis queue
* Admin websocket endpoint
* Client limited channels
* `history_drop_inactive` channel option now gone
* Websocket prepared message support (though this one can be pushed back at some point).

[New documentation](https://centrifugal.github.io/centrifugo/) contains actual information and tips about migration from v1.

As said above new version uses JWT tokens for authentication and private channel authorization. And there is no API request body signing anymore. This all means that there is no real need using API clients (like `cent`, `phpcent`, `jscent`, `rubycent`, `gocent` before) – you can use any JWT library for your language and just send commands from your code – this is just simple JSON objects. Though those libraries still make sense to simplify integration a bit.

At moment there are no native mobile clients. I.e. `centrifuge-ios` and `centrifuge-android` have not been updated to Centrifugo v2 yet.
