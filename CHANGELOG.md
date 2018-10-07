v2.0.1
======

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

v2.0.0
======

This is a new major version of Centrifugo. New version has some important changes and useful features.

Centrifugo v2 serves the same purpose as Centrifugo v1. Centrifugo v2 is not backwards compatible with v1 – migration to it will require adapting both backend and frontend sides of your application (of course if you decide to migrate).

Centrifugo is now based on new library [centrifuge](https://github.com/centrifugal/centrifuge) for Go language. That library can be used standalone to get even more than Centrifugo server provides – like custom authentication, your own permission management, asynchronous message passing, RPC calls etc.

Highlights of v2:

* Cleaner and more structured client-server protocol defined in protobuf schema. Protocol is more compact because some fields with default values that were sent previously now omitted
* Binary Websocket support (Protobuf). Protobuf allows to transfer data in much more compact and performant way than before. Of course JSON is still the main serialization format
* JWT for authentication and private channel authorization instead of hand-crafted HMAC sign. This means that there is no need in extra libraries to generate connection and subscription tokens. There are [plenty of JWT libraries](https://jwt.io/) for all languages
* Prometheus integration and automatic export of stats to Graphite. Now Centrifugo easily integrates in modern monitoring stack – no need to manually export stats
* Refactored [Javascript](https://github.com/centrifugal/centrifuge-js) (ES6), [Go](https://github.com/centrifugal/centrifuge-go) and [gomobile client](https://github.com/centrifugal/centrifuge-mobile) libraries
* Simplified HTTP API authentication (no request body signing anymore)
* GRPC for server API
* New `presence_stats` API command to get compact presence information - how many clients and unique users in channel
* Structured logging with coloured output during development
* Mechanism to automatically merge several Websocket messages into one to reduce syscall amount thus be more performant under heavy load
* Better recovery algorithm to fix several `recovered` flag false positives
* Goreleaser for automatic releases to Github

Some things were removed from Centrifugo in v2 release:

* Publishing over Redis queue
* Admin websocket endpoint
* Client limited channels
* `history_drop_inactive` channel option now gone
* Websocket prepared message support (though this one can be pushed back at some point).

[New documentation](https://centrifugal.github.io/centrifugo/) contains actual information and tips about migration from v1.

As mentioned above new version uses JWT tokens for authentication and private channel authorization. And there is no API request body signing anymore. This all means that using API clients (like `cent`, `phpcent`, `jscent`, `rubycent`, `gocent` before) is not necessary anymore – you can use any JWT library for your language and just send commands from your code – this is just simple JSON objects. Though API libraries still make sense to simplify integration a bit.

At moment there are no native mobile clients. I.e. `centrifuge-ios` and `centrifuge-android` have not been updated to Centrifugo v2 yet.

v1.8.0
======

No backwards incompatible changes here.

### Features

* package for Ubuntu 18.04
* add Centrifugo `version` to stats output. Also add rusage stime and utime values to metrics. See [#222](https://github.com/centrifugal/centrifugo/issues/222) for details. Thanks to @Sannis for contributions
* expose more configuration options to be set over environment variables. See [commit](https://github.com/centrifugal/centrifugo/commit/bf8655914ef94aaa4b2579d943b64fc63e7b9b08) and [related issue](https://github.com/centrifugal/centrifugo/issues/223)
* more context in debug logs regarding to client connection. See [#201](https://github.com/centrifugal/centrifugo/issues/201)
* fix deb package upgrade - see [#219](https://github.com/centrifugal/centrifugo/issues/219) for details

### Internal

* using Go 1.10.3 for builds

v1.7.9
======

No backwards incompatible changes here.

### Fixes

* fix malformed JSON when using empty `info` in connection refresh request - see [#214](https://github.com/centrifugal/centrifugo/issues/214).

### Features

* support ACME http_01 challenge using new `ssl_autocert_http` boolean option. Centrifugo will serve http_01 ACME challenge on port 80. See [#210](https://github.com/centrifugal/centrifugo/issues/210) for more details. 

### Internal

* using Go 1.10.1 for builds

v1.7.8
======

No backwards incompatible changes here.

### Fixes

* the fix of goroutine leak in 1.7.7 was incomplete - looks like in this release the problem described in [#207](https://github.com/centrifugal/centrifugo/issues/207) gone away.

v1.7.7
======

No backwards incompatible changes here.

### Fixes

* fix goroutine leak due to deadlock, see [#207](https://github.com/centrifugal/centrifugo/issues/207)

### Features

* possibility to set message `uid` via API request - see [#205](https://github.com/centrifugal/centrifugo/pull/205)

### Internal

* do not send `unsubscribe` messages to client on shutdown - it will unsubscribe automatically on disconnect on client side
* using Go 1.10 for builds

v1.7.6
======

No backwards incompatible changes here.

### Fixes

* fix setting config via environment vars - `CENTRIFUGO_` prefix did not work since 1.7.4  

v1.7.5
======

No backwards incompatible changes here.

The only change is using new version of Go for builds (Go 1.9.2). This will allow to analize performance profiles more easily without having to use binaries. See [this new wiki page](https://github.com/centrifugal/centrifugo/wiki/Investigating-performance-issues) about investigating performance issues.

v1.7.4
======

No backwards incompatible changes here.

This release is centered around internal refactoring to detach node from server - see more details in [#186](https://github.com/centrifugal/centrifugo/pull/186).

### Features

* optionally create PID file using `--pid_file` command line option.
* create connections in separate goroutines to slightly improve GC (and therefore reduce memory usage).

### Internal (for developers/contributors)

* Using Go 1.8.3 for builds

v1.7.3
======

No backwards incompatible changes here.

This release built using new version of Go - 1.8.1, previously Centrifugo used Go 1.7.5, so here we benefit from Go evolution improvements - the most notable is improvements in GC pauses which should in turn improve Centrifugo latency. It also reduces memory usage by about 15-20% when websocket compression enabled.

v1.7.2
======

No backwards incompatible changes here.

### Fixes

* fix reusing read and write buffers returned from connection hijack. This was added in previous release but due to the bug in configuration the feature did not work.

v1.7.1
======

No backwards incompatible changes here.

### Fixes

* fix mass resubscribe after several Redis disconnects in a row - more details in [#163](https://github.com/centrifugal/centrifugo/pull/163)

### Features

* update Gorilla Websocket lib - it now tries to reuse buffers returned from Go http library `hijack` method. We adapted Centrifugo default websocket buffer options to utilize this feature (`websocket_read_buffer_size` and `websocket_write_buffer_size` now `0` by default).


v1.7.0
======

This release changes two important aspects of Centrifugo. We expect that it will be fully backwards compatible with previous one in most scenarios until you were using `timestamp` message field somehow.

### What's changed

* integration with Gorilla Websocket [PreparedMessage](https://godoc.org/github.com/gorilla/websocket#PreparedMessage) for raw websocket. We expect it to drastically improve websocket compression case - reducing both memory and CPU in large fan-out scenarios. This change does not affect SockJS in any way.
* `timestamp` field removed from message. See [#147](https://github.com/centrifugal/centrifugo/issues/147) for motivation.
* Several new memory metrics - `node_memory_heap_sys`, `node_memory_heap_alloc`, `node_memory_stack_inuse`

v1.6.5
======

No backwards incompatible changes here.

### Features

* resolve `history_drop_inactive` option edge case (described in [#50](https://github.com/centrifugal/centrifugo/issues/50))
* two new options for autocert: `ssl_autocert_force_rsa` and `ssl_autocert_server_name`. See [docs](https://fzambia.gitbooks.io/centrifugal/content/deploy/certificates.html#automatic-certificates) for description 

### Fixes

* update web interface - in new version we don't show connection endpoints on main page as we can't show them reliably. Final endpoints depend on your production proxy/firewall politics (and port configuration) so we don't try to guess.


v1.6.4
======

No backwards incompatible changes here.

We **consider removing** `timestamp` field from message as it's seems useless and never used by Centrifugo users. Applications that need timestamp for some reason can include it into message JSON payload. If you have any objections please look at [issue #147](https://github.com/centrifugal/centrifugo/issues/147) and write your thoughts against removing this field.

### Features

* configurable websocket compression level - see [updated docs](https://fzambia.gitbooks.io/centrifugal/content/mixed/websocket_compression.html). Bear in mind that compression is still very CPU and memory expensive
* new metric `node_uptime_seconds` - see [updated docs](https://fzambia.gitbooks.io/centrifugal/content/server/stats.html) for stats

### Fixes

* fixes crash when using builtin TLS server - see [#145](https://github.com/centrifugal/centrifugo/issues/145)
* redirect Go std lib logging into our INFO logger

### Internal (for developers/contributors)

* Using Go 1.7.5 for builds
* As soon as Go 1.8 out we will be able to remove `x/net/http2` dependency as standard lib will contain fix for [#145](https://github.com/centrifugal/centrifugo/issues/145)


v1.6.3
======

This release fixes wrong decision made in 1.6.x related to pings. We don't rely on client to server 
pings to disconnect clients anymore, we also moved back SockJS heartbeat frames - i.e. sending them 
from server to client every 25 seconds as before (in Centrifugo < 1.6.0). Recent changes in `centrifuge-js` (version 1.4.2) allowed us to not introduce addition reconnects for SockJS polling 
transports when sending client to server automatic ping. We also updated documentation [chapter about 
pings](https://fzambia.gitbooks.io/centrifugal/content/mixed/ping.html) a bit.

### Fixes

* Random disconnects from Centrifugo when using automatic client to server pings. This is a default 
behaviour so it affects almost everyone who using Centrifugo 1.6.x, fixes https://github.com/centrifugal/centrifugo/issues/142
* Fix writing headers after headers already written in raw websocket endpoint - this remove annoying log line appearing after client can't upgrade connection to Websocket.


v1.6.2
======

### Features

* Use Redis pipelining and single connection for presence/history/channels operations. This increases performance of those operations especially on systems with many CPU cores.
* Homebrew formula to install Centrifugo on MacOS, see README for instructions.
* Update gorilla websocket library - there is one more update for websocket compression: pool flate readers which should increase compression performance.

### Fixes

* Fix calling presence remove for every channel (not only channels with presence option enabled).
* Change subscribing/unsubscribing algorithm to Redis channels - it fixes theoretical possibility of wrong subscribing state in Redis.

### Internal (for developers/contributors)

* We don't use `disconnect` message before closing client connections anymore - we rely on websocket/SockJS close reason now (which is JSON encoded `DisconnectAdvice`). Our js client already handles that reason, so no breaking changes there. Some work required in other clients though to support `reconnect: false` in advice.

v1.6.1
======

This release fixes some configuration problems introduced by v1.6.0 and adds Let's Encrypt support.

### Features

* automatic TLS certificates from Let's Encrypt - see [#133](https://github.com/centrifugal/centrifugo/issues/133) and new [dedicated documentation chapter](https://fzambia.gitbooks.io/centrifugal/content/deploy/certificates.html)
* websocket compression performance improvement (due to Gorilla Websocket library update)

### Fixes

* fix SSL/TLS certificates file option names - see [#132](https://github.com/centrifugal/centrifugo/issues/132)
* fix `web` option that must enable admin socket automatically - see [#136](https://github.com/centrifugal/centrifugo/issues/136)

v1.6.0
======

This Centrifugo release is a massive 4-months refactoring of internals with the goal to separate code of different components such as engine, server, metrics, clients to own packages with well-defined API to communicate between them. The code layout changed dramatically. Look at `libcentrifugo` folder [before](https://github.com/centrifugal/centrifugo/tree/v1.5.1/libcentrifugo) and [after](https://github.com/centrifugal/centrifugo/tree/master/libcentrifugo)! Unfortunately there are backwards incompatibilities with previous release - see notes below. The most significant one is changed metrics format in `stats` and `node` API command responses.

With new code layout it's much more simple to create custom engines or servers – each with own metrics and configuration options. **We can not guarantee** though that we will keep `libcentrifugo` packages API stable – **our primary goal is still building Centrifugo standalone server**. So if we find something that must be fixed or improved internally - we will fix/improve it even if this could result in packages API changes.

As Centrifugo written in Go the only performant way to write plugins is to import them in `main.go` file and build Centrifugo with them. So if you want to create custom build with custom server or custom engine you will need to change `main.go` file and build Centrifugo yourself. But at least it's easier than supporting full Centrifugo fork.

### Release highlights:

* New metrics. Several useful new metrics have been added. For example HTTP API and client request HDR histograms. See updated documentation for complete list. Refactoring resulted in backwards incompatible issue when working with Centrifugo metrics (see below). [Here is a docs chapter](https://fzambia.gitbooks.io/centrifugal/content/server/stats.html) about metrics.
* Optimizations for client side ping, `centrifuge-js` now automatically sends periodic `ping` commands to server. Centrifugo checks client's last activity time and closes stale connections. Builtin SockJS server won't send heartbeat frames to SockJS clients by default. You can restore the old behaviour though: setting `ping: false` on client side and `sockjs_heartbeat_delay: 25` option in Centrifugo configuration. This all means that you better update `centrifuge-js` client to latest version (`1.4.0`). Read [more about pings in docs](https://fzambia.gitbooks.io/centrifugal/content/mixed/ping.html).
* Experimental websocket compression support for raw websockets - see [#115](https://github.com/centrifugal/centrifugo/issues/115). Read more details how to enable it [in docs](https://fzambia.gitbooks.io/centrifugal/content/mixed/websocket_compression.html). Keep in mind that enabling websocket compression can result in slower Centrifugo performance - depending on your load this can be noticeable.
* Serious improvements in Redis API queue consuming. There was a bottleneck as we used BLPOP command to get every message from Redis which resulted in extra RTT. Now it's fixed and we can get several API messages from queue at once and process them. The format of Redis API queue changed - see new format description [in docs](https://fzambia.gitbooks.io/centrifugal/content/server/engines.html). Actually it's now the same as single HTTP API command - so we believe you should be comfortable with it. Old format is still supported but **DEPRECATED** and will be removed in next releases.
* Redis sharding support. See more details [in docs](https://fzambia.gitbooks.io/centrifugal/content/server/scaling.html). This resolves some fears about Redis being bottleneck on some large Centrifugo setups. Though we have not heard such stories yet. Redis is single-threaded server, it's insanely fast but if your Redis approaches 100% CPU usage then this sharding feature is what can help your application to scale. 
* Many minor internal improvements.

### Fixes:

* This release fixes crash when `jsonp` transport was used by SockJS client - this was fixed recently in sockjs-go library.
* Memory bursts fixed on node shutdown when we gracefully disconnect many connected clients.

### Backwards incompatible changes:

* `stats` and `node` command response body format changed – metrics now represented as map containing string keys and integer values. So you may need to update your monitoring scripts.
* Default log level now is `info` - `debug` level is too chatty for production logs as there are tons of API requests per second, tons of client connect/disconnect events. If you still want to see logs about all connections and API requests - set `log_level` option to `debug`.
* `channel_prefix` option renamed to `redis_prefix`.
* `web_password` and `web_secret` option aliases not supported anymore. Use `admin_password` and `admin_secret`.
* `insecure_web` option removed – web interface now available without any password if `insecure_admin` option enabled. Of course in this case you should remember about protecting your admin endpoints with firewall rules. Btw we recommend to do this even if you are using admin password.
* Admin `info` response format changed a bit - but this most possibly will not affect anyone as it was mostly used by embedded web interface only.
* Some internal undocumented options have been changed. Btw [we documented several useful options](https://fzambia.gitbooks.io/centrifugal/content/server/advanced_configuration.html) which can be helpful in some cases.

### Several internal highlights (mostly for Go developers):

* Code base is more simple and readable now.
* Protobuf v3 for message schema (using gogoprotobuf library). proto2 and proto3 are wire compatible.
* Client transport now abstracted away - so it would be much easier in future to add new transport in addition/replacement to Websocket/SockJS.
* API abstracted away from protocol - it would be easier in future to add new API requests source.
* No performance penalty was introduced during this refactoring.
* We process PUB/SUB messages in several goroutines, preserving message order.
* We use `statik` (https://github.com/rakyll/statik) to embed web interface now.
* go1.7.4 used for builds.


v1.5.1
======

* Fixes [#94](https://github.com/centrifugal/centrifugo/issues).


v1.5.0
======

Some upgrade steps needed to migrate to this release, see below.

This release is a major refactoring of Centrifugo internal engine. Centrifugo now uses `protobuf` format while transferring data between nodes and serializing history/presence data in Redis. We expect that all those changes **should not affect your working application code** though as new serialization format used **you may need to run** `FLUSHDB` command if using Redis engine to remove presence/history data encoded as JSON (if you don't use presence and history then no need to do this). If you have any problems with new serialization format used by Centrifugo internally – we can consider making old JSON encoding optional in future releases. Depending on how you use Centrifugo with Redis Engine those internal changes can result in nearly the same performance (if real-time content mostly generated by users online, so messages coming to Centrifugo have to be delivered to at least one client) or up to 2x API request speed up (if your real-time mostly generated by application backend itself so some messages published in channel with no active subscribers and there is no need to do JSON encoding work).

* Using SockJS 1.1 by default (actually as by default we use jsdelivr.net CDN version it will be latest minor release - at moment of writing 1.1.1) – this means that you need to also upgrade SockJS on client side to the same version (because **some iframe-based SockJS transports require exact version match on client and server**). Note that you can set desired SockJS version used by Centrifugo server via `sockjs_url` option (which is now by default `//cdn.jsdelivr.net/sockjs/1.1/sockjs.min.js`). So if you don't want to upgrade SockJS on client side or just want to fix version used or use your own hosted SockJS library (which is good if you think about old browser support and don't want to be affected by minor SockJS library releases) - use that option.
* `centrifugo version` now shows Go language version used to build binary.
* Performance and memory allocation improvements.


v1.4.5
======

No backwards incompatible changes here. This release uses go1.6.2

* HTTP/2 support. This means that Centrifugo can utilize HTTP/2 protocol automatically - this is especially useful for HTTP-based SockJS transports (no limits for open connections per domain anymore). Note that HTTP/2 will work only when your setup utilizes `https`. Also HTTP/2 does not affect websockets because of missing protocol upgrade possibilities in HTTP/2 protocol - so websockets will work in the same way as before. Also if you have any proxy before Centrifugo then depending on your setup some reconfiguration may be required to make HTTP/2 work.

Just to remember how to test Centrifugo with SSL: [follow instructions](https://devcenter.heroku.com/articles/ssl-certificate-self) from Heroku article to generate self-signed certificate files. Then start Centrifugo like this:

```
./centrifugo --config=config.json --web --ssl --ssl_key=server.key --ssl_cert=server.crt
```

Go to https://localhost:8000/ and confirm that you trust certificate of this site (this is because of self-signed certificate, in case of valid certificate you don't need this step). Then you can test secure endpoint connections.


v1.4.4
======

One more fix for v1.4.2 release here

* proper aliasing of `admin_password` and `admin_secret` configuration options. See [#88](https://github.com/centrifugal/centrifugo/issues/88) 


v1.4.3
======

**Fix of security vulnerability introduced in v1.4.2**, see below.

* If you are using Centrifugo v1.4.2 (previous versions not affected) with admin socket enabled (with `--admin` or `--web` options) and your admin endpoint not protected by firewall somehow then you must update to this version. Otherwise it's possible to connect to admin websocket endpoint and run any command without authentication. It's recommended to update your secret key after upgrade. So sorry for this.


v1.4.2
======

* Redis Sentinel support for Redis high availability setup. [Docs](https://fzambia.gitbooks.io/centrifugal/content/deploy/sentinel.html)
* Redis Engine now uses Redis pipeline for batching publish operations - this results in latency and throughput improvments when publish rate is high.
* Refactored admin websocket. New option `admin` to enable admin websocket. New option `insecure_admin` to make this endpoint insecure (useful when admin websocket endpoint/port protected by firewall rules). `web_password` option renamed to `admin_password`, `web_secret` option renamed to `admin_secret`, `insecure_web` renamed to `insecure_admin`. **But all old option names still supported to not break things in existing setups**. Also note, that when you run Centrifugo with `web` interface enabled - you also make admin websocket available, because web interface uses it. A little more info [in pull request](https://github.com/centrifugal/centrifugo/pull/83).
* Presence Redis Engine methods rewritten to lua to be atomic.
* Some Redis connection params now can be set over environment variables. See [#81](https://github.com/centrifugal/centrifugo/issues/81)
* Fix busy loop when attempting to reconnect to Redis. Fixes large CPU usage while reconnecting.
* Shorter message `uid`s (22 bytes instead of 36). This was made in order to get some performance improvements.


v1.4.1
======

* fix server crash on 32-bit architectures (due to [this](https://golang.org/src/sync/atomic/doc.go?s=1207:1656#L36)), see more details in [#74](https://github.com/centrifugal/centrifugo/issues/74).
* fix compatibility with gocent introduced in v1.4.0


v1.4.0
======

No backwards incompatible changes here for most usage scenarios, but look carefully on notes below.

* Timers in metrics marked as deprecated. `time_api_mean`, `time_client_mean`, `time_api_max`, `time_client_max` now return 0. This was made because timer's implementation used `Timer` from `go-metrics` library that does not suit very well for Centrifugo needs - so values were mostly useless in practice. So we decided to get rid of them for now to not confuse our users.
* New `node` API method to get information from single node. That information will contain counters without aggregation over minute interval (what `stats` method does by default). So it can be useful if your metric aggregation system can deal with non-aggregated counters over time period itself. Also note that to use this method you should send API request to each Centrifugo node separately - as this method returns current raw statistics about one node. See [issue](https://github.com/centrifugal/centrifugo/issues/68) for motivation description.
* Centrifugo now handles SIGTERM in addition to SIGINT and makes `shutdown` when this signal received. During shutdown Centrifugo returns 503 status code on requests to handlers and closes client connections so clients will reconnect. If shutdown finished without errors in 10 seconds interval then Centrifugo exits with status code 0 (instead of 130 before, this fixes behaviour behind `systemd` after SIGTERM received).
* Maximum limit in bytes for client request was added. It can be changed using `client_request_max_size` config option. By default 65536 bytes (64kb).
* Packages for 64-bit Debian, Centos and Ubuntu [hosted on packagecloud.io](https://packagecloud.io/FZambia/centrifugo). If you are using Debian 7 or 8, Centos 6 or 7, Ubuntu 14.04 or Ubuntu 16.04 - you can find packages for those linux distribution following to packagecloud. Packages will be created every time we release new Centrifugo version.


v1.3.3
======

No backwards incompatible changes here

* fix automatic presence expire in Redis engine - could lead to small memory leaks in Redis when using presence. Also could result in wrong presence information after non-graceful Centrifugo node shutdown.
* configurable limit for amount of channels each client can subscribe to. Default `100`. Can be changed using `client_channel_limit` configuration option.


v1.3.2
======

This release built using go 1.5.3 and [includes security fix in Go lang](https://groups.google.com/forum/#!topic/golang-announce/MEATuOi_ei4)

* empty errors not included in client response (**this requires using Javascript client >= 1.1.0**)
* optimization in Redis engine when using history - one round trip to Redis to publish message and save it into history instead of two. This was done over registering lua script on server start.
* client errors improvements - include error advice when error occurred (fix or retry at moment)

Also note that Javascript client will be fully refreshed soon. See [this pull request](https://github.com/centrifugal/centrifuge-js/pull/7)

v1.3.1
======

* fix port configuration introduced in v1.3.0: `--port` should override default values for `admin_port` and `api_port`
* use the same (http/https) scheme for Sockjs default iframe script source.
* fix possible deadlock in shutdown

v1.3.0
======

Possible backwards incompatibility here (in client side code) - see first point.

* omit fields in message JSON if field contains empty value: `client` on top level, `info` on top level, `default_info` in `info` object, `channel_info` in `info` object. This also affects top level data in join/leave messages and presence data – i.e. `default_info` and `channel_info` keys not included in JSON when empty. This can require adapting your client side code a bit if you rely on these keys but for most cases this should not affect your application. But we strongly suggest to test before updating. This change allows to reduce message size. See migration notes below for more details.
* new option `--admin_port` to bind admin websocket and web interface to separate port. [#44](https://github.com/centrifugal/centrifugo/issues/44)
* new option `--api_port` to bind API endpoint to separate port. [#44](https://github.com/centrifugal/centrifugo/issues/44)
* new option `--insecure_web` to use web interface without setting `web_password` and `web_secret` (for use in development or when you protected web interface by firewall rules). [#44](https://github.com/centrifugal/centrifugo/issues/44)
* new channel option `history_drop_inactive` to drastically reduce resource usage (engine memory, messages travelling around) when you use message history. See [#50](https://github.com/centrifugal/centrifugo/issues/50)
* new Redis engine option `--redis_api_num_shards`. This option sets a number of Redis shard queues Centrifugo will use in addition to standard `centrifugo.api` queue. This allows to increase amount of messages you can publish into Centrifugo and preserve message order in channels. See [#52](https://github.com/centrifugal/centrifugo/issues/52) and [documentation](https://fzambia.gitbooks.io/centrifugal/content/server/engines.html) for more details.
* fix race condition resulting in client disconnections on high channel subscribe/unsubscribe rate. [#54](https://github.com/centrifugal/centrifugo/issues/54)
* refactor `last_event_id` related stuff to prevent memory leaks on large amount of channels. [#48](https://github.com/centrifugal/centrifugo/issues/48)
* send special disconnect message to client when we don't want it to reconnect to Centrifugo (at moment to client sending malformed message). 
* pong wait handler for raw websocket to detect non responding clients.

Also it's recommended to update javascipt client to latest version as it has some useful changes (see its changelog).

How to migrate
--------------

Message before:

```javascript
{
	"uid":"442586d4-688c-4a0d-52ad-d0a13d201dfc",
	"timestamp":"1450817253",
	"info": null,
	"channel":"$public:chat",
	"data":{"input":"1"},
	"client":""
}
```

Message now:

```javascript
{
	"uid":"442586d4-688c-4a0d-52ad-d0a13d201dfc",
	"timestamp":"1450817253",
	"channel":"$public:chat",
	"data":{"input":"1"},
}
```

I.e. not using empty `client` and `info` keys. If those keys are non empty then they present in message.

Join message before:

```javascript
{
	"user":"2694",
	"client":"93615872-4e45-4da2-4733-55c955133436",
	"default_info": null,
	"channel_info":null
}
```

Join message now:

```javascript
{
	"user":"2694",
	"client":"93615872-4e45-4da2-4733-55c955133436"
}
```

If "default_info" or "channel_info" exist then they would be included:

```javascript
{
	"user":"2694",
	"client":"93615872-4e45-4da2-4733-55c955133436",
	"default_info": {"username": "FZambia"},
	"channel_info": {"extra": "some data here"}
}
```


v1.2.0
======

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
