Centrifugo is an open-source scalable real-time messaging server. Centrifugo can instantly deliver messages to application online users connected over supported transports (WebSocket, HTTP-streaming, SSE/EventSource, GRPC, WebTransport). Centrifugo has the concept of a channel – so it's a user-facing PUB/SUB server.

Centrifugo is language-agnostic and can be used to build chat apps, live comments, multiplayer games, real-time data visualizations, collaborative tools, etc. in combination with any backend. It is well suited for modern architectures and allows decoupling the business logic from the real-time transport layer.

Several official client SDKs for browser and mobile development wrap the bidirectional protocol. In addition, Centrifugo supports a unidirectional approach for simple use cases with no SDK dependency.

For details, go to the [Centrifugo documentation site](https://centrifugal.dev).

## What's changed

### Improvements

* Publish Centrifugo Protobuf definitions to [Buf Schema Registry](https://buf.build/product/bsr) [#863](https://github.com/centrifugal/centrifugo/pull/863). This means that to use Centrifugo GRPC APIs it's now possible to depend on pre-generated Protobuf definitions instead of manually generating them from the schema file.
  * [apiproto](https://buf.build/centrifugo/apiproto/docs/main:centrifugal.centrifugo.api) - definitions of server GRPC API
  * [unistream](https://buf.build/centrifugo/unistream/docs/main:centrifugal.centrifugo.unistream) - definitions of unidirectional GRPC stream
  * [proxyproto](https://buf.build/centrifugo/proxyproto/docs/main:centrifugal.centrifugo.proxy) - definitions of proxy GRPC API
* New integer option `grpc_api_max_receive_message_size` (number of bytes). If set to a value > 0 allows setting `grpc.MaxRecvMsgSize` option for GRPC API server. The option controls the max size of message GRPC server can receive. By default, GRPC library uses 4194304 bytes (4MB).

### Fixes

* Fix occasional `panic: DedicatedClient should not be used after recycled` panic which could happen under load during problems with Redis connection.

### Miscellaneous

* Release is built with Go 1.22.5
* All dependencies were updated to latest versions
* Check out [Centrifugo v6 roadmap](https://github.com/centrifugal/centrifugo/issues/856) issue. It outlines some important changes planned for the next major release. The date of the v6 release is not yet specified. 
