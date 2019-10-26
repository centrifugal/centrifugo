No backwards incompatible changes here.

Improvements:

* New channel and channel namespace options: `presence_disable_for_client` and `history_disable_for_client`. `presence_disable_for_client` allows to make presence available only for server side API. `history_disable_for_client` allows to make history available only for server side API. Previously when enabled presence and history were available for both client and server APIs. Now you can disable for client side. History recovery mechanism if enabled will continue to work for clients anyway even if `history_disable_for_client` is on
* Wait for close handshake completion before terminating WebSocket connection from server side. This allows to gracefully shutdown WebSocket sessions

Fixes:

* Fix crash due to race condition, race reproduced when history recover option was on. See [commit](https://github.com/centrifugal/centrifuge/pull/73/files) with with
* Fix lack of `client_anonymous` option. See [#304](https://github.com/centrifugal/centrifugo/issues/304)

This release based on Go 1.13.x
