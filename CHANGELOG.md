v1.2.0 (not released yet)
=========================

No backwards incompatible changes here.

* New `recover` option to automatically recover missed messages based on last message ID. See [pull request](https://github.com/centrifugal/centrifugo/pull/42) and [chapter in docs](https://fzambia.gitbooks.io/centrifugal/content/server/recover.html) for more information. Note that you need centrifuge-js >= v1.1.0 to use new `recover` option
* New `broadcast` API method to send the same data into many channels. See [issue](https://github.com/centrifugal/centrifugo/issues/41) and updated [API description in docs](https://fzambia.gitbooks.io/centrifugal/content/server/api.html)
* Dockerfile now checks SHA256 sum when downloading release zip archive.
* release built using Go 1.5.2


v1.1.0
======

No backwards incompatible changes here.

* support enabling web interface over environment variable CENTRIFUGO_WEB
* close client's connection after its message queue exceeds 10MB (default, can be modified using `max_client_queue_size` configuration file option)
* fix theoretical server crash on start when reading from redis API queue


v1.0.0
======

A bad and a good news here. Let's start with a good one. Centrifugo is still real-time messaging server and just got v1.0 release. The bad – it is not fully backwards compatible with previous versions. Actually there are three changes that ruin compatibility. If you don't use web interface and private channels then there is only one change. But it affects all stack - Centrifugo itself, client library and API library.

Starting from this release Centrifugo won't support multiple registered projects. It will work with only one application. You don't need to use `project key` anymore. Changes resulted in simplified
configuration file format. The only required option is `secret` now. See updated documentation 
to see how to set `secret`. Also changes opened a way for exporting Centrifugo node statistics via HTTP API `stats` command.

As this is v1 release we'll try to be more careful about backwards compatibility in future. But as we are trying to make a good software required changes will be done if needed.

Highlights of this release are:

* Centrifugo now works with single project only. No more `project key`. `secret` the only required configuration option.
* web interface is now embedded, this means that when downloading release you get possibility to run Centrifugo web interface just providing `--web` flag to `centrifugo` when starting process.
* when `secret` set via environment variable `CENTRIFUGO_SECRET` then configuration file is not required anymore. But note, that when Centrifugo configured via environment variables it's not possible to reload configuration sending HUP signal to process.
* new `stats` command added to export various node stats and metrics via HTTP API call. Look its response example [in docs chapter](https://fzambia.gitbooks.io/centrifugal/content/server/api.html).
* new `insecure_api` option to turn on insecure HTTP API mode. Read more [in docs chapter](https://fzambia.gitbooks.io/centrifugal/content/mixed/insecure_mode.html).
* minor clean-ups in client protocol. But as protocol incapsulated in javascript client library you only need to update centrifuge-js.
* release built using Go 1.5.1

[Documentation](https://fzambia.gitbooks.io/centrifugal/content/) was updated to fit all these release notes. Also all API and client libraries were updated – Javascript browser client (`centrifuge-js`), Python API client (`cent`), Django helper module (`adjacent`). API clients for Ruby (`centrifuge-ruby`) and PHP (`phpcent`) too. Admin web interface was also updated to support changes introduced here.

There are 2 new API libraries: [gocent](https://github.com/centrifugal/gocent) and [jscent](https://github.com/centrifugal/jscent). First for Go language. And second for NodeJS.

Also if you are interested take a look at [centrifuge-go](https://github.com/centrifugal/centrifuge-go) – experimental Centrifugo client for Go language. It allows to connect to Centrifugo from non-browser environment. Also it can be used as a reference to make a client in another language (still hoping that clients in Java/Objective-C/Swift to use from Android/IOS applications appear one day – but we need community help here).

How to migrate
--------------

* Use new versions of Centrifugal libraries - browser client and API client. Project key not needed in client connection parameters, in client token generation, in HTTP API client initialization.
* Another backwards incompatible change related to private channel subscriptions. Actually this is not related to Centrifugo but to Javascript client but it's better to write about it here. Centrifuge-js now sends JSON (`application/json`) request instead of `application/x-www-form-urlencoded` when client wants to subscribe on private channel. See [in docs](https://fzambia.gitbooks.io/centrifugal/content/mixed/private_channels.html) how to deal with JSON in this case.
* `--web` is now a boolean flag. Previously it was used to set path to admin web interface. Now it indicates whether or not Centrifugo must serve web interface. To provide path to custom web application directory use `--web_path` string option.

I.e. before v1 you started Centrifugo like this to use web interface:

```
centrifugo --config=config.json --web=/path/to/web/app
```

Now all you need to do is run:

```
centrifugo --config=config.json --web
```

And no need to download web interface repository at all! Just run command above and check http://localhost:8000.

If you don't want to use embedded web interface you can still specify path to your own web interface directory:

```
centrifugo --config=config.json --web --web_path=/path/to/web/app
```


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