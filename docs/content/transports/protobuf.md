# Protobuf binary protocol

In most cases you will use Centrifugo with JSON protocol which is used by default. It consists of simple human-readable frames that can be easily inspected. Also it's a very simple task to publish JSON encoded data to HTTP API endpoint. You may want to use binary Protobuf client protocol if:

* you want less traffic on wire as Protobuf is very compact
* you want maximum performance as Protobuf encoding/decoding is very efficient
* you can sacrifice human-readable JSON for your application

Binary protobuf protocol only works for raw Websocket connections (as SockJS can't deal with binary). With most clients to use binary you just need to provide query parameter `format` to Websocket URL, so final URL look like:

```
wss://centrifugo.example.com/connection/websocket?format=protobuf
```

After doing this Centrifugo will use binary frames to pass data between client and server. Your application specific payload can be random bytes.

!!! note
    You still can continue to encode your application specific data as JSON when using Protobuf protocol thus have a possibility to coexist with clients that use JSON protocol on the same Centrifugo installation inside the same channels.
