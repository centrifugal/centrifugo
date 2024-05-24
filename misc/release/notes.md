Centrifugo is an open-source scalable real-time messaging server. Centrifugo can instantly deliver messages to application online users connected over supported transports (WebSocket, HTTP-streaming, SSE/EventSource, GRPC, WebTransport). Centrifugo has the concept of a channel – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Several official client SDKs for browser and mobile development wrap the bidirectional protocol. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev).

## What's changed

Centrifugo v5.4.0 is out with several nice improvements.

### Improvements

* [Delta compression in channels](https://centrifugal.dev/docs/server/delta_compression) using Fossil delta algorithm – this is a way to reduce bandwidth costs by only sending the diff from the previous publication. The benefit from enabling delta compression in channels highly depends on data in channels. Delta compression requires Centrifugo SDK to support it, and at this point is only supported by Centrifugo Javascript SDK (starting since `centrifuge-js` v5.2.0), and only for client-side subscriptions. Also, check out a new post in Centrifugal blog - [Experimenting with real-time data compression by simulating a football match events](https://centrifugal.dev/blog/2024/05/27/real-time-data-compression-experiments) – which is very relevant and provides curious insights about data compression when using various Centrifugo configurations.
* [Cache recovery mode](https://centrifugal.dev/docs/server/cache_recovery) in channels. It allows using Centrifugo as sth like a real-time key-value store – as soon as a client subscribes to a channel (and uses recovery while subscribing) – the latest publication from history stream will be immediately sent to a subscriber. Upon resubscribe/reconnect – publication is also immediately delivered. Resolves [#745](https://github.com/centrifugal/centrifugo/issues/745).
* New metric `centrifugo_client_num_server_unsubscribes` (counter) to show server initiated unsubscribe pushes with `code` label.
* New option `client_connect_include_server_time` to include server time in connect reply/push [#817], relates [#787](https://github.com/centrifugal/centrifugo/issues/787). Time is sent as Unix timestamp in milliseconds. At this point we do not support it in SDKs since the use case in the issue uses unidirectional transport, but we will add `time` to `connected` event context in future releases.
* Kafka consumer: proper parallelism for partition processing [#814](https://github.com/centrifugal/centrifugo/pull/814) – this helps to avoid blocking all partitions processing when single partition is blocked for some reason.
* Performance optimizations for WebSocket Upgrade – number of allocations per operation reduced from 9 to 3, so connection establishment is slightly more efficient now.

### Misc

* Release is built with Go 1.22.3
* All dependencies were updated to latest versions
* Improvements in documentation - several typos fixed, more clear description of features, new diagrams. Also, one Easter Egg (tip: set `lights` to `up` in localStorage, use dark theme)
