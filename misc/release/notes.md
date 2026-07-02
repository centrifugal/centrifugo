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

### Miscellaneous

* This release is built with Go 1.26.4
* Dependency updates
* See also the corresponding [Centrifugo PRO release](https://github.com/centrifugal/centrifugo-pro/releases/tag/v6.9.0).
