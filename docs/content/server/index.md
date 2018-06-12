# Server overview

Centrifugo server is written in Go language. It's an open-source software, the source code is available [on Github](https://github.com/centrifugal/centrifugo).

Centrifugo is built around [centrifuge](https://github.com/centrifugal/centrifuge) library for Go language. That library defines custom protocol and message types which must be sent over various transports (Websocket, SockJS). Server clients use that protocol internally and provide simple API to features - making persistent connection, subscribing on channels, calling RPC commands and more.

This documentation chapter covers some server concepts in detail. This is documentation for Centrifugo server but many things said here are also valid for centrifuge library as it's a core of Centrifugo server. 
