Centrifugo is an open-source scalable real-time messaging server. It instantly delivers messages to application online users connected over supported transports (WebSocket, HTTP-streaming, Server-Sent Events (SSE), GRPC, WebTransport). Centrifugo is built around channel subscriptions – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, AI streaming responses, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Official client SDKs are available for JavaScript (browser, Node.js, React Native), Dart/Flutter, Swift, Java, Python, Go, and .NET. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev). For runnable demos see [centrifugal/examples](https://github.com/centrifugal/examples).

## What's changed

### Improvements

* New [`auto_cache_recover`](https://centrifugal.dev/docs/server/cache_recovery#automatic-cache-recovery) channel namespace option to automatically recover subscriptions on (re)subscribe without the subscriber requesting recovery itself, see [#1158](https://github.com/centrifugal/centrifugo/pull/1158). In [cache recovery mode](https://centrifugal.dev/docs/server/cache_recovery) this delivers the latest channel publication on every (re)subscribe – without the client providing an empty `since` position itself. It also enables this for server-side subscriptions, which is especially useful for unidirectional clients that may not even know channel names. The option requires `force_recovery` and `force_recovery_mode` set to `cache`. Subscribe and connect proxies may also enable it per subscription with the new `cache_recover` field in [`SubscribeOptions`](https://centrifugal.dev/docs/server/proxy#subscribeoptions).
* OpenTelemetry: Centrifugo node ID is now used as the `service.instance.id` resource attribute, see [#1155](https://github.com/centrifugal/centrifugo/pull/1155). Each Centrifugo process now reports telemetry under a distinct identity, which avoids backends that require points of a time series to arrive in order (notably Google Cloud Managed Service for Prometheus) rejecting or collapsing metrics when several instances report under the same identity. `OTEL_SERVICE_NAME` and `OTEL_RESOURCE_ATTRIBUTES` still take precedence over Centrifugo defaults.
* All official Centrifugo SDKs now support a `getState` subscription callback – read the stream position first, then load your initial state, and return the position so the SDK subscribes from exactly there and recovers on every reconnect. This closes the gap between loading state from your own database and subscribing, see [Using recovery in your app](https://centrifugal.dev/docs/server/history_and_recovery#using-recovery-in-your-app).
* All official Centrifugo SDKs now support [publication filtering by tags](https://centrifugal.dev/docs/server/publication_filtering) – clients can subscribe with a filter expression so only publications whose tags match are delivered, reducing bandwidth and client-side processing (previously available in `centrifuge-js` only).

### Documentation

* The [Client protocol](https://centrifugal.dev/docs/transports/client_protocol) chapter was reworked and now ships with diagrams explaining the frame anatomy, command/reply sequence, push delivery, ping-pong, batching, and the JSON/Protobuf formats.
* The [History and recovery](https://centrifugal.dev/docs/server/history_and_recovery) chapter was significantly expanded with diagrams covering stream vs cache recovery modes, the recovery decision flow, and recovery storm mitigation.
* The [GrandChat tutorial](https://centrifugal.dev/docs/tutorial/intro) was actualized for 2026 – the Django/React code was modernized to recent versions and made more idiomatic and type-safe, and a new [Two-column layout](https://centrifugal.dev/docs/tutorial/two_column) chapter shows how to build a Telegram/Slack-style messenger layout (room list and open room side by side) live from a single personal-channel subscription.

### Miscellaneous

* The `linux/386` (32-bit x86) binary is no longer published in releases, see [#1154](https://github.com/centrifugal/centrifugo/pull/1154).
* This release is built with Go 1.26.4
* Dependency updates
* See also the corresponding [Centrifugo PRO release](https://github.com/centrifugal/centrifugo-pro/releases/tag/v6.8.3).
