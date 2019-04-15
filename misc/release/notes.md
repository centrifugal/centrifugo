This release fixes two regressions introduced by v2.2.0.

Improvements:

* [New documentation chapter](https://centrifugal.github.io/centrifugo/server/grpc_api/) about GRPC API in Centrifugo.

Fixes:

* Fix client disconnect in channels with enabled history but disabled recovery
* Fix wrong Push type sent in Redis engine: Leave message was used where Join required
