Centrifugo now uses `https://cdn.jsdelivr.net/npm/sockjs-client@1/dist/sockjs.min.js` as default SockJS url. This allows Centrifugo to always be in sync with recent v1 SockJS client. This is important to note because SockJS requires client versions to exactly match on server and client sides when using transports involving iframe. Don't forget that Centrifugo has `sockjs_url` option to set custom SockJS URL to use on server side.

Improvements:

* support setting all configuration options over environment variables in format `CENTRIFUGO_<OPTION_NAME>`. This was available before but starting from this release we will support setting **all** options over env
* show HTTP status code in logs when debug log level on
* option to customize HTTP handler endpoints, see [docs](https://centrifugal.github.io/centrifugo/server/configuration/#customize-handler-endpoints)

Fixes:

* Fix setting `presence_disable_for_client` and `history_disable_for_client` on config top level
* Fix Let's Encrypt integration by updating to ACMEv2 / RFC 8555 compilant acme library, see [#311](https://github.com/centrifugal/centrifugo/issues/311)
