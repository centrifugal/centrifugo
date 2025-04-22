Centrifugo is an open-source scalable real-time messaging server. Centrifugo can instantly deliver messages to application online users connected over supported transports (WebSocket, HTTP-streaming, Server-Sent Events (SSE/EventSource), GRPC, WebTransport). Centrifugo has the concept of a channel – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Several official client SDKs for browser and mobile development wrap the bidirectional protocol. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev).

## What's changed

### Improvements

* Add five new [async consumers](https://centrifugal.dev/docs/server/consumers) [#968](https://github.com/centrifugal/centrifugo/pull/968):
  * [Google Cloud PUB/SUB](https://cloud.google.com/pubsub/docs/pubsub-basics)
  * [AWS SQS](https://aws.amazon.com/sqs/)
  * [Azure Service Bus](https://learn.microsoft.com/en-us/azure/service-bus-messaging/service-bus-messaging-overview)
  * [Redis Stream](https://redis.io/docs/latest/develop/data-types/streams/)
  * [Nats Jetstream](https://docs.nats.io/nats-concepts/jetstream)
* Skip unordered publications on Broker level based on `version` and `version_epoch` fields [#971](https://github.com/centrifugal/centrifugo/pull/971)
* Kafka consumer: request type in the payload if method header provided, publication data mode improvements [#956](https://github.com/centrifugal/centrifugo/pull/956)
* New `configdoc` cli helper to display the entire configuration as HTML or Markdown [#959](https://github.com/centrifugal/centrifugo/pull/959) – run `./centrifugo configdoc` to see it in action.
* Support tags and time fields in publication from Nats broker [#964](https://github.com/centrifugal/centrifugo/pull/964)
* Handle jwks optional alg [#962](https://github.com/centrifugal/centrifugo/pull/962), solves [#961](https://github.com/centrifugal/centrifugo/issues/961)
* Performance: remove allocation during subprotocol selection [centrifugal/centrifuge#476](https://github.com/centrifugal/centrifuge/pull/476)
* Performance: getting messages from Client's queue in batch instead of one by one separate calls.
* Log whether FIPS mode enabled on Centrifugo start [#975](https://github.com/centrifugal/centrifugo/pull/975)

### Fixes

* Fix concurrent map iteration and write panic happening during issues with Redis [centrifugal/centrifuge#473](https://github.com/centrifugal/centrifuge/pull/473)
* Fix unmarshalling of duration type in environment variable json [#973](https://github.com/centrifugal/centrifugo/pull/973)

### Miscellaneous

* Cleanups in `api.proto` – messages related to legacy HTTP API were removed.
* This release is built with Go 1.24.2.
* See also the corresponding [Centrifugo PRO release](https://github.com/centrifugal/centrifugo-pro/releases/tag/v6.2.0).
