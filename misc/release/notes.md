This release has several fixes and performance improvements

Improvements:

* Use latest SockJS url (SockJS version 1.3) for iframe transports
* Improve performance of massive subscriptions to different channels
* Allow dot in namespace names

Fixes:

* Fix of possible deadlock in Redis Engine when subscribe operation fails
* Fix admin web interface [logout issue](https://github.com/centrifugal/web/issues/14) when session expired
* Fix io timeout error when using Redis Engine with sharding enabled
* Fix `checkconfig` command
* Fix typo in metric name - see [#233](https://github.com/centrifugal/centrifugo/pull/233)
