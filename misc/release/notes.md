Centrifugo is an open-source scalable real-time messaging server. Centrifugo can instantly deliver messages to application online users connected over supported transports (WebSocket, HTTP-streaming, SSE/EventSource, GRPC, WebTransport). Centrifugo has the concept of a channel – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Several official client SDKs for browser and mobile development wrap the bidirectional protocol. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev).

## What's changed

Centrifugo v5.4.0 is now available, featuring several notable improvements.

### Improvements

* [Delta Compression](https://centrifugal.dev/docs/server/delta_compression) in channels. Utilizing the Fossil delta algorithm, this feature reduces bandwidth costs by sending only the differences from the previous publication. The effectiveness of delta compression varies depending on the data within the channels. It requires support from the Centrifugo SDK and is only available in the Centrifugo JavaScript SDK at this point (starting from centrifuge-js v5.2.0), and only for client-side subscriptions. For more insights about delta compression, and also other compression configurations within different Centrifugo protocol formats, read our new blog post [Experimenting with Real-Time Data Compression by Simulating Football Match Events](https://centrifugal.dev/blog/2024/05/27/real-time-data-compression-experiments).
* [Cache Recovery Mode](https://centrifugal.dev/docs/server/cache_recovery) in channels. This allows Centrifugo to function as a real-time key-value store. When a client first subscribes to a channel with recovery enabled and recover flag on, the latest publication from the history stream is immediately sent to the subscriber. On resubscription, the latest publication is also delivered (only if needed, based on the provided offset). This improvement addresses [#745](https://github.com/centrifugal/centrifugo/issues/745).
* A new metric, `centrifugo_client_num_server_unsubscribes` (counter), shows server-initiated unsubscribe pushes with the `code` label.
* A new option, `client_connect_include_server_time`, includes server `time` in the connection reply/push [#817], resolves [#787](https://github.com/centrifugal/centrifugo/issues/787). The time is sent as a Unix timestamp in milliseconds. Currently, SDK support is not available since the use case in the issue involves unidirectional transport, but we plan adding time to the `connected` event context in future SDK releases.
* Kafka Consumer: Proper parallelism for partition processing [#814](https://github.com/centrifugal/centrifugo/pull/814) – ensures that the processing of all partitions is not blocked when a single partition is blocked for some reason.
* Performance optimizations for WebSocket Upgrade. The number of allocations per Upgrade operation has been reduced, making connection establishment stage slightly more efficient.

### Miscellaneous

* Release is built with Go 1.22.3
* All dependencies were updated to latest versions
* Improvements in documentation - several typos fixed, more clear description of features, new diagrams. Also, one Easter Egg (tip: set `lights` to `up` in localStorage, use dark theme).
