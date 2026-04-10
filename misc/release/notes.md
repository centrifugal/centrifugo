Centrifugo is an open-source scalable real-time messaging server. Centrifugo can instantly deliver messages to application online users connected over supported transports (WebSocket, HTTP-streaming, Server-Sent Events (SSE/EventSource), GRPC, WebTransport). Centrifugo has the concept of a channel – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Several official client SDKs for browser and mobile development wrap the bidirectional protocol. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev).

## What's changed

This release contains breaking change to address CVE discovered in [Dynamic JWKs endpoint](https://centrifugal.dev/docs/server/authentication#dynamic-jwks-endpoint) feature. If you use that feature you need to update Centrifugo configuration. See fixes section for the details.

### Improvements

* Kafka consumer now supports AWS STS AssumeRole for MSK IAM authentication via the new `consumers[].kafka.assume_role_arn` option, [#1129](https://github.com/centrifugal/centrifugo/pull/1129) by @samir-is-here. When set together with `sasl_mechanism: "aws-msk-iam"`, Centrifugo loads base credentials via the AWS SDK default credential chain and assumes the specified IAM role to obtain temporary credentials with automatically refreshed session tokens. This is useful for cross-account MSK access or when running Centrifugo with an EC2/EKS/ECS instance profile. Static `sasl_user`/`sasl_password` keys remain the default when `assume_role_arn` is empty. See [documentation](https://centrifugal.dev/docs/server/consumers#consumerskafkaassume_role_arn).

### Fixes

* CI fix: set LocalStack image version to 4.14 in development setup, [#1119](https://github.com/centrifugal/centrifugo/pull/1119).

### Miscellaneous

* This release is built with Go 1.26.2
* Dependency updates
* See also the corresponding [Centrifugo PRO release](https://github.com/centrifugal/centrifugo-pro/releases/tag/v6.7.1).
