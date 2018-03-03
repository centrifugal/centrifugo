# Client protocol

This chapter describes internal client-server protocol in details to help developers build custom client libraries.

### What client can do.

When you are using Centrifuge/Centrifugo client you expect some core things from it:

* connect to server and authenticate. Depending on transport endpoint address can differ. For example Centrifugo JSON-encoded Websocket endpoint is `ws://centrifugo.example.com/connection/websocket`.
* subscribe on channels developer wants. This allows to recieve messages published into channels in real-time.
* have a possibility to make RPC calls, publish, asking for presence etc.
* refresh client connection credentials when connection session lifetime is going to expire.
* handle ping/pong messaging with server under the hood to maintain connection alive and detect broken connection.
* handle protocol-specific errors, reconnect and recover missed messages automatically.

At moment Centrifuge/Centrifugo can work with several transports:

* Websocket
* SockJS
* GRPC

 This document describes protocol specifics for Websocket transport which supports binary and text formats to transfer data. As Centrifuge has various types of messages it serializes protocol messages using JSON or Protobuf (in case of binary websockets).

!!! note
    SockJS works almost the same way as JSON websocket described here but has its own extra framing on top of Centrifuge protocol messages. SockJS can only work with JSON - it's not possible to transfer binary data over it. SockJS is only needed as fallback to Websocket in web browsers.

!!! note
    GRPC support is an experimental at moment. GRPC works similar to what described here but it has its own transport details - Centrifuge library can not control how data travel over network and just uses GRPC generated API to pass messages between server and client over bidirectional streaming.

### Top level framing

Centrifuge protocol defined in [Protobuf schema](https://github.com/centrifugal/centrifuge/blob/master/misc/proto/client.proto).

Client sends `Command` to server.

Server sends `Reply` to client.

One request from client to server and one response from server to client can have more than one `Command` or `Reply`.

When JSON format is used then many `Command` can be sent from client to server in JSON streaming line-delimited format. I.e. many commands delimited by new line symbol `\n`.

```
{"id": 1, "method": 2, "params": {"channel": "ch1"}}
{"id": 2, "method": 2, "params": {"channel": "ch2"}}
```

!!! note
    This doc will use JSON format for examples because it's human-readable. Everything said here for JSON is also true for Protobuf encoded case.

When Protobuf format is used then many `Command` can be sent from client to server in length-delimited format where each individual `Command` marshaled to bytes prepended by `varint` length.

The same relates to many `Reply` in one response from server to client. Line-delimited JSON and varint-length prefixed Protobuf.

As you see above each `Command` has `id` field. This is an incremental integer field. This field will be echoed in server to client replies to commands so client could match a certain `Reply` to `Command` sent before. This is important because Websocket is asynchronous protocol where server and client both send messages in full-duplex mode.

So you can expect something like this when sending commands to server:

```
{"id": 1, "result": {}}
{"id": 2, "result": {}}
```

Besides `id` `Reply` from server to client have two important fields: `result` and `error`.

`result` contains useful payload object which can be different depending on `Reply`.

`error` contains error object in case of `Command` processing resulted in some error on server. `error` is optional and if `Reply` does not have `error` then it means that `Command` processed successfuly and client can parse `result` object in an appropriate way.

`error` objects looks like this:

```
{
    "code": 100,
    "message": "internal server error",
    "retry": true
}
```

We will talk more about error handling below.

The special type of `Reply` is asynchronous `Reply`. Those replies have no `id` field set (or `id` can be equal to zero). Async replies can come to client in any moment - not as reaction to issued `Command` but as message from server to client in arbitrary time. For example this can be message published into channel.

Centrifuge library defines several command types client can issue. And well-written client must be aware of all those commands and client workflow. Communication with Centrifuge/Centrifugo server starts with `connect` command.

### Connect

TODO...

### Subscribe

TODO...

### Unsubscribe

TODO...

### Refresh

TODO...

### RPC-like calls: publish, history, presence, rpc

TODO...

### Asynchronous server-to-client messages

TODO...

### Handle disconnects

TODO...

### Handle errors

TODO...
