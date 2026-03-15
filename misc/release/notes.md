Centrifugo is an open-source scalable real-time messaging server. Centrifugo can instantly deliver messages to application online users connected over supported transports (WebSocket, HTTP-streaming, Server-Sent Events (SSE/EventSource), GRPC, WebTransport). Centrifugo has the concept of a channel – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Several official client SDKs for browser and mobile development wrap the bidirectional protocol. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev).

## What's changed

This release contains breaking change to address CVE discovered in [Dynamic JWKs endpoint](https://centrifugal.dev/docs/server/authentication#dynamic-jwks-endpoint) feature. If you use that feature you need to update Centrifugo configuration. See fixes section for the details.

### Improvements

* This release is the first built with Go 1.26. This version of the Go language includes a new garbage collector called the [Green Tea garbage collector](https://go.dev/doc/go1.26#new-garbage-collector). This may affect the performance of your Centrifugo installation; in most cases, we expect the impact to be positive. If you notice any performance changes in Centrifugo after upgrading to this release, please let us know in the community rooms. More information about the new GC can be found [here](https://go.dev/blog/greenteagc).
* Updated the Alpine image to 3.22 in the Dockerfile.
* Improve lint layout to improve local DX

### Fixes

* [CVE-2026-32301](https://github.com/centrifugal/centrifugo/security/advisories/CVE-2026-32301) Fixed SSRF vulnerability in [Dynamic JWKS endpoint](https://centrifugal.dev/docs/server/authentication#dynamic-jwks-endpoint) feature. When using JWKS endpoint URL templates with placeholders extracted from JWT claims via `issuer_regex` or `audience_regex`, an attacker could craft a JWT with malicious claim values to redirect JWKS key fetches to an attacker-controlled server, enabling token forgery. **Action required:** if you use dynamic JWKS endpoints, update your `issuer_regex`/`audience_regex` patterns so that named capture groups used in the JWKS URL template contain only an explicit list of allowed literal values (e.g., `(?P<tenant>tenant1|tenant2|tenant3)` instead of `(?P<tenant>.+)`). Centrifugo will now reject configurations where these groups allow arbitrary input. A temporary escape hatch `client.token.insecure_skip_jwks_endpoint_safety_check` option is available but will be removed in future releases.
* The Go version update (1.25.7 to 1.26.1) and update of Go x/net library allow inheriting fixes for several recently discovered CVE.

### Miscellaneous

* This release is built with Go 1.26.1
* See also the corresponding [Centrifugo PRO release](https://github.com/centrifugal/centrifugo-pro/releases/tag/v6.6.6).
