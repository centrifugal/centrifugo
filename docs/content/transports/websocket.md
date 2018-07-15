# Websocket

[Websocket](https://en.wikipedia.org/wiki/WebSocket) is the main transport in Centrifugo. It's a very efficient low-overhead protocol on top of TCP.

The biggest advantage is that Websocket works out of the box in all modern browsers and almost all programming languages have Websocket implementations. This makes Websocket a pretty universal transport that can even be used to connect to Centrifugo from web apps and mobile apps and other environments.

Websocket connection endpoint in Centrifugo is `/connection/websocket`. If you want to use Protobuf binary protocol then you have to connect to `/connection/websocket?format=protobuf`
