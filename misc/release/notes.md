Centrifugo is an open-source scalable real-time messaging server. Centrifugo can instantly deliver messages to application online users connected over supported transports (WebSocket, HTTP-streaming, SSE/EventSource, GRPC, SockJS, WebTransport). Centrifugo has the concept of a channel – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Several official client SDKs for browser and mobile development wrap the bidirectional protocol. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev).

## What's changed

### Improvements

* Centrifugo finally has an official tutorial which shows the process of building real-time application in detail. It's called [Grand tutorial](https://centrifugal.dev/docs/tutorial/intro), and we have a special section on the site for it. In the tutorial we build a WebSocket chat (messenger) app with Django, React and Centrifugo. We cover many aspects of building a production-grade app with Centrifugo. The idea to have it in a separate section of the doc site instead of a blog is to maintain the tutorial actual and extend as time goes.
* Introducing built-in [asynchronous consumers](https://centrifugal.dev/docs/server/consumers). From [PostgreSQL outbox table](https://centrifugal.dev/docs/server/consumers#postgresql-outbox-consumer) (to support transactional outbox out-of-the-box) and from [Kafka topics](https://centrifugal.dev/docs/server/consumers#kafka-consumer). Note that grand tutorial mentioned above shows using both built-in consumers in action, including CDC approach to stream data to Centrifugo with Kafka Connect using Debezium connector for PostgreSQL. 
* It's now possible to provide an `idempotency_key` when calling `publish` or `broadcast` server API methods. This is a key Centrifugo will use to prevent duplicate sending of the same publication to a channel. This allows making effective retries when publishing to Centrifugo. Centrifugo keeps a cache with results that used idempotency keys during publishing for 5 minutes and uses it to prevent publishing a message and prevent adding it to history, if already presented in cache. See updated docs for [publish](https://centrifugal.dev/docs/server/server_api#publish) and [broadcast](https://centrifugal.dev/docs/server/server_api#publish) server APIs.
* Refactor `MakeTLSConfig` and support mTLS on server by @tie, see [#739](https://github.com/centrifugal/centrifugo/pull/739). This means mTLS is supported for all TLS configurations in Centrifugo – by using TLS options `tls_client_ca` or `tls_client_ca_pem`. 
* Added possibility to set `parallel` boolean flag in [batch](https://centrifugal.dev/docs/server/server_api#batch) API – to make batch commands processing parallel on Centrifugo side. This may provide reduced latency (especially in case of using Redis engine as you don't need to wait N * RTT time for sequential command processing).

### Misc

* Release is built using Go v1.21.5
* Slightly improved code base to use `Service` interface with `Run(ctx context.Context) error` method when extending core Centrifugo functionality. This is now a recommended way for proper start and shutdown of different services in Centrifugo.
