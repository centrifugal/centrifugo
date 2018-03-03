# Client protocol

This chapter describes internal client-server protocol in details to help developers build custom client libraries.

Note that you can always look at existing client implementations in case of any questions, for example [centrifuge-js](https://github.com/centrifugal/centrifuge-js/blob/master/src/centrifuge.js).

### What client should do

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

Centrifuge protocol defined in [Protobuf schema](https://github.com/centrifugal/centrifuge/blob/master/misc/proto/client.proto). That schema is a source of truth and all protocol description below describes messages from that schema.

Client sends `Command` to server.

Server sends `Reply` to client.

One request from client to server and one response from server to client can have more than one `Command` or `Reply`.

When JSON format is used then many `Command` can be sent from client to server in JSON streaming line-delimited format. I.e. many commands delimited by new line symbol `\n`.

```javascript
{"id": 1, "method": "subscribe", "params": {"channel": "ch1"}}
{"id": 2, "method": "subscribe", "params": {"channel": "ch2"}}
```

!!! note
    This doc will use JSON format for examples because it's human-readable. Everything said here for JSON is also true for Protobuf encoded case.

When Protobuf format is used then many `Command` can be sent from client to server in length-delimited format where each individual `Command` marshaled to bytes prepended by `varint` length.

The same relates to many `Reply` in one response from server to client. Line-delimited JSON and varint-length prefixed Protobuf.

As you see above each `Command` has `id` field. This is an incremental integer field. This field will be echoed in server to client replies to commands so client could match a certain `Reply` to `Command` sent before. This is important because Websocket is asynchronous protocol where server and client both send messages in full-duplex mode.

So you can expect something like this when sending commands to server:

```javascript
{"id": 1, "result": {}}
{"id": 2, "result": {}}
```

Besides `id` `Reply` from server to client have two important fields: `result` and `error`.

`result` contains useful payload object which can be different depending on `Reply`.

`error` contains error object in case of `Command` processing resulted in some error on server. `error` is optional and if `Reply` does not have `error` then it means that `Command` processed successfuly and client can parse `result` object in an appropriate way.

`error` objects looks like this:

```javascript
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

First of all client must dial with server and then send `connect` `Command` to it.

Default Websocket endpoint in Centrifugo is:

```
ws://centrifugo.example.com/connection/websocket
```

In case of using TLS:

```
wss://centrifugo.example.com/connection/websocket
```

After successful dial to websocket endpoint client must send `connect` command to server to authorize itself.

`connect` command looks like:

```javascript
{
    "id": 1,
    "method": "connect",
    "params": {
        "user": "42",
        "exp": "1520094208",
        "info": "",
        "sign": "xxx"
    }
}
```

Where params fields are passed to client from application backend:

* string `user` - current user ID. Can be empty string for unauthorized user.
* string `exp` - timestamp seconds when client connection expires.
* string `info` - optional base64 encoded information about connection. This is JSON object encoded to base64 in case of JSON format used and arbitrary bytes for Protobuf format.
* string `sign` - HMAC SHA-256 sign generated on backend side from Centrifugo secret and fields above. This sign helps to prove the fact that client passed valid `user`, `exp`, `info` fields to server.

In response to `connect` command server sends connect reply. It looks this way:

```javascript
{
    "id":1,
    "result":{
        "client":"421bf374-dd01-4f82-9def-8c31697e956f",
        "version":"2.0.0"
    }
}
```

`result` has some fields:

* string `client` - unique client connection ID server issued to this connection
* string `version` - server version
* optional bool `expires` - whether or not server will expire connection
* optional bool `expired` - whether or not connection credentials already expired and must be refreshed
* optional int32 `ttl` - time in seconds until connection will expire

### Subscribe

As soon as client successfully connected and got unique connection ID it is ready to
subscribe on channels. To do this it must send `subscribe` command to server:

```javascript
{
    "id": 2,
    "method": "subscribe",
    "params": {
        "channel": "ch1"
    }
}
```

Fields that can be set in `params` are:

* string `channel` - channel to subscribe

In response to subscribe client receives reply like:

```javascript
{
    "id":2,
    "result":{}
}
```

`result` can have the following fields:

* optional array `publications` -
* optional string `last` - 
* optional bool `recovered` - 

After client received successful reply on `subscribe` command it will receive asynchronous 
reply messages published to this channel. Messages can be of several types:

* Publication message
* Join message
* Leave message
* Unsub message

See more about asynchronous messages below. 

### Unsubscribe

This is simple. When client wants to unsubscribe from channel and therefore stop receiving asynchronous subscription messages from connection related to channel it must call `unsubscribe` command:

```javascript
{
    "id": 3,
    "method": "unsubscribe",
    "params": {
        "channel": "ch1"
    }
}
```

Actually server response does not mean a lot for client - it must immediately remove channel subscription from internal implementation data structures and ignore all messages related to channel.

### Refresh

It's possible to turn on client connection expiration mechanism on server. While enabled server will keep track of connections whose time of life (defined by `exp` timestamp) is close to the end. In this case connection will be closed. Client can prevent closing connection refreshing it's connection credentials. To do this it must send `refresh` command to server. `refresh` command similar to `connect`:

```javascript
{
    "id": 4,
    "method": "refresh",
    "params": {
        "user": "42",
        "exp": "1520096218",
        "info": "",
        "sign": "xxx"
    }
}
```

Just with actual `exp` and new `sign`.

### RPC-like calls: publish, history, presence

The mechanics of these calls is simple - client sends command and expects response from server.

`publish` command allows to publish message into channel from client.

!!! note
    To publish from client `publish` option in server configuration must be set to `true`

`history` allows to ask server for channel history if enabled.

`presence` allows to ask server for channel presence information if enabled.

### Asynchronous server-to-client messages

There are several types of asynchronous messages that can come from server to client. All of them relate to current client subscriptions.

The most important message is `Publication`:

```javascript
{
    "result":{
        "channel":"ch1",
        "data":{
            "uid":"Ry4z8l6GvNMejwMxB7Sohe",
            "data":{"input":"1"},
            "info":{
                "user":"2694",
                "client":"5c48510e-cf49-4fa8-a9b2-490b22231e74",
                "conn_info":{"name":"Alexander"},
                "chan_info":{}
            }
        }
    }
}
```

`Publication` is a message published into channel. Note that there is no `id` field in this message - this symptom
allows to distinguish it from `Reply` to `Command`.  

Next message is `Join` message:

```javascript
{
    "result":{
        "type":1,
        "channel":"ch1",
        "data":{
            "info":{
                "user":"2694",
                "client":"5c48510e-cf49-4fa8-a9b2-490b22231e74",
                "conn_info":{"name":"Alexander"},
                "chan_info":{}
            }
        }
    }
}
```

`Join` messages sent when someone joined (subscribed on) channel.

!!! note
    To enable `Join` and `Leave` messages `join_leave` option must be enabled on server globally or for channel namespace.

`Leave` messages sent when someone left (unsubscribed from) channel.

```javascript
{
    "result":{
        "type":2,
        "channel":"ch1",
        "data":{
            "info":{
                "user":"2694",
                "client":"5c48510e-cf49-4fa8-a9b2-490b22231e74",
                "conn_info":{"name":"Alexander"},
                "chan_info":{}
            }
        }
    }
}
```

And finally `Unsub` message that means that server unsubscribed current client from channel:

```javascript
{
    "result":{
        "type":3,
        "channel":"ch1",
        "data":{}
    }
}
```

It's possible to distinguish between different types of asynchronous messages looking at `type` field (for `Publication` this field not set or `0`).

### Ping Pong

To maintain connection alive and detect broken connections client must periodically send `ping` commands to server and expect replies to it. Ping command looks like:

```javascript
{
    "id":32,
    "method":"ping"
}
```

Server just echoes this command back. When client does not receive ping reply for some time it must consider connection broken and try to reconnect. Recommended ping interval is 25 seconds, recommended period to wait for pong is 1-5 seconds. Though those numbers can vary.

### Handle disconnects

Client should handle disconnect advices from server. In websocket case disconnect advice is sent in reason field of CLOSE Websocket frame. Reason contains string which is `disconnect` object encoded into JSON (even in case of Protobuf scenario). That objects looks like:

```javascript
{
    "reason": "shutdown",
    "reconnect": true 
}
```

It contains string reason of connection closing and advice to reconnect or not. Client should take this reconnect advice into account.

In case of network problems and random disconnect from server without well known reason client should always try to  reconnect with exponential intervals.

### Handle errors

This section contains advices to error handling in client implementations.

Errors can happen during various operations and can be handled in special way in context of some commands to tolerate network and server problems.

Errors during `connect` must result in full client reconnect.

Errors during `subscribe` must result in full client reconnect if error is temporary. And be sent to error event handler of subscription if persistent. Persistent errors are errors like `permission denied`, `bad request`, `namespace not found` etc. Persistent errors in most situation mean an error from developers side.

Errors during rpc-like operations can be just returned to caller - i.e. user javascript code. Calls like `history` and `presence` are idempotent. You should be accurate with unidempotent operations like `publish` - in case of client timeout it's possible to send the same message into channel twice if retry publish after timeout - so if you care about this case make sure you have some protection from displaying message twice on client side (maybe some sort of unique key in payload).
