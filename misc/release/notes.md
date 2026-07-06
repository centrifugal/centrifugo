Centrifugo is an open-source scalable real-time messaging server. It instantly delivers messages to application online users connected over supported transports (WebSocket, HTTP-streaming, Server-Sent Events (SSE), GRPC, WebTransport). Centrifugo is built around channel subscriptions – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, AI streaming responses, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Official client SDKs are available for JavaScript (browser, Node.js, React Native), Dart/Flutter, Swift, Java, Python, Go, and .NET. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev). For runnable demos see [centrifugal/examples](https://github.com/centrifugal/examples).

## What's changed

### Breaking changes

**TLDR**: Proxy `http_headers` is now transport-only; Need to configure explicit `emulated_headers` list for emulated headers in Centrifugo server configuration.

Before v6.9.0, the proxy `http_headers` option forwarded headers from **two different sources**:

- **transport-level headers** — set on the real connection request (by the client transport or a reverse proxy/gateway in front of Centrifugo);
- **client-supplied emulated headers** — sent by the client inside its connect frame via the [headers emulation](https://centrifugal.dev/docs/server/proxy#http-headers-emulation) feature.

In v6.9.0 these two sources are split into separate options:

| Option             | Source                                  | Trust                                                        |
|--------------------|-----------------------------------------|--------------------------------------------------------------|
| `http_headers`     | the real connection request only        | controllable by your edge (a proxy/gateway can set/strip it) |
| `emulated_headers` | the client's headers emulation map only | always client-controlled — always an untrusted input         |

`http_headers` no longer forwards emulated headers. To forward a header supplied via headers emulation, you must now list it in the new `emulated_headers` option near `http_headers`.

**Why we had to make this a breaking change.** Before v6.9.0 a name listed in `http_headers` could be filled from **either** the real connection request **or** the client's emulation map. That made a trusted header like `x-user-id` forgeable in a way that is **not the operator's mistake**:

* An operator adds `x-user-id` to `http_headers` for a legitimate reason — their reverse proxy sets it, and they want that identity forwarded to the backend. They have no reason to expect this also lets a client supply `x-user-id` via emulation, especially if they have never heard of the headers emulation feature.
* That reverse proxy **might** set `x-user-id` only for authenticated requests and leave it fully unset otherwise.
* A client then sends `x-user-id` in its connect frame via emulation. For any request where the proxy did not set the real header, Centrifugo forwarded the client's **forged** value to the backend, which trusted it as an authenticated identity.

A setup where the proxy **always** sets `x-user-id` (for example to an empty string even for anonymous requests) is *not* vulnerable. But Centrifugo cannot inspect your proxy to know whether you did that, and cannot tell a genuine `x-user-id` from a client-emulated one at request time — so there is no way for Centrifugo to make this safe automatically or to warn only the affected setups.

The only robust fix is structural to make operators aware of the feature: split the sources so `http_headers` is **incapable** of carrying a client-emulated value. A name in `http_headers` is now sourced only from the real HTTP connection; anything a client sends via emulation must be explicitly opted into `emulated_headers` (and is clearly documented as untrusted input). This removes the possibility of the flaw entirely, rather than relying on every operator to configure their proxy defensively against a feature they may not even know exists.

**Are you affected?** Only if you rely on headers emulation:

* using [headers](https://github.com/centrifugal/centrifuge-js/blob/77d9fdc1050da0eec0e385819dc380a80430edfe/src/types.ts#L127) option in `centrifuge-js`
* using [headers](https://github.com/centrifugal/centrifuge-dart/blob/f91c8f158f17885bfdbdd0e970069216e89d59e3/lib/src/client_config.dart#L47) option in `centrifuge-dart` (web platform only — on native `dart:io` these are sent as real WebSocket upgrade headers and are not affected)
* using [headers](https://github.com/centrifugal/centrifuge-csharp/blob/ce8a732deabfcac7cf6354447a4e092e1871ddf7/src/Centrifugal.Centrifuge/ClientOptions.cs#L86) option in `centrifuge-csharp`
* using unidirectional transports and passing `headers` as part of [ConnectRequest](https://centrifugal.dev/docs/transports/uni_client_protocol#connectrequest)

You are **not** affected if you are not using headers at all or only forward real transport headers (e.g. `Cookie`, `Authorization`, `X-Real-Ip` set by your reverse proxy or by native clients) — those keep working under `http_headers` with no change.

**How to migrate:** Before upgrading to v6.9.0, add the header names you allow to be sent over headers emulation to `emulated_headers` proxy configuration object option near `http_headers`. Apply this change to every proxy that used emulated headers (`connect`, `refresh`, channel proxies, RPC proxies, and any named proxies in the top-level `proxies` list).

```json
{
  "client": {
    "proxy": {
      "connect": {
        "enabled": true,
        "endpoint": "https://your_backend/centrifugo/connect",
        "http_headers": ["X-User-Id", "Authorization"],
        "emulated_headers": ["Authorization"]
      }
    }
  }
}
```

**If you can't decide which headers to move right away, but need to update to v6.9.0** – then as a workaround, copy the whole `http_headers` list into `emulated_headers` nearby to preserve the pre-v6.9.0 behavior, then trim `emulated_headers` down to only the names your backend treats as untrusted client input.

Long-term, we will also rename `headers` to `emulated_headers` in SDKs that provide this feature.

Please reach out in the community rooms if the help with the migration is needed.

The security issue was reported in [GHSA-9468-v6mj-fppw](https://github.com/centrifugal/centrifugo/security/advisories/GHSA-9468-v6mj-fppw).

### Improvements

* NATS JetStream consumer: recreate the consumer on `ErrConsumerDeleted`, `ErrConsumerNotFound` and `ErrStreamNotFound` instead of getting permanently stuck ([#1166](https://github.com/centrifugal/centrifugo/pull/1166), commit [`920d0c23`](https://github.com/centrifugal/centrifugo/commit/920d0c23)). By @thuy-le-kafi.
* Reworked web admin UI — now Vite-based, uses CodeMirror instead of Monaco (for offline work) and includes various UX improvements ([#1169](https://github.com/centrifugal/centrifugo/pull/1169), commit [`fa984f0f`](https://github.com/centrifugal/centrifugo/commit/fa984f0f)).
* NATS JetStream consumer: back off redelivery with capped exponential delay instead of an instant `Nak`, avoiding hot redelivery loops on a poison message ([#1175](https://github.com/centrifugal/centrifugo/pull/1175), commit [`ddda397e`](https://github.com/centrifugal/centrifugo/commit/ddda397e)).
* Google Pub/Sub consumer: re-subscribe with capped backoff on a receive error instead of exiting the receive loop (which previously left the consumer silently stopped) ([#1176](https://github.com/centrifugal/centrifugo/pull/1176), commit [`5f6b3871`](https://github.com/centrifugal/centrifugo/commit/5f6b3871)).
* NATS JetStream consumer: reset the consume backoff only after a session has run healthy long enough, preventing rapid reconnect flapping ([#1176](https://github.com/centrifugal/centrifugo/pull/1176), commit [`c591b954`](https://github.com/centrifugal/centrifugo/commit/c591b954)).

### Fixes

A lot of issues were discovered and then fixed by LLM:

* Authentication: hold the read lock across the whole token verification to fix a data race between token verification and JWKS/config reload ([#1168](https://github.com/centrifugal/centrifugo/pull/1168), commit [`e30aca89`](https://github.com/centrifugal/centrifugo/commit/e30aca89)).
* PostgreSQL broker: fix `hashtext(channel)` integer overflow that could panic shard routing when the hash landed exactly on `INT_MIN` ([#1170](https://github.com/centrifugal/centrifugo/pull/1170), commit [`b52a3cb4`](https://github.com/centrifugal/centrifugo/commit/b52a3cb4)).
* PostgreSQL broker: self-heal a partial fresh install where only one of the JSONB/binary variant tables was created, which previously wedged schema setup ([#1171](https://github.com/centrifugal/centrifugo/pull/1171), commit [`cd85c450`](https://github.com/centrifugal/centrifugo/commit/cd85c450)).
* PostgreSQL broker: fix publication timestamps being zeroed by incorrect `timestamptz` text parsing ([#1174](https://github.com/centrifugal/centrifugo/pull/1174), commit [`3e163854`](https://github.com/centrifugal/centrifugo/commit/3e163854)).
* PostgreSQL stream broker: fix reverse history pagination returning already-seen or empty pages and never reaching older messages ([#1177](https://github.com/centrifugal/centrifugo/pull/1177), commit [`2a58bf7f`](https://github.com/centrifugal/centrifugo/commit/2a58bf7f)).
* PostgreSQL outbox: ping the advisory-lock connection while the worker is busy (not only when idle) so a silently dropped lock session is detected promptly, preventing dual-leader processing ([#1172](https://github.com/centrifugal/centrifugo/pull/1172), commit [`aee4cb6c`](https://github.com/centrifugal/centrifugo/commit/aee4cb6c)).
* NATS: strip the internal epoch tag before delivering publications so it no longer leaks to clients ([#1174](https://github.com/centrifugal/centrifugo/pull/1174), commit [`05fb837c`](https://github.com/centrifugal/centrifugo/commit/05fb837c)).
* Azure Service Bus: abandon (instead of complete) a message when processing fails so it is redelivered rather than silently dropped ([#1173](https://github.com/centrifugal/centrifugo/pull/1173), commit [`0b59fc3a`](https://github.com/centrifugal/centrifugo/commit/0b59fc3a)).
* Azure Service Bus: guard a boolean application-property type assertion against a panic on non-string values ([#1172](https://github.com/centrifugal/centrifugo/pull/1172), commit [`4cce70ff`](https://github.com/centrifugal/centrifugo/commit/4cce70ff)).
* Unidirectional WebSocket: clear the HTTP/2 write deadline after a frame ping — a leftover expired deadline could otherwise fail the stream permanently on HTTP/2 ([#1175](https://github.com/centrifugal/centrifugo/pull/1175), commit [`519c27a4`](https://github.com/centrifugal/centrifugo/commit/519c27a4)).
* Bidirectional WebSocket: clear the HTTP/2 write deadline after a frame ping when native frame ping-pong is used. Similar fix to the above's for unidirectional WebSocket. See [centrifugal/centrifuge#588](https://github.com/centrifugal/centrifuge/pull/588).
* WebTransport: coerce a `message_size_limit` of `0` to the default, preventing unbounded per-message memory use (OOM) ([#1179](https://github.com/centrifugal/centrifugo/pull/1179), commit [`45751b29`](https://github.com/centrifugal/centrifugo/commit/45751b29)).
* Unidirectional gRPC: use the uni-gRPC TLS config for the uni-gRPC server (the API-gRPC TLS config was used before) ([#1180](https://github.com/centrifugal/centrifugo/pull/1180), commit [`2fffa613`](https://github.com/centrifugal/centrifugo/commit/2fffa613)).
* Unidirectional gRPC: gracefully shut down the uni-gRPC server on exit — it was assigned to the wrong variable and never stopped ([#1180](https://github.com/centrifugal/centrifugo/pull/1180), commit [`2169cdcc`](https://github.com/centrifugal/centrifugo/commit/2169cdcc)).
* Redis: use the Sentinel TLS config for the Sentinel connection instead of the main Redis TLS config ([#1179](https://github.com/centrifugal/centrifugo/pull/1179), commit [`a043b736`](https://github.com/centrifugal/centrifugo/commit/a043b736)).
* Fix crash on a null connect request received over unidirectional transports ([#1173](https://github.com/centrifugal/centrifugo/pull/1173), commit [`e5140842`](https://github.com/centrifugal/centrifugo/commit/e5140842)).
* Fix crash on a nil command inside a batch server API request ([#1173](https://github.com/centrifugal/centrifugo/pull/1173), commit [`59f0ed16`](https://github.com/centrifugal/centrifugo/commit/59f0ed16)).
* Server API: decode `b64data`/`b64info` in the `subscribe` method (they were ignored) ([#1177](https://github.com/centrifugal/centrifugo/pull/1177), commit [`e1b3ecdd`](https://github.com/centrifugal/centrifugo/commit/e1b3ecdd)).
* Server API: translate the unrecoverable-position error in the map read methods so clients receive the proper error ([#1178](https://github.com/centrifugal/centrifugo/pull/1178), commit [`3278277f`](https://github.com/centrifugal/centrifugo/commit/3278277f)).
* Proxy: cancel the subscribe-stream proxy on all post-open error paths, fixing a context/goroutine leak ([#1179](https://github.com/centrifugal/centrifugo/pull/1179), commit [`8eb4fe9c`](https://github.com/centrifugal/centrifugo/commit/8eb4fe9c)).
* Enforce `connection_rate_limit` even when `connection_limit` is not set ([#1174](https://github.com/centrifugal/centrifugo/pull/1174), commit [`a914f5cf`](https://github.com/centrifugal/centrifugo/commit/a914f5cf)).
* Stop the Redis queue from installing a process-global signal handler ([#1174](https://github.com/centrifugal/centrifugo/pull/1174), commit [`658bc52e`](https://github.com/centrifugal/centrifugo/commit/658bc52e)).
* Metrics: label per-channel Broadcast publish failures as `broadcast_publish` instead of reusing the `publish` label ([#1172](https://github.com/centrifugal/centrifugo/pull/1172), commit [`0188e07f`](https://github.com/centrifugal/centrifugo/commit/0188e07f)).
* Metrics: bound HTTP metric label cardinality by the matched route pattern instead of the raw request path ([#1179](https://github.com/centrifugal/centrifugo/pull/1179), commit [`475a8669`](https://github.com/centrifugal/centrifugo/commit/475a8669)).
* Config validation: fix transform error messages and the `h2c_external` check ([#1178](https://github.com/centrifugal/centrifugo/pull/1178), commit [`6d057175`](https://github.com/centrifugal/centrifugo/commit/6d057175)).
* Logging: redact passwords containing special characters in logged URLs ([#1180](https://github.com/centrifugal/centrifugo/pull/1180), commit [`84c32661`](https://github.com/centrifugal/centrifugo/commit/84c32661)).

### Miscellaneous

* Removed the undocumented and unused dev page ([#1181](https://github.com/centrifugal/centrifugo/pull/1181), commit [`d29c3893`](https://github.com/centrifugal/centrifugo/commit/d29c3893)).
* This release is built with Go 1.26.4
* Dependency updates
* See also the corresponding [Centrifugo PRO release](https://github.com/centrifugal/centrifugo-pro/releases/tag/v6.9.0).
