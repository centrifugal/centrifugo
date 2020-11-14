Minor backwards incompatible changes here when using `client_user_connection_limit` option – see below.

Centrifugo v2.8.0 has many internal changes that could affect overall performance and stability. In general, we expect better latency between a client and a server, but servers under heavy load can notice a small regression in CPU usage.

Improvements:

* Centrifugo can now maintain single connection from a user when personal server-side channel used. See [#396](https://github.com/centrifugal/centrifugo/issues/396) and [docs](https://centrifugal.github.io/centrifugo/server/server_subs/#maintain-single-user-connection) 
* New option `client_concurrency`. This option allows processing client commands concurrently. Depending on your use case this option has potential to radically reduce latency between client and Centrifugo. See [detailed description in docs](https://centrifugal.github.io/centrifugo/server/configuration/#client_concurrency)
* When using `client_user_connection_limit` and user reaches max amount of connections Centrifugo will now disconnect client with `connection limit` reason instead of returning `limit exceeded` error. Centrifugo will give a client advice to not reconnect.

Centrifugo v2.8.0 based on latest Go 1.15.5
