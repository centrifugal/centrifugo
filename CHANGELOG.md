v1.0.0
======

I have a bad and a good news for you. Let's start with good one. Centrifugo is still real-time messaging server and just got v1 release. The bad – I've broken backwards compatibility with previous versions. Actually there is just one change that ruins compatibility – starting
from this release Centrifugo won't support multiple registered projects. It will work with only
one application. You don't need to use `project key` anymore. Changes resulted in simplified
configuration file format. The only required option is `secret` now. See updated documentation 
to see how to set `secret`.

As this is v1 release I'll try to be more careful about backwards compatibility with libraries in 
future. But all backwards incompatibilities here means that all you need to to is update related Centrifugal libraries you are using and slightly change configuration file.

Changes in this release are:

* works with single project only. No more `project key`. `secret` the only required configuration option.
* when `secret` set via environment variable `CENTRIFUGO_SECRET` then configuration file is not required anymore. But when Centrifugo configured via environment variables it's not possible to reload configuration sending HUP signal to process.
* new `stats` command to export various stats and metrics over API call.
* new `insecure_api` option to turn on insecure HTTP API mode. Read more [in docs chapter](https://fzambia.gitbooks.io/centrifugal/content/mixed/insecure_mode.html).
* minor clean-ups in client protocol. As protocol incapsulated in javascript client library you only need to update it.

I've updated and released all API and client libraries I support. centrifuge-js, Cent, Adjacent. API clients for Ruby and PHP will be updated in near future.

Admin web interface was also updated to support changes introduced here.

There are 2 new API libraries: [gocent](https://github.com/centrifugal/gocent) and [jscent](https://github.com/centrifugal/jscent). First for Go language. And second for NodeJS.

Also if you are interested take a look at [centrifuge-go](https://github.com/centrifugal/centrifuge-go) experimental Centrifugo client for Go language.

How to migrate
--------------

Use new versions of Centrifugal libraries. Project key not needed in client connection parameters, in client token generation, in HTTP API client initialization.


v0.3.0
======

* new `channels` API command – allows to get list of active channnels in project at moment (with one or more subscribers).
* `message_send_timeout` option default value is now 0 (last default value was 60 seconds) i.e. send timeout is not used by default. This means that Centrifugo won't start lots of goroutines and timers for every message sent to client. This helps to drastically reduce memory allocations. But in this case it's recommended to keep Centrifugo behind properly configured reverse proxy like Nginx to deal with connection edge cases - slow reads, slow writes etc.
* Centrifugo now sends pings into pure Websocket connections. Default value is 25 seconds and can be adjusted using `ping_interval` configuration option. Note that this option also sets SockJS heartbeat messages interval. This opens a road to set reasonable value for Nginx `proxy_read_timeout` for `/connection` location to mimic behaviour of `message_send_timeout` which is now not used by default
* improvements in Redis Engine locking.
* tests now require Redis instance running to test Redis engine. Tests use Redis database 9 to run commands and if that database is not empty then tests will fail to prevent corrupting existing data.
* all dependencies now vendored.

v0.2.4
======

* HTTP API endpoint now can handle json requests. Used in client written in Go at moment. Old behaviour have not changed, so this is absolutely optional.
* dependency packages updated to latest versions - websocket, securecookie, sockjs-go.

v0.2.3
======

Critical bug fix for Redis Engine!

* fixed bug when entire server could unsubscribe from Redis channel when client closed its connection.


v0.2.2
======

* Add TLS support. New flags are:
  * `--ssl`                   - accept SSL connections. This requires an X509 certificate and a key file.
  * `--ssl_cert="file.cert"`  - path to X509 certificate file.
  * `--ssl_key="file.key"`    - path to X509 certificate key.
* Updated Dockerfile

v0.2.1
======

* set expire on presence hash and set keys in Redis Engine. This prevents staling presence keys in Redis.

v0.2.0
======

* add optional `client` field to publish API requests. `client` will be added on top level of 
	published message. This means that there is now a way to include `client` connection ID to 
	publish API request to Centrifugo (to get client connection ID call `centrifuge.getClientId()` in 
	javascript). This client will be included in a message as I said above and you can compare
	current client ID in javascript with `client` ID in message and if both values equal then in 
	some situations you will wish to drop this message as it was published by this client and 
	probably already processed (via optimistic optimization or after successful AJAX call to web 
	application backend initiated this message).
* client limited channels - like user limited channels but for client. Only client with ID used in
	channel name can subscribe on such channel. Default client channel boundary is `&`. If you have
	channels with `&` in its name - then you must adapt your channel names to not use `&` or run Centrifugo with another client channel boundary using `client_channel_boundary` configuration
	file option.
* fix for presence and history client calls - check subscription on channel before returning result 
	or return Permission Denied error if not subscribed.
* handle interrupts - unsubscribe clients from all channels. Many thanks again to Mr Klaus Post.	
* code refactoring, detach libcentrifugo real-time core from Centrifugo service.

v0.1.1
======

Lots of internal refactoring, no API changes. Thanks to Mr Klaus Post (@klauspost) and Mr Dmitry Chestnykh (@dchest)

v0.1.0
======

First release. New [documentation](http://fzambia.gitbooks.io/centrifugal/content/).