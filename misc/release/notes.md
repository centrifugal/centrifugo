Centrifugo is an open-source scalable real-time messaging server. It instantly delivers messages to application online users connected over supported transports (WebSocket, HTTP-streaming, Server-Sent Events (SSE), GRPC, WebTransport). Centrifugo is built around channel subscriptions – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, AI streaming responses, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Official client SDKs are available for JavaScript (browser, Node.js, React Native), Dart/Flutter, Swift, Java, Python, Go, and .NET. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev). For runnable demos see [centrifugal/examples](https://github.com/centrifugal/examples).

## What's changed

### Improvements

* OpenTelemetry: authenticate the OTLP exporter with Google Cloud Application Default Credentials (ADC) via the new `google_cloud_adc_auth` option, see [#1143](https://github.com/centrifugal/centrifugo/pull/1143) and [#1148](https://github.com/centrifugal/centrifugo/pull/1148). This allows exporting traces directly to Google Cloud's OTLP endpoint (`telemetry.googleapis.com`) without a sidecar collector, and works with both the `grpc` and `http/protobuf` exporter protocols.
* Kafka consumer: added a configurable `dial_timeout` (default `3s`) for establishing a TCP connection to a single broker, and made the initial `Ping` timeout scale with the number of seed brokers so discovery no longer fails prematurely when some brokers are unreachable, see [#1151](https://github.com/centrifugal/centrifugo/pull/1151).
* Centrifugo official Helm chart now supports k8s Gateway API - see [Helm chart 13.3.0 release](https://github.com/centrifugal/helm-charts/releases/tag/centrifugo-13.3.0)
* Centrifugo now does not embed generated JSON config schema - configuration structure for `configdoc` is calculated in runtime

### Fixes

* Kafka consumer with AWS MSK IAM auth: re-assume the STS role on each SASL re-auth instead of reusing cached credentials, fixing periodic `ILLEGAL_SASL_STATE` errors during re-authentication, see [#1146](https://github.com/centrifugal/centrifugo/pull/1146) by @samir-is-here which fixes [#1144](https://github.com/centrifugal/centrifugo/issues/1144).
* Fix unidirectional subscribe stream proxy not closing on unsubscribe, see [#1150](https://github.com/centrifugal/centrifugo/pull/1150).

### Miscellaneous

* This release is built with Go 1.26.4
* Dependency updates
* See also the corresponding [Centrifugo PRO release](https://github.com/centrifugal/centrifugo-pro/releases/tag/v6.8.2).
