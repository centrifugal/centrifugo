No backwards incompatible changes here.

Fixes:

* Fix deadlock during PUB/SUB sync in channels with recovery. Addresses [this report](https://github.com/centrifugal/centrifugo/issues/486).
* Fix `redis_db` option: was ignored previously – [#487](https://github.com/centrifugal/centrifugo/issues/487).
