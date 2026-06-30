Centrifugo is an open-source scalable real-time messaging server. It instantly delivers messages to application online users connected over supported transports (WebSocket, HTTP-streaming, Server-Sent Events (SSE), GRPC, WebTransport). Centrifugo is built around channel subscriptions – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, AI streaming responses, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Official client SDKs are available for JavaScript (browser, Node.js, React Native), Dart/Flutter, Swift, Java, Python, Go, and .NET. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev). For runnable demos see [centrifugal/examples](https://github.com/centrifugal/examples).

## What's changed

### Breaking changes

**TLDR**: Proxy `http_headers` is now transport-only; need to use explicit `client_emulated_headers` list for emulated headers

Before v6.9.0, the proxy `http_headers` option forwarded headers from **two different sources**:

- **transport-level headers** — set on the real connection request (by the client transport or a reverse proxy/gateway in front of Centrifugo);
- **client-supplied emulated headers** — sent by the client inside its connect frame via the [headers emulation](https://centrifugal.dev/docs/server/proxy#http-headers-emulation) feature.

In v6.9.0 these two sources are split into separate options:

| Option                    | Source                                                 | Trust                                                        |
|---------------------------|--------------------------------------------------------|--------------------------------------------------------------|
| `http_headers`            | the real connection request only                       | controllable by your edge (a proxy/gateway can set/strip it) |
| `client_emulated_headers` | the client's headers emulation map only                | always client-controlled — treat as untrusted input          |

`http_headers` no longer forwards emulated headers. To forward a header supplied via headers emulation, you must now list it in the new `client_emulated_headers` option.

**Why:** forwarding a client-supplied emulated value under the same list operators used for trusted transport headers made it possible to accidentally forward a forgeable value to a backend that trusted it (e.g. an identity header). For example, this could happen for bidirectional WebSocket if your proxy sets the `x-user-id` header for an authenticated user, but skips setting it for a non-authenticated one. If your proxy always sets `x-user-id` to an empty string in the non-authenticated case – there was no security issue.

To avoid possible mistakes on that level we decided to split the sources to make the boundary explicit: a name in `http_headers` can never be injected by the client's emulation map.

**Are you affected?** Only if you rely on headers emulation:

* using [headers](https://github.com/centrifugal/centrifuge-js/blob/77d9fdc1050da0eec0e385819dc380a80430edfe/src/types.ts#L127) option in `centrifuge-js`
* using [headers](https://github.com/centrifugal/centrifuge-dart/blob/f91c8f158f17885bfdbdd0e970069216e89d59e3/lib/src/client_config.dart#L47) option in `centrifuge-dart` (web platform only — on native `dart:io` these are sent as real WebSocket upgrade headers and are not affected)
* using [headers](https://github.com/centrifugal/centrifuge-csharp/blob/ce8a732deabfcac7cf6354447a4e092e1871ddf7/src/Centrifugal.Centrifuge/ClientOptions.cs#L86) option in `centrifuge-csharp`
* using unidirectional transports and passing `headers` as part of [ConnectRequest](https://centrifugal.dev/docs/transports/uni_client_protocol#connectrequest)

You are **not** affected if you are not using headers at all or only forward real transport headers (e.g. `Cookie`, `Authorization`, `X-Real-Ip` set by your reverse proxy or by native clients) — those keep working under `http_headers` with no change.

**How to migrate:** Before upgrading to v6.9.0, add the header names you allow to be sent over headers emulation to `client_emulated_headers` proxy configuration object option. Apply this change to every proxy that used emulated headers (`connect`, `refresh`, channel proxies, RPC proxies, and any named proxies in the top-level `proxies` list).

**Temporary compatibility flag:** if you can't migrate config immediately, set `http_headers_include_client_emulated: true` on a proxy to restore the pre-v6.9.0 behavior (header names in `http_headers` are again also sourced from emulation). This option is **deprecated**, logs a warning at startup, and **will be removed in Centrifugo v7** — use it only as a temporary workaround.

```json
{
  "client": {
    "proxy": {
      "connect": {
        "enabled": true,
        "endpoint": "https://your_backend/centrifugo/connect",
        "http_headers": ["Cookie", "Authorization"],
        "http_headers_include_client_emulated": true
      }
    }
  }
}
```

**Note:** Never put an origin-trusted identity header (e.g. a gateway-injected `x-user-id`) in `client_emulated_headers`, since that can make it forgeable if the transport-level header is not set (i.e. the proxy skips setting the `x-user-id` header for an unauthenticated user).

Long-term, we will also rename `headers` to `emulated_headers` in SDKs that provide this feature.

Please reach out in the community rooms for the help with the migration.

### Miscellaneous

* See also the corresponding [Centrifugo PRO release](https://github.com/centrifugal/centrifugo-pro/releases).
