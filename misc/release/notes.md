This is a new major version of Centrifugo. New version has some important changes and useful features.

Centrifugo v2 serves the same purpose as Centrifugo v1 but is not backwards compatible – migration to it will require adapting both backend and frontend sides of your application (of course if you decide to migrate).

Centrifugo is now based on new library [centrifuge](https://github.com/centrifugal/centrifuge) for Go language. That library can be used standalone to get even more than Centrifugo server provides – like custom auth, permissions, asynchronous message passing and RPC calls.

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
* Websocket prepared message support (though this one can be pushed back at some point).

[New documentation](https://centrifugal.github.io/centrifugo/) contains actual information and tips about migration from v1.
