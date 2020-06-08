# Websocket

[Websocket](https://en.wikipedia.org/wiki/WebSocket) is the main transport in Centrifugo. It's a very efficient low-overhead protocol on top of TCP.

The biggest advantage is that Websocket works out of the box in all modern browsers and almost all programming languages have Websocket implementations. This makes Websocket a pretty universal transport that can even be used to connect to Centrifugo from web apps and mobile apps and other environments.

Websocket connection endpoint in Centrifugo is `/connection/websocket`. If you want to use Protobuf binary protocol then you need to connect to `/connection/websocket?format=protobuf`

### Websocket compression

An experimental feature for raw websocket endpoint - `permessage-deflate` compression for  websocket messages. Btw look at [great article](https://www.igvita.com/2013/11/27/configuring-and-optimizing-websocket-compression/) about websocket compression.

We consider this experimental because this websocket compression is experimental in [Gorilla Websocket](https://github.com/gorilla/websocket) library that Centrifugo uses internally.

Websocket compression can reduce an amount of traffic travelling over the wire. But keep in mind that **enabling websocket compression will result in much slower Centrifugo performance and more memory usage** – depending on your message rate this can be very noticeable.

To enable websocket compression for raw websocket endpoint set `websocket_compression: true` in configuration file. After this clients that support permessage-deflate will negotiate compression with server automatically. Note that enabling compression does not mean that every connection will use it - this depends on client support for this feature.

Another option is `websocket_compression_min_size`. Default 0. This is a minimal size of message in bytes for which we use `deflate` compression when writing it to client's connection. Default value `0` means that we will compress all messages when `websocket_compression` enabled and compression support negotiated with client.

It's also possible to control websocket compression level defined at [compress/flate](https://golang.org/pkg/compress/flate/#NewWriter) By default when compression with a client negotiated Centrifugo uses compression level 1 (BestSpeed). If you want to set custom compression level use `websocket_compression_level` configuration option.

If you have a few writes then `websocket_use_write_buffer_pool` (boolean, default `false`) option can reduce memory usage of Centrifugo a bit as there won't be separate write buffer binded to each WebSocket connection.
