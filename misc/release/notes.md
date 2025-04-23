Centrifugo is an open-source scalable real-time messaging server. Centrifugo can instantly deliver messages to application online users connected over supported transports (WebSocket, HTTP-streaming, Server-Sent Events (SSE/EventSource), GRPC, WebTransport). Centrifugo has the concept of a channel – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Several official client SDKs for browser and mobile development wrap the bidirectional protocol. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev).

## What's changed

### Improvements

* Added five new built-in [async consumers](https://centrifugal.dev/docs/server/consumers) in [#968](https://github.com/centrifugal/centrifugo/pull/968). We believe having them in Centrifugo can drastically simplify introducing real-time messages in existing systems, built on modern cloud infrastructures. In addition to [Kafka](https://centrifugal.dev/docs/server/consumers#kafka-consumer) and [PostgreSQL](https://centrifugal.dev/docs/server/consumers#postgresql-outbox-consumer) outbox table, Centrifugo now has consumers from the following popular messaging systems:
  * [Redis Stream](https://redis.io/docs/latest/develop/data-types/streams/), [docs](https://centrifugal.dev/docs/server/consumers#redis-stream)
  * [Nats Jetstream](https://docs.nats.io/nats-concepts/jetstream), [docs](https://centrifugal.dev/docs/server/consumers#nats-jetstream)
  * [Google Cloud PUB/SUB](https://cloud.google.com/pubsub/docs/pubsub-basics), [docs](https://centrifugal.dev/docs/server/consumers#nats-jetstream)
  * [AWS SQS](https://aws.amazon.com/sqs/), [docs](https://centrifugal.dev/docs/server/consumers#aws-sqs)
  * [Azure Service Bus](https://learn.microsoft.com/en-us/azure/service-bus-messaging/service-bus-messaging-overview), [docs](https://centrifugal.dev/docs/server/consumers#azure-service-bus)
* Skip unordered publications at the Broker level based on `version` and `version_epoch` fields [#971](https://github.com/centrifugal/centrifugo/pull/971). See updated [publish](https://centrifugal.dev/docs/server/server_api#publish) and [broadcast](https://centrifugal.dev/docs/server/server_api#broadcast) API docs. Note, this is mostly useful for a scenario, when each channel publication has the entire state instead of incremental updates – so skipping intermediate publications is safe and beneficial (especially given Centrifugo now has more built-in asynchronous consumers, and some of them can not provide ordered processing).
* New option for each asynchronous consumer configuration object: `api_command_format`. By default, this option's value is `method_payload`, and Centrifugo expects JSON object with `method` and `payload` fields in the message content received from external system. This is friendly for PostgreSQL outbox and its CDC integrations. But we want a more native integration with Centrifugo Protobuf schema. Now, if `api_command_format` is set to `proto_command` the content of message may represent `Command` message from Centrifugo [server API Protobuf definitions](https://github.com/centrifugal/centrifugo/blob/f7e28e93636d207e06ad397b46b20128238e67dc/internal/apiproto/api.proto#L43). For now, we still only support JSON message. But the new supported format is more efficient for deserialization, and opens a road for binary Protobuf encoding of the payload in the future in a schema-friendly way.
* New `configdoc` CLI helper to display the full Centrifugo configuration as HTML or Markdown [#959](https://github.com/centrifugal/centrifugo/pull/959) — run `./centrifugo configdoc` to see it in action.
* Added support for tags in publications from the Nats broker [#964](https://github.com/centrifugal/centrifugo/pull/964).
* Handle optional `alg` field in JWKS [#962](https://github.com/centrifugal/centrifugo/pull/962), resolves [#961](https://github.com/centrifugal/centrifugo/issues/961).
* Log whether FIPS mode is enabled during Centrifugo startup [#975](https://github.com/centrifugal/centrifugo/pull/975). See [FIPS 140-3 Compliance](https://go.dev/doc/security/fips140) document (part of Go 1.24 release).
* Performance: Removed allocation during WebSocket subprotocol selection [centrifugal/centrifuge#476](https://github.com/centrifugal/centrifuge/pull/476) during Upgrade.
* Performance: Slightly improving client queue processing by fetching messages in batch instead of one-by-one.

### Fixes

* Fixed concurrent map iteration and write panic occurring during Redis issues [centrifugal/centrifuge#473](https://github.com/centrifugal/centrifuge/pull/473).
* Fixed unmarshalling of `duration` type from environment variable JSON [#973](https://github.com/centrifugal/centrifugo/pull/973).

### Miscellaneous

* New animation on https://centrifugal.dev/ – with burstable bubbles.
* Cleanup in `api.proto` — removed messages related to the legacy HTTP API.
* This release is built with Go 1.24.2.
* See also the corresponding [Centrifugo PRO release](https://github.com/centrifugal/centrifugo-pro/releases/tag/v6.2.0).
