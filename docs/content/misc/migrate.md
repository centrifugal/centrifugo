# Migration notes from Centrifugo v1

In version 2 of Centrifugo many things changed in backwards incompatible way comparing to version 1. This document aims to help Centrifugo v1 users to migrate their projects to version 2 (if they want to).

### New client protocol and client libraries

In Centrifugo v2 internal client-server protocol changed meaning that old client library version won't work with new server. So first step in migrating - update client libraries to new version with Centrifugo v2 support.

While refactoring client's API changed a bit so you have to adapt your code to those changes.

For the moment of this writing we have no native mobile libraries for Centrifugo v2. So if you are using `centrifuge-ios` or `centrifuge-android` then you can't migrate to v2 until those libraries will be ported.

### Migrate communication with API

Centrifugo v2 simplified communication with API - requests should not be signed with secret key anymore thus you can simply integrate your backend with Centrifugo without using any of our helper libraries - just send JSON API command as POST request to api endpoint. Don't forget to use api key and protect API endpoint with TLS (more information in server API description document).

Centrifugo v1 could process messages published in Redis queue. In v2 this possibility was removed because this technique is not good in terms of error handling and non-deterministic delay before message will be processed by Centrifugo node worker. Migrate to using HTTP or GRPC API.

### Use JWT instead of hand-crafted connection token

In Centrifugo v2 you must use JWT instead of hand-crafted tokens of v1. This means that you need to download JWT library for your language (there are plenty of them – see jwt.io) and build connection token with it.

See dedicated docs chapter to see how token can be built. 

All connection information will be passed inside this single token string. This means you only need to pass one string to your frontend. No need to pass `user`, `timestamp`, `info` anymore. This also means that you will have less problems with escaping features of template engines - because JWT is safe base64 string.

Connection expiration (connection check mechanism) now based on `exp` claim of JWT – you don't need to enable it globally in configuration. 

### Use JWT instead of hand-crafted signature for private subscriptions

Read chapter about private subscriptions to find how you should now use JWT for private channel subscriptions.

### Channel options changed

Channel option `recover` now called `history_recover`.

There is no `watch` channel option anymore - in Centrifugo v2 admin websocket connection was removed as it made code base much more overhelmed for almost nothing. 

### SockJS endpoint changed

It's now `/connection/sockjs` instead of `/connection`

### New way to export metrics

Centrifugo is now uses Prometheus primitives internally so if you are using Prometheus you can simply configure it to monitor Centrifugo. Also Centrifugo is able to automatically convert and export metrics to Graphite. See special Monitoring chapter in server docs.

Previously you have to periodically call `stats` command and export metrics manually. This is gone in Centrifugo v2.

### Options renamed

Some of advanced options have been renamed – if you are using advanced configuration then refer to documentation to find actual option names.

### No client limited channels anymore

That was a pretty useless feature of Centrifugo v1.

### New reserved symbols in channel name

Symbols `*` and `/` in channel name are now reserved for Centrifugo future needs - please do not use it in channels.

### Centrifugo v1 repos

Here some links for those who still use Centrifugo v1

* [Centrifugo v1 source code](https://github.com/centrifugal/centrifugo/tree/v1)
* [Centrifugo v1 documentation](https://fzambia.gitbooks.io/centrifugal/content/)
* [centrifuge-js v1](https://github.com/centrifugal/centrifuge-js/tree/v1)
* [centrifuge-go](https://github.com/centrifugal/centrifuge-go/tree/v0.0.1)
* [centrifuge-mobile](https://github.com/centrifugal/centrifuge-mobile/tree/v0.0.1)
* [examples](https://github.com/centrifugal/examples/tree/ac45e31c7d94ce1d32b59f6e262ce1ec3f3f42f0)
