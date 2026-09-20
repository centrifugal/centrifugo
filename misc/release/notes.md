Centrifugo is an open-source scalable real-time messaging server. It instantly delivers messages to application online users connected over supported transports (WebSocket, HTTP-streaming, Server-Sent Events (SSE), GRPC, WebTransport). Centrifugo is built around channel subscriptions – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, AI streaming responses, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Official client SDKs are available for JavaScript (browser, Node.js, React Native), Dart/Flutter, Swift, Java, Python, Go, and .NET. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev). For runnable demos see [centrifugal/examples](https://github.com/centrifugal/examples).

## What's changed

### Improvements

* Publications into a channel are no longer delayed by clients subscribing to it with recovery or positioning (see the fix below). In a benchmark which publishes into a channel with 100 subscribers while other clients connect with it and a second channel whose history read takes 1ms, publish latency dropped from 249µs to 28µs on average and from 1240µs to 46µs at p99. Broadcasting to a client which is not subscribing to anything now costs an atomic load instead of a mutex and a map lookup (a broadcast to 100 subscribers went from 27.4µs to 23.9µs), and a new connection allocates 8 times instead of 10, since the recovery sync state is only initialized upon the first recovering subscribe ([centrifugal/centrifuge#640](https://github.com/centrifugal/centrifuge/pull/640)).
* Faster fossil delta encoding – Centrifugo now uses [centrifugal/fdelta](https://github.com/centrifugal/fdelta) package instead of `shadowspore/fossil-delta`. Creating a delta is 3.5-4.2x faster on JSON payloads and allocates once instead of five times (128 B/op at 16 KiB against 19 KB). Produced deltas are byte-compatible in both directions, so already deployed SDKs apply them unchanged ([centrifugal/centrifuge#638](https://github.com/centrifugal/centrifuge/pull/638)).
* Less work per broadcast when delta compression is used by JSON clients. A JSON client receives a delta as a JSON string, so a delta may only be built when the previous payload is valid UTF-8. That check ran once for every distinct encoding combination among a channel's subscribers, and again for the broker's and the node's previous publication – re-scanning the same bytes. Now a payload is scanned at most once per broadcast, and not at all when no JSON client takes deltas. It only matters for payloads carrying non-ASCII text (`utf8.Valid` has an ASCII fast path) – on such payloads the broadcast is 12-21% faster depending on the number of encoding combinations in the channel ([centrifugal/centrifuge#637](https://github.com/centrifugal/centrifuge/pull/637)).

### Fixes

* Fix a possible node deadlock when a client subscribes to a channel with recovery or positioning enabled. Such a subscription kept its recovery buffer locked until the subscribe result was written, while a publication into the channel waited for that buffer holding the hub shard lock – so publications, subscribes, unsubscribes, disconnects and graceful shutdown for all channels of that shard could block. Publications no longer wait for a subscribing client ([centrifugal/centrifuge#640](https://github.com/centrifugal/centrifuge/pull/640)).
* Delta compression and publication tags filters are documented as mutually exclusive, but Centrifugo still negotiated delta for a subscription with a tags filter. To keep the chain of deltas intact, publications filtered out for such a subscriber were sent to it anyway – so the filter was not applied at all. Delta is no longer negotiated for subscriptions with a tags filter ([centrifugal/centrifuge#634](https://github.com/centrifugal/centrifuge/pull/634)).
* Map channels: when a channel's stream expired while its top offset and epoch survived, a read since an older offset returned an empty result with no error – recovery reported `recovered: true` while silently skipping the lost publications. Such a read now results in an unrecoverable position error, so clients resync from the full state ([centrifugal/centrifuge#636](https://github.com/centrifugal/centrifuge/pull/636)).

### Miscellaneous

* This release is built with Go 1.26.8.
* Dependency updates.
* See also the corresponding [Centrifugo PRO release](https://github.com/centrifugal/centrifugo-pro/releases/tag/v6.9.7).
