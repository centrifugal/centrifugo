Centrifugo is an open-source scalable real-time messaging server in a language-agnostic way. It can be a missing piece in your application infrastructure for introducing real-time features. Think chats, live comments, multiplayer games, streaming metrics – you'll be able to build amazing web and mobile real-time apps with a help of Centrifugo. Choose the approach you like:

* bidirectional communication over WebSocket or SockJS
* or unidirectional communication over WebSocket, EventSource (Server-Sent Events), HTTP-streaming, GRPC
* or... combine both!

See [centrifugal.dev](https://centrifugal.dev/) for more information.

## Release notes

No backwards incompatible changes here.

### Improvements

* Centrifugo now periodically sends anonymous usage information (once in 24 hours). That information is impersonal and does not include sensitive data, passwords, IP addresses, hostnames, etc. Only counters to estimate version and installation size distribution, and feature usage. See implementation in [#516](https://github.com/centrifugal/centrifugo/pull/516). Please do not disable usage stats sending without reason. If you depend on Centrifugo – sure you are interested in further project improvements. Usage stats help us understand Centrifugo use cases better, concentrate on widely-used features, and be confident we are moving in the right direction. Developing in the dark is hard, and decisions may be non-optimal. See [docs](https://centrifugal.dev/docs/next/server/configuration#anonymous-usage-stats) for more details.

### Misc

* We continue working on Centrifugo v4, look at our [v4 roadmap](https://github.com/centrifugal/centrifugo/issues/500) where the latest updates are shared. BTW Centrifugo v3 already has code to work over new protocol which we aim to make default in v4. It's already possible to try out our own bidirectional emulation layer with HTTP-streaming and Eventsource transports. Don't hesitate reaching out if you depend on Centrifugo and want to understand more what's coming in next major release. We are actively collecting feedback at the moment.
* This release is built with Go 1.17.10.
