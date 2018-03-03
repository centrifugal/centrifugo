# Server overview

Centrifugo server is written in Go language. It's an open-source software, the source code is available [on Github](https://github.com/centrifugal/centrifugo).

Centrifugo is built around centrifuge library for Go language. That library defines custom protocol and message types which must be sent over various transports (Websocket, GRPC, SockJS). Our clients use that protocol internally and provide simple API to features - making persistent connection, subscribing on channels, calling RPC commands and more.
