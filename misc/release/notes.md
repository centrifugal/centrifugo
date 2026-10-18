Centrifugo is an open-source scalable real-time messaging server. It instantly delivers messages to application online users connected over supported transports (WebSocket, HTTP-streaming, Server-Sent Events (SSE), GRPC, WebTransport). Centrifugo is built around channel subscriptions – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, AI streaming responses, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Official client SDKs are available for JavaScript (browser, Node.js, React Native), Dart/Flutter, Swift, Java, Python, Go, and .NET. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev). For runnable demos see [centrifugal/examples](https://github.com/centrifugal/examples).

## What's changed

### Improvements

* Stricter JSON parsing in the client protocol. Centrifugo now follows RFC 8259 exactly. It rejects invalid UTF-8, repeated keys, extra data after a message, keys with different letter case and too large integers. Before, such messages were partly accepted. Now Centrifugo and your backend always read a message the same way, and invalid UTF-8 can't reach browsers (a browser closes the WebSocket when it gets invalid UTF-8). Official SDKs are not affected. See two edge cases in the behaviour changes below ([centrifugal/protocol#56](https://github.com/centrifugal/protocol/pull/56), [centrifugal/centrifuge#652](https://github.com/centrifugal/centrifuge/pull/652), [#1259](https://github.com/centrifugal/centrifugo/pull/1259)).
* Centrifugo now uses its own JSON code generator, `cfjson`, instead of `easyjson` (and `segmentio/encoding` for decoding), and its own Protobuf generator, `cfprotobuf`, instead of `vtprotobuf`. JSON output is the same byte for byte. In protocol benchmarks on Linux amd64, JSON encoding takes 46% less time and JSON decoding 25% less time ([centrifugal/protocol#56](https://github.com/centrifugal/protocol/pull/56)).
* Server API errors show if a retry makes sense. Errors have a new `temporary` field (similar to the client protocol Error field): it is `true` when the same request may succeed later. There is a new error `111 too many requests` (HTTP 429, gRPC `ResourceExhausted`). Requests which can never succeed now return 107 (bad request), 102 (unknown channel) or 108 (not available) instead of `100 internal`, so clients don't retry them ([#1247](https://github.com/centrifugal/centrifugo/pull/1247)).
* Faster memory broker history. History is split into 64 parts by channel, each with its own lock. Before, all channels used one lock. Publishing with history into 10000 channels from all CPUs now takes 1.61µs per publication instead of 2.90µs (−44%) ([centrifugal/centrifuge#666](https://github.com/centrifugal/centrifuge/pull/666)).
* Faster broadcast to subscriptions with positioning: Centrifugo reads the current time once per broadcast, not once per subscriber. 10–15% faster for 100 and 1000 subscribers ([centrifugal/centrifuge#665](https://github.com/centrifugal/centrifuge/pull/665)).
* Unidirectional SSE no longer copies every message when writing it to a connection. With a 1 KB payload, memory allocated per delivered message dropped from about 1,220 bytes to 58 bytes, and with a 10 KB payload from about 10,650 bytes to 330 bytes. Garbage collection almost stops in these tests, and CPU usage is a few percent lower. Unidirectional HTTP streaming also allocates a bit less per message. Thanks @darakanoit ([#1251](https://github.com/centrifugal/centrifugo/pull/1251)).
* Broadcast with `publication_data_format` `json` or `json_object` checks the data once per broadcast, not once per channel. A broadcast into 1000 channels with a 10 KB payload uses 60% less CPU (7.35 ms → 2.94 ms) and is 33% faster; with a 1 KB payload it uses 17% less CPU. Thanks @darakanoit ([#1252](https://github.com/centrifugal/centrifugo/pull/1252)).
* Faster JWKS token verification under load. A JWKS cache hit no longer takes a lock or allocates memory. Parallel connect token verification is 34% faster with RSA-2048 keys (4.76 µs → 3.13 µs) and 15% faster with ES256 keys. Thanks @darakanoit ([#1253](https://github.com/centrifugal/centrifugo/pull/1253)).
* PostgreSQL stream and map brokers no longer allocate memory on an empty outbox poll. On an idle server, the stream broker allocated 25.8 MiB per second and the map broker 11.6 MiB per second; now both allocate about 130 KiB per second, with no garbage collection cycles. Thanks @darakanoit ([#1250](https://github.com/centrifugal/centrifugo/pull/1250)).
* Config reload on SIGHUP improvements. Before, if a subscription token setting failed to apply, the connection token settings were already changed, and Centrifugo worked with a mix of old and new settings ([#1267](https://github.com/centrifugal/centrifugo/pull/1267)).
* JWKS keys are updated after key rotation. Centrifugo loads keys from the JWKS endpoint again one hour after the last load. A key removed from the endpoint stops working within one hour. Before, a key which was used all the time was never loaded again. Also, tokens with unknown `kid` values now share one request to the JWKS endpoint, so fake tokens can't overload it ([#1255](https://github.com/centrifugal/centrifugo/pull/1255), [#1264](https://github.com/centrifugal/centrifugo/pull/1264)).

### Behaviour changes

Most setups need no changes. Please check this list before upgrading:

* Publication data for JSON subscribers must be valid UTF-8 JSON. If you publish invalid UTF-8, or binary data (gRPC `data` or HTTP `b64data`) into a channel with JSON subscribers, they are disconnected with code `3506` ([centrifugal/centrifuge#652](https://github.com/centrifugal/centrifuge/pull/652)).
* Async consumers retry only temporary errors (100 and 111). For other errors, Centrifugo logs the error and marks the message as processed ([#1247](https://github.com/centrifugal/centrifugo/pull/1247)).
* Client publish into a namespace with a non-stream subscription type (`map`, `map_clients`, `map_users`, `shared_poll`) is now refused with `103 permission denied` ([#1280](https://github.com/centrifugal/centrifugo/pull/1280)).
* Join/leave override in a subscription token now works. If the token sets `join_leave: true` and the namespace has `join_leave` off, the client gets join/leave messages when it asks for them and has presence permission. Before, Centrifugo checked only the namespace option ([#1277](https://github.com/centrifugal/centrifugo/pull/1277)).
* Nats raw mode: if a channel has `publication_data_format` set to `json` or `json_object`, messages which don't match it are not delivered ([#1258](https://github.com/centrifugal/centrifugo/pull/1258)).
* A connection token with invalid `b64info` is now an invalid token. The client is disconnected. Before, it reconnected again and again with the same token. An invalid `pattern` in the `channels` server API call now returns 107 (bad request) instead of 100 (internal error) ([#1267](https://github.com/centrifugal/centrifugo/pull/1267)).

### Fixes

* Join/leave, positioning and recovery requested by a client now work for subscriptions allowed by the subscribe proxy. Join/leave also works for subscribe stream proxy subscriptions. Permission checks are the same as for other subscriptions. Thanks @AdzerKI ([#1270](https://github.com/centrifugal/centrifugo/pull/1270), [#1277](https://github.com/centrifugal/centrifugo/pull/1277)).
* A connection without user ID could subscribe to a user-limited channel with an empty user in it, such as `user#` or `user#42,`. Now it can't. Channels requested on connect are no longer subscribed in a namespace with subscribe stream proxy or with a non-stream subscription type ([#1279](https://github.com/centrifugal/centrifugo/pull/1279), [#1280](https://github.com/centrifugal/centrifugo/pull/1280)).
* Subscribe stream proxy fixes ([#1262](https://github.com/centrifugal/centrifugo/pull/1262), [#1272](https://github.com/centrifugal/centrifugo/pull/1272)):
  * closing the stream of an old subscription no longer unsubscribes a new one;
  * a refused stream subscription no longer keeps a goroutine until disconnect;
  * a first message which comes at the same moment as the timeout no longer leaves the subscription on a closed stream.
* With `client.concurrency`, disconnect no longer waits for a timeout if the client goes away while its subscribe waits in the queue ([#1268](https://github.com/centrifugal/centrifugo/pull/1268)).
* A `null` item in a shared poll refresh proxy response, or `null` channel options in connect proxy `subs`, no longer crash Centrifugo ([#1265](https://github.com/centrifugal/centrifugo/pull/1265), [centrifugal/centrifuge#647](https://github.com/centrifugal/centrifuge/pull/647)).
* `history_meta_ttl` now works for subscriptions with a subscription token, or allowed by subscribe or subscribe stream proxy ([#1260](https://github.com/centrifugal/centrifugo/pull/1260)).
* Redis: after a PUB/SUB reconnect, each broker resubscribes only to its own channels. Before, two Redis brokers with the same prefix on one Redis delivered every publication twice. Also, when a subscribe fails because of a broker error, the channel no longer stays subscribed in Redis ([centrifugal/centrifuge#660](https://github.com/centrifugal/centrifuge/pull/660)).
* A client no longer gets a publication without offset for a channel right after its subscribe failed, or an unsubscribe push for a subscribe which never got its result ([centrifugal/centrifuge#664](https://github.com/centrifugal/centrifuge/pull/664), [centrifugal/centrifuge#667](https://github.com/centrifugal/centrifuge/pull/667)).
* Map subscriptions (experimental):
  * namespace map options now work for subscriptions with a token or allowed by a proxy ([#1260](https://github.com/centrifugal/centrifugo/pull/1260), [#1271](https://github.com/centrifugal/centrifugo/pull/1271));
  * Redis map broker: a removed key is published with its tags, so subscribers with a tags filter now get the removal and don't keep old keys ([centrifugal/centrifuge#654](https://github.com/centrifugal/centrifuge/pull/654));
  * Redis map broker: the cleanup worker no longer runs in an endless loop which used CPU on Centrifugo and Redis. This happened after a key was refreshed or removed early. Affected channels are fixed on the first cleanup after upgrade ([centrifugal/centrifuge#663](https://github.com/centrifugal/centrifuge/pull/663));
  * PostgreSQL map broker: keys now expire also in channels whose namespace was removed. Such channels no longer stop keys in other channels from expiring ([#1276](https://github.com/centrifugal/centrifugo/pull/1276)).

### Miscellaneous

* This release is built with Go 1.27.2.
* Signed releases and SBOMs. Release artifacts are signed with Sigstore cosign keyless signing in the GitHub Actions release workflow. The checksums file covers release archives, DEB and RPM packages and SBOMs, its signature is published next to it as `centrifugo_<version>_checksums.txt.sigstore.json`. Docker images are signed too. Every release archive comes with an SPDX SBOM (`<archive>.sbom.json`). See [how to verify release artifacts](https://centrifugal.dev/docs/getting-started/installation#verifying-release-artifacts).
* Protobuf definitions in Buf Schema Registry are now also labeled with the release version, e.g. `buf.build/centrifugo/apiproto:v6.10.0`.
* Dependency updates.
* See also the corresponding [Centrifugo PRO release](https://github.com/centrifugal/centrifugo-pro/releases/tag/v6.10.0).
