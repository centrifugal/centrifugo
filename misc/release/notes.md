Centrifugo is an open-source scalable real-time messaging server. It instantly delivers messages to application online users connected over supported transports (WebSocket, HTTP-streaming, Server-Sent Events (SSE), GRPC, WebTransport). Centrifugo is built around channel subscriptions – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, AI streaming responses, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Official client SDKs are available for JavaScript (browser, Node.js, React Native), Dart/Flutter, Swift, Java, Python, Go, and .NET. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev). For runnable demos see [centrifugal/examples](https://github.com/centrifugal/examples).

## What's changed

### Improvements

* Ed25519 public key for EdDSA tokens. New option `client.token.ed25519_public_key` (and the same option in `client.subscription_token`) takes a PEM encoded Ed25519 public key to verify EdDSA signed connection and subscription tokens. Before, EdDSA tokens could be verified only with keys from a JWKS endpoint, while RSA and ECDSA keys could already be set directly. See [JWT authentication](https://centrifugal.dev/docs/server/authentication) ([#1288](https://github.com/centrifugal/centrifugo/pull/1288)).
* JWKS key rotation and outages no longer disconnect clients for good ([#1289](https://github.com/centrifugal/centrifugo/pull/1289)). See [JSON Web Key support](https://centrifugal.dev/docs/server/authentication#json-web-key-support):
  * a token whose key is not returned by the JWKS endpoint (for example, during a key rollout, or a token signed with a retired key) is now treated as an expired token (`109`, or `3005` on unidirectional transports), so the client gets a new token and reconnects. Before, it was an invalid token (`3500`), and client SDKs stopped reconnecting;
  * if the JWKS endpoint can't be reached, the client gets a temporary error and retries, instead of an invalid token;
  * keys loaded from the JWKS endpoint are now kept for 5 minutes instead of one hour, so a key removed from the endpoint (for example, after a leak) stops working within 5 minutes. Keys already loaded are still used while the endpoint can't be reached;
  * the same applies to subscription tokens. Tokens with a bad signature or malformed tokens are still invalid.
* `proxy.proto` now documents every message and field: what each request field carries, what each result field changes, and which responses honour `error` and `disconnect`. See also [proxy docs](https://centrifugal.dev/docs/server/proxy) ([#1287](https://github.com/centrifugal/centrifugo/pull/1287)).

### Fixes

* Node metrics interval in the `info` server API is the configured interval again (e.g. `60`), not a slightly different measured value like `59.999856375` ([centrifugal/centrifuge#671](https://github.com/centrifugal/centrifuge/pull/671)).

### Miscellaneous

* Docker image is rebuilt with the latest Alpine `ca-certificates-bundle`, which removes trust in the compromised eMudhra intermediate CA certificates. Centrifugo itself already rejected these certificates since v6.10.1 (Go 1.27.3), but other tools in the image which use the system trust store did not, and container scanners reported the image as vulnerable ([#1290](https://github.com/centrifugal/centrifugo/issues/1290)).
* This release is built with Go 1.27.3.
* Dependency updates.
* See also the corresponding [Centrifugo PRO release](https://github.com/centrifugal/centrifugo-pro/releases/tag/v6.10.2).
