Centrifugo is an open-source scalable real-time messaging server. Centrifugo can instantly deliver messages to application online users connected over supported transports (WebSocket, HTTP-streaming, SSE/EventSource, GRPC, SockJS, WebTransport). Centrifugo has the concept of a channel – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Several official client SDKs for browser and mobile development wrap the bidirectional protocol. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev).

## What's changed

### Improvements

* Launching the official tutorial showing the process of building complex real-time application with Centrifugo in detail. It's called [Grand Tutorial](https://centrifugal.dev/docs/tutorial/intro), and we have a special section on the site for it. In the tutorial we build a WebSocket chat (messenger) app with Django, React and Centrifugo. We cover various aspects of building a production-grade app with Centrifugo. The idea to keep it in a separate section on the site instead of a blog is to maintain the tutorial actual and extend as time goes.
* Introducing built-in [asynchronous consumers](https://centrifugal.dev/docs/server/consumers). From [PostgreSQL outbox table](https://centrifugal.dev/docs/server/consumers#postgresql-outbox-consumer) (to natively support transactional outbox) and from [Kafka topics](https://centrifugal.dev/docs/server/consumers#kafka-consumer). Notably, the "Grand Tutorial" mentioned earlier demonstrates the practical use of both built-in consumers, including the Change Data Capture (CDC) approach for streaming data to Centrifugo using the Debezium connector for PostgreSQL with Kafka Connect.
* Centrifugo now offers the capability to specify an `idempotency_key` when invoking `publish` or `broadcast` server API methods. This is a key Centrifugo will use to prevent duplicate sending of the same publication to a channel. This allows making effective retries when publishing to Centrifugo. Centrifugo maintains a cache of results associated with the used idempotency keys during publishing for a duration of 5 minutes. This cache is utilized to prevent the redundant publication of a message and its addition to the message history if it already exists in the cache. See updated docs for [publish](https://centrifugal.dev/docs/server/server_api#publish) and [broadcast](https://centrifugal.dev/docs/server/server_api#publish) server APIs.
* Refactor `MakeTLSConfig` and support mTLS on server by @tie, see [#739](https://github.com/centrifugal/centrifugo/pull/739). This means mTLS is supported for all TLS configurations in Centrifugo – by using TLS options `tls_client_ca` or `tls_client_ca_pem`. 
* Added possibility to set `parallel` boolean flag in [batch](https://centrifugal.dev/docs/server/server_api#batch) API – to make batch commands processing parallel on Centrifugo side, potentially reducing latency, especially when using the Redis engine, as there is no need to wait for N * RTT (Round Trip Time) for sequential command processing.

### Misc

* Release is built using Go v1.21.5
* Improvements to the code base by introducing the use of the `Service` interface with a `Run(ctx context.Context) error` method. This approach is now recommended for the proper initiation and termination of various extension services within Centrifugo.
