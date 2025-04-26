Centrifugo is an open-source scalable real-time messaging server. Centrifugo can instantly deliver messages to application online users connected over supported transports (WebSocket, HTTP-streaming, Server-Sent Events (SSE/EventSource), GRPC, WebTransport). Centrifugo has the concept of a channel – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Several official client SDKs for browser and mobile development wrap the bidirectional protocol. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev).

## What's changed

### Improvements

* Added five new built-in [async consumers](https://centrifugal.dev/docs/server/consumers) in [#968](https://github.com/centrifugal/centrifugo/pull/968). We believe having them in Centrifugo can drastically simplify introducing real-time messages in existing systems, built on modern cloud infrastructures. In addition to [Kafka](https://centrifugal.dev/docs/server/consumers#kafka-consumer) and [PostgreSQL](https://centrifugal.dev/docs/server/consumers#postgresql-outbox-consumer) outbox table, Centrifugo now has consumers from the following popular messaging systems:
  * [Redis Stream](https://redis.io/docs/latest/develop/data-types/streams/), see integration [docs](https://centrifugal.dev/docs/server/consumers#redis-stream)
  * [Nats Jetstream](https://docs.nats.io/nats-concepts/jetstream), see integration [docs](https://centrifugal.dev/docs/server/consumers#nats-jetstream)
  * [Google Cloud PUB/SUB](https://cloud.google.com/pubsub/docs/pubsub-basics), see integration [docs](https://centrifugal.dev/docs/server/consumers#nats-jetstream)
  * [AWS SQS](https://aws.amazon.com/sqs/), see integration [docs](https://centrifugal.dev/docs/server/consumers#aws-sqs)
  * [Azure Service Bus](https://learn.microsoft.com/en-us/azure/service-bus-messaging/service-bus-messaging-overview), see integration [docs](https://centrifugal.dev/docs/server/consumers#azure-service-bus)
  * Currently, docs may lack some details but hopefully will be improved with time and feedback received. Please reach out in the [community](https://centrifugal.dev/docs/getting-started/community) channels if you have any questions or suggestions.
* Skip unordered publications at the Broker level based on `version` and `version_epoch` fields [#971](https://github.com/centrifugal/centrifugo/pull/971). See updated [publish](https://centrifugal.dev/docs/server/server_api#publish) and [broadcast](https://centrifugal.dev/docs/server/server_api#broadcast) API docs. Note, this is mostly useful for a scenario, when each channel publication has the entire state instead of incremental updates – so skipping intermediate publications is safe and beneficial (especially given Centrifugo now has more built-in asynchronous consumers, and some of them can not provide ordered processing).
* Added support for specifying the API command method in messages consumed by asynchronous consumers via a header, property, or attribute. When this is provided, the message payload must be a serialized API request object (as used in the [server HTTP API](https://centrifugal.dev/docs/server/server_api)). The exact mechanism is consumer-specific. For example, the Kafka consumer supports a `centrifugo-method` header, which can be set to values like `publish`, `broadcast`, or `send_push_notification` (and so on), allowing the message payload to be a JSON API request. This approach improves decoding efficiency and may open a road for using binary encoded command requests in the future.
* New [configdoc](https://centrifugal.dev/docs/server/console_commands#configdoc) CLI helper to display the full Centrifugo configuration as HTML or Markdown [#959](https://github.com/centrifugal/centrifugo/pull/959) — run `centrifugo configdoc` to see it in action.
* Added support for tags in publications from the Nats broker [#964](https://github.com/centrifugal/centrifugo/pull/964).
* Handle optional `alg` field in JWKS [#962](https://github.com/centrifugal/centrifugo/pull/962), resolves [#961](https://github.com/centrifugal/centrifugo/issues/961).
* Log whether FIPS mode is enabled during Centrifugo startup [#975](https://github.com/centrifugal/centrifugo/pull/975). See [FIPS 140-3 Compliance](https://go.dev/doc/security/fips140) document (part of Go 1.24 release).
* Performance: Removed allocation during WebSocket subprotocol selection [centrifugal/centrifuge#476](https://github.com/centrifugal/centrifuge/pull/476) during Upgrade.
* Performance: Slightly improving client queue processing by fetching messages in batch instead of one-by-one.

### Fixes

* Fixed concurrent map iteration and write panic occurring during Redis issues [centrifugal/centrifuge#473](https://github.com/centrifugal/centrifuge/pull/473).
* Fixed unmarshalling of `duration` type from environment variable JSON [#973](https://github.com/centrifugal/centrifugo/pull/973).
* Fixed an issue where `channel_replacements` were not applied when publishing to a channel via the Centrifugo API in NATS Raw Mode. See [#977](https://github.com/centrifugal/centrifugo/issues/977).

### New tutorial in blog

To support the changes in this release, we published a new blog post:

[Building a real-time WebSocket leaderboard with Centrifugo and Redis](/blog/2025/04/28/websocket-real-time-leaderboard).

In it, we create a real-time leaderboard using Centrifugo, Redis, React and Python. Showing the usage of Centrifugo built-in asynchronous consumer from Redis Stream and using `version`/`version_epoch` fields. The post additionally showcases Fossil delta compression and cache recovery mode.

### Miscellaneous

* New animation on https://centrifugal.dev/ – with burstable bubbles upon click, your kids will love it 🙃
* Cleanup in `api.proto` — removed messages related to the legacy HTTP API.
* This release is built with Go 1.24.2.
* See also the corresponding [Centrifugo PRO release](https://github.com/centrifugal/centrifugo-pro/releases/tag/v6.2.0).
