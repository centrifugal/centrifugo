Centrifugo is an open-source scalable real-time messaging server in a language-agnostic way. It can be a missing piece in your application infrastructure for introducing real-time features. Think chats, live comments, multiplayer games, streaming metrics – you'll be able to build amazing web and mobile real-time apps with a help of Centrifugo. Choose the approach you like:

* bidirectional communication over WebSocket or SockJS
* or unidirectional communication over WebSocket, EventSource (Server-Sent Events), HTTP-streaming, GRPC
* or... combine both!

See [centrifugal.dev](https://centrifugal.dev/) for more information.

## Release notes

No backwards incompatible changes here.

### Improvements

* Centrifugo now periodically sends anonymous usage statistics (once in 24 hours). The information Centrifugo sends is absolutely impersonal, does not include any sensitive information, ip addresses, hostnames, etc. Only counters to estimate installation size distribution and feature use. See implementation in [#516](https://github.com/centrifugal/centrifugo/pull/516). We asking you to not disable stats sending without a proper reason – if you depend on Centrifugo then sure you are interested in further project improvements. Usage stats will help us understand Centrifugo use cases better, improve features which are widely used. See [docs](https://centrifugal.dev/docs/next/server/configuration#anonymous-usage-stats) for more details.

### Misc

* This release is built with Go 1.17.10.
