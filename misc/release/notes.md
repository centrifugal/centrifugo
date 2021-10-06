No backwards incompatible changes here.

Fixes:

* Fix SockJS data escaping on EventSource fallback. See [igm/sockjs-go#100](https://github.com/igm/sockjs-go/issues/100) for more information. In short – this bug could prevent a message with `%` symbol inside be properly parsed by a SockJS Javascript client – thus not processed by a frontend at all.
* Fix panic on concurrent subscribe to the same channels with recovery feature on. More details in [centrifugal/centrifuge#207](https://github.com/centrifugal/centrifuge/pull/207)
