Centrifugo is an open-source scalable real-time messaging server. It instantly delivers messages to application online users connected over supported transports (WebSocket, HTTP-streaming, Server-Sent Events (SSE), GRPC, WebTransport). Centrifugo is built around channel subscriptions – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, AI streaming responses, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Official client SDKs are available for JavaScript (browser, Node.js, React Native), Dart/Flutter, Swift, Java, Python, Go, Rust, and .NET. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev). For runnable demos see [centrifugal/examples](https://github.com/centrifugal/examples).

## What's changed

### Improvements

* Graceful shutdown now covers everything Centrifugo runs ([#1302](https://github.com/centrifugal/centrifugo/pull/1302)). See [signal handling](https://centrifugal.dev/docs/server/configuration#signal-handling):
  * [async consumers](https://centrifugal.dev/docs/server/consumers) – Kafka, Redis Streams, NATS JetStream, PostgreSQL outbox, AWS SQS, Google Cloud PUB/SUB and Azure Service Bus – stop taking new messages, then finish and acknowledge the messages they already took. A message processed right before a stop is no longer delivered again after the restart;
  * connections to Redis, PostgreSQL, NATS, proxy endpoints and JWKS endpoints are closed, and buffered OpenTelemetry spans are exported;
  * the whole shutdown finishes within `shutdown.timeout`. If it doesn't, Centrifugo logs `shutdown timeout reached` and exits with code 1, as before;
  * when a server, an async consumer or another component fails after start, Centrifugo shuts down gracefully before exiting with code 1, instead of exiting at once;
  * startup errors such as a port in use or a bad TLS configuration are reported before Centrifugo starts processing messages from async consumers.
* New `--watch-config-file` flag restarts Centrifugo when the configuration in its config file changes – **for development**. All options apply, unlike with SIGHUP reload. A change which fails validation is logged and ignored, and Centrifugo keeps running. The restart happens in the same process, which keeps its PID. See [restart on config file change](https://centrifugal.dev/docs/server/configuration#restart-on-config-file-change) ([#502](https://github.com/centrifugal/centrifugo/issues/502), [#1302](https://github.com/centrifugal/centrifugo/pull/1302)).
* New channel options `recovery_max_publication_limit` and `history_max_publication_limit` set the maximum number of publications returned in one recovery and in one history request for a namespace. Before, only `client.recovery_max_publication_limit` and `client.history_max_publication_limit` existed, applying to all channels. A namespace whose history holds many small publications – for example, AI responses streamed in chunks – can now have a higher limit than the rest. Zero, the default, keeps using the `client.*` options. See [channel options](https://centrifugal.dev/docs/server/channels#recovery_max_publication_limit) and [Centrifugo for AI apps](https://centrifugal.dev/docs/getting-started/ai_apps#configuring-a-channel-for-a-stream) ([#1294](https://github.com/centrifugal/centrifugo/pull/1294), [centrifugal/centrifuge#674](https://github.com/centrifugal/centrifuge/pull/674)).
* Better comments in Protobuf definitions for server API and Proxy.

### Fixes

* SIGHUP reload pointing channel or RPC options to a proxy which was not created on start was reported as successful, and the next operation using that proxy panicked. Such a reload is now rejected with an error, and the previous configuration stays active – a restart is required to start using a new proxy. Note: enabling a proxy kind for which no proxy was created on start used to reload successfully and then return `108 not available` to clients, and is now rejected too. See [signal handling](https://centrifugal.dev/docs/server/configuration#signal-handling) ([#1300](https://github.com/centrifugal/centrifugo/pull/1300)).
* Redis Streams consumer: messages read from Redis right before shutdown could be left unprocessed, and processed only once `visibility_timeout` passed (30 seconds by default) ([#1307](https://github.com/centrifugal/centrifugo/pull/1307)).
* A server-side subscribe, refresh or message sent with the server API to a client which was still connecting could reach the client before its connect reply. They now come after it ([centrifugal/centrifuge#676](https://github.com/centrifugal/centrifuge/pull/676)).
* Other subscribers of a channel could get a leave event without a join event for a client whose server-side subscribe happened while its message queue was full ([centrifugal/centrifuge#673](https://github.com/centrifugal/centrifuge/pull/673)).

### Miscellaneous

* New [Server SDKs](https://centrifugal.dev/docs/server/server_sdks) page in the docs lists the official server SDKs – `gocent`, `pycent`, `jscent`, `phpcent`, `rubycent`, `rustcent` and `csharpcent` – with install lines, a first publication in each language, notes for AI coding agents, and when to use GRPC instead of the HTTP API. All server SDKs follow a [common SDK spec](https://github.com/centrifugal/gocent/blob/master/spec.md). 
* [centrifuge-rust](https://github.com/centrifugal/centrifuge-rust) is now listed among the official [client SDKs](https://centrifugal.dev/docs/transports/client_sdk), with Rust examples in the client API docs. The C# SDK is no longer marked as work in progress.
* [Centrifugo for AI apps](https://centrifugal.dev/docs/getting-started/ai_apps) now covers checkpoints for long responses, loading a conversation with `getState`, and stopping generation from every device. See the new [Vercel AI SDK chat example](https://github.com/centrifugal/examples/tree/master/v6/ai-sdk-chat).
* Web UI is updated to the latest dependencies ([#1305](https://github.com/centrifugal/centrifugo/pull/1305)).
* This release is built with Go 1.27.3.
* Dependency updates.
* See also the corresponding [Centrifugo PRO release](https://github.com/centrifugal/centrifugo-pro/releases/tag/v6.10.3).
