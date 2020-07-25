# Client protocol

This chapter describes internal client-server protocol in details to help developers build new client libraries and understand how existing client libraries work.

Note that you can always look at [existing client implementations](../libraries/client.md) in case of any questions. Not all clients support all available server features though.

### Client implementation feature matrix

First we will look at list of features client library should support. Depending on client implementation some features can be not implemented. If you an author of client library you can use this list as checklist.

Our current client feature matrix looks like this:

- [x] connect to server (both Centrifugo and Centrifuge-based) using JSON protocol format
- [x] connect to server (both Centrifugo and Centrifuge-based) using Protobuf protocol format
- [x] connect to server with token (JWT in Centrifugo case, any string token in Centrifuge library case)
- [x] connect to server with custom headers (not available in a browser)
- [x] automatic reconnect in case of dial problems (network)
- [x] an exponential backoff for reconnect process
- [x] possibility to set handlers for connect and disconnect events
- [x] extract and expose disconnect reason
- [x] subscribe on a channel and provide a way to handle asynchronous Publications coming from a channel
- [x] handle Join and Leave messages from a channel
- [x] handle Unsubscribe notifications
- [x] publish method of Subscription
- [x] unsubscribe method of Subscription
- [x] presence method of Subscription
- [x] presence stats method of Subscription
- [x] history method of Subscription
- [x] send asynchronous messages to server
- [x] handle asynchronous messages from server
- [x] send RPC requests to server
- [x] publish to channel without being subscribed
- [x] subscribe to private (token-protected) channels with token
- [x] connection token refresh mechanism
- [x] private channel subscription token refresh
- [x] client protocol level ping/pong to find broken connection
- [x] automatic reconnect in case of connect or subscribe command timeouts
- [x] handle connection expired error
- [x] handle subscription expired error
- [x] server-side subscriptions
- [x] message recovery mechanism

Below I'll try to describe most of these points in detail.

This document describes protocol specifics for Websocket transport which supports binary and text formats to transfer data. As Centrifugo and Centrifuge library for Go have various types of messages it serializes protocol messages using JSON or Protobuf formats.

!!! note
    SockJS works almost the same way as JSON websocket described here but has its own extra framing on top of Centrifuge protocol messages. SockJS can only work with JSON - it's not possible to transfer binary data over it. SockJS is only needed as fallback to Websocket in web browsers.

### Top level framing

Centrifuge protocol defined in [Protobuf schema](https://github.com/centrifugal/protocol/blob/master/definitions/client.proto). That schema is a source of truth and all protocol description below describes messages from that schema.

Client sends `Command` to server. Server sends `Reply` to client. All communication between client and server is a bidirectional exchange of `Command` and `Reply` messages.

One request from client to server and one response from server to client can have more than one `Command` or `Reply`. This allows reducing number of system calls for writing and reading data.

When JSON format used then many `Command` can be sent from client to server in JSON streaming line-delimited format. I.e. each individual `Command` encoded to JSON and then commands joined together using new line symbol `\n`:

```text
{"id": 1, "method": "subscribe", "params": {"channel": "ch1"}}
{"id": 2, "method": "subscribe", "params": {"channel": "ch2"}}
```

For example here is how we do this in Javascript client when JSON format used:

```javascript
function encodeCommands(commands) {
    const encodedCommands = [];
    for (const i in commands) {
      if (commands.hasOwnProperty(i)) {
        encodedCommands.push(JSON.stringify(commands[i]));
      }
    }
    return encodedCommands.join('\n');
}
```

!!! note
    This doc will use JSON format for examples because it's human-readable. Everything said here for JSON is also true for Protobuf encoded case. The only difference is how several individual `Command` or server `Reply` joined into one request – see details below.

!!! note
    Method represented as a ENUM in protobuf schema and can be sent as integer value. Though it's possible to send it as string in JSON case – this was made to make JSON protocol human-friendly.

When Protobuf format used then many `Command` can be sent from client to server in length-delimited format where each individual `Command` marshaled to bytes prepended by `varint` length. See existing client implementations for encoding example.

The same rules relate to many `Reply` in one response from server to client. Line-delimited JSON and varint-length prefixed Protobuf also used there.

For example here is how we read server response and extracting individual replies in Javascript client when JSON format used:

```javascript
function decodeReplies(data) {
    const replies = [];
    const encodedReplies = data.split('\n');
    for (const i in encodedReplies) {
      if (encodedReplies.hasOwnProperty(i)) {
        if (!encodedReplies[i]) {
          continue;
        }
        const reply = JSON.parse(encodedReplies[i]);
        replies.push(reply);
      }
    }
    return replies;
}
```

For Protobuf case see existing client implementations for decoding example.

As you can see each `Command` has `id` field. This is an incremental `uint32` field. This field will be echoed in a server replies to commands so client could match a certain `Reply` to `Command` sent before. This is important since Websocket is an asynchronous protocol where server and client both send messages at any moment and there is no builtin request-response matching. Having `id` allows matching a reply with a command send before.

So you can expect something like this in response after sending commands to server:

```text
{"id": 1, "result": {}}
{"id": 2, "result": {}}
```

Besides `id` `Reply` from server to client have two important fields: `result` and `error`.

`result` contains useful payload object which is different for various `Reply` messages.

`error` contains error description if `Command` processing resulted in some error on a server. `error` is optional and if `Reply` does not have `error` then it means that `Command` processed successfully and client can parse `result` object appropriately.

`error` looks like this in JSON case:

```json
{
    "code": 100,
    "message": "internal server error"
}
```

We will talk more about error handling below.

The special type of `Reply` is **asynchronous** `Reply`. Such replies have no `id` field set (or `id` can be equal to zero). Async replies can come to client in any moment - not as reaction to issued `Command` but as message from server to client in arbitrary time. For example this can be a message published into channel.

Centrifuge library defines several command types client can issue. A well-written client must be aware of all those commands and client workflow. Communication with Centrifuge/Centrifugo server starts with issuing `connect` command.

### Connect

First of all client must dial with a server and then send `connect` `Command` to it.

Default Websocket endpoint in Centrifugo is:

```
ws://centrifugo.example.com/connection/websocket
```

In case of using TLS:

```
wss://centrifugo.example.com/connection/websocket
```

After a successful dial to WebSocket endpoint client must send `connect` command to server to authorize itself.

`connect` command looks like:

```json
{
    "id": 1,
    "method": "connect",
    "params": {
        "token": "JWT",
        "data": {}
    }
}
```

Where params fields:

* optional string `token` - connection token. Can be ommited if token-based auth not used.
* `data` - can contain custom connect data, for example it can contain client settings.

In response to `connect` command server sends a connect reply. It looks this way:

```json
{
    "id": 1,
    "result":{
        "client": "421bf374-dd01-4f82-9def-8c31697e956f",
        "version": "2.0.0"
    }
}
```

`result` has some fields:

* string `client` - unique client connection ID server issued to this connection
* string `version` - server version
* optional bool `expires` - whether a server will expire connection at some point
* optional int32 `ttl` - time in seconds until connection expires

### Subscribe

As soon as client successfully connected and got unique connection ID it is ready to
subscribe on channels. To do this it must send `subscribe` command to server:

```json
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

In response to subscribe a client receives reply like:

```json
{
    "id": 2,
    "result": {}
}
```

`result` can have the following fields that relate to subscription expiration:

* optional bool `expires` - indicates whether subscription expires or not.
* optional uint32 `ttl` - number of seconds until subscription expire.

Also several fields that relate to message recovery:

* optional bool `recoverable` - means that messages can be recovered in this subscription.
* optional uint64 `offset` - current publication offset inside channel
* optional string `epoch` - current epoch inside channel
* optional array `publications` - this is an array of missed publications in channel. When received client must call general publication event handler for each message in this array.
* optional bool `recovered` - this flag set to `true` when server thinks that all missed publications successfully recovered and send in subscribe reply (in `publications` array) and `false` otherwise.

See more about meaning of recovery related fields in [special doc chapter](recovery.md).

After a client received a successful reply on `subscribe` command it will receive asynchronous reply messages published to this channel. Messages can be of several types:

* `Publication` message
* `Join` message
* `Leave` message
* `Unsub` message
* `Message` message
* `Sub` message

See more about asynchronous messages below. 

### Unsubscribe

When client wants to unsubscribe from a channel and therefore stop receiving asynchronous subscription messages from connection related to channel it must call `unsubscribe` command:

```json
{
    "id": 3,
    "method": "unsubscribe",
    "params": {
        "channel": "ch1"
    }
}
```

Actually server response does not mean a lot for a client - it must immediately remove channel subscription from internal implementation data structures and ignore all messages related to channel.

### Refresh

It's possible to turn on client connection expiration mechanism on a server. While enabled server will keep track of connections whose time of life is close to the end (connection lifetime set on connection authentication phase). In this case connection will be closed. Client can prevent closing connection refreshing its connection credentials. To do this it must send `refresh` command to server. `refresh` command is similar to `connect`:

```json
{
    "id": 4,
    "method": "refresh",
    "params": {
        "token": "<refreshed token>"
    }
}
```

The tip whether a connection must be refreshed by a client comes in reply to `connect` command shown above - fields `expires` and `ttl`.

When client connection expire mechanism is on the value of field `expires` in connect reply is `true`. In this case client implementation should look at `ttl` value which is seconds left until connection will be considered expired. Client must send `refresh` command after this `ttl` seconds. Server gives client a configured window to refresh token after `ttl` passed and then closes connection if client have not updated its token.

When connecting with already expired token an error will be returned (with code `109`). In this case client should refresh its token and reconnect with exponential backoff. 

### RPC-like calls: publish, history, presence

The mechanics of these calls is simple - client sends command and expects response from server.

`publish` command allows to publish a message into a channel from a client.

!!! note
    To publish from client `publish` option in server configuration must be set to `true`

`history` allows asking a server for channel history if enabled.

`presence` allows asking a server for channel presence information if enabled.

### Asynchronous server-to-client messages

There are several types of asynchronous messages that can come from server to client. All of them relate to current client subscriptions.

The most important message is `Publication`:

```json
{
    "result":{
        "channel":"ch1",
        "data":{
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

```json
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

```json
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

Finally `Unsub` message that means that server unsubscribed current client from a channel:

```json
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

```json
{
    "id":32,
    "method":"ping"
}
```

Server just echoes this command back. When client does not receive ping reply for some time it must consider connection broken and try to reconnect. Recommended ping interval is 25 seconds, recommended period to wait for pong is 1-5 seconds. Though those numbers can vary.

### Handle disconnects

Client should handle disconnect advices from server. In websocket case disconnect advice is sent in reason field of CLOSE Websocket frame. Reason contains string which is `disconnect` object encoded into JSON (even in case of Protobuf scenario). That objects looks like:

```json
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

Errors during `connect` must result in full client reconnect with exponential backoff strategy. The special case is error with code `110` which signals that connection token already expired. As we said above client should update its connection JWT before connecting to server again.  

Errors during `subscribe` must result in full client reconnect in case of internal error (code `100`). And be sent to subscribe error event handler of subscription if received error is persistent. Persistent errors are errors like `permission denied`, `bad request`, `namespace not found` etc. Persistent errors in most situation mean a mistake from developers side.

The special corner case is client-side timeout during `subscribe` operation. As protocol is asynchronous it's possible in this case that server will eventually subscribe client on channel but client will think that it's not subscribed. It's possible to retry subscription request and tolerate `already subscribed` (code `105`) error as expected. But the simplest solution is to reconnect entirely as this is simpler and gives client a chance to connect to working server instance.

Errors during rpc-like operations can be just returned to caller - i.e. user javascript code. Calls like `history` and `presence` are idempotent. You should be accurate with unidempotent operations like `publish` - in case of client timeout it's possible to send the same message into channel twice if retry publish after timeout - so users of libraries must care about this case – making sure they have some protection from displaying message twice on client side (maybe some sort of unique key in payload).

### Client implementation advices

Here are some advices about client public API. Examples here are in Javascript language. This is just an attempt to help in developing a client - but rules here is not obligatorily the best way to implement client.

Create client instance:

```javascript
const centrifuge = new Centrifuge("ws://localhost:8000/connection/websocket", {});
```

Set connection token (in case of using Centrifugo):

```javascript
centrifuge.setToken("XXX")
```

Connect to server:

```javascript
centrifuge.connect();
```

2 event handlers can be set to `centrifuge` object: `connect` and `disconnect`

```javascript
centrifuge.on('connect', function(context) {
    console.log(context);
});

centrifuge.on('disconnect', function(context) {
    console.log(context);
});
```

Client created in `disconnected` state with `reconnect` attribute set to `true` and `reconnecting` flag set to `false` . After `connect()` called state goes to `connecting`. It's only possible to connect from `disconnected` state. Every time `connect()` called `reconnect` flag of client must be set to `true`. After each failed connect attempt state must be set to `disconnected`, `disconnect` event must be emitted (only if `reconnecting` flag is `false`), and then `reconnecting` flag must be set to `true` (if client should continue reconnecting) to not emit `disconnect` event again after next in a row connect attempt failure. In case of failure next connection attempt must be scheduled automatically with backoff strategy. On successful connect `reconnecting` flag must be set to `false`, backoff retry must be resetted and `connect` event must be emitted. When connection lost then the same set of actions as when connect failed must be performed.

Client must allow to subscribe on channels:

```javascript
var subscription = centrifuge.subscribe("channel", eventHandlers);
```

Subscription object created and control immediately returned to caller - subscribing must be performed asynchronously. This is required because client can automatically reconnect later so event-based model better suites for subscriptions. 

Subscription should support several event handlers:

* handler for publication received from channel
* join message handler
* leave message handler
* error handler
* subscribe success event handler
* unsubscribe event handler

Every time client connects to server it must restore all subscriptions.

Every time client disconnects from server it must call unsubscribe handlers for all active subscriptions and then emit disconnect event.

Client must periodically (once in 25 secs, configurable) send ping messages to server. If pong has not beed received in 5 secs (configurable) then client must disconnect from server and try to reconnect with backoff strategy.

Client can automatically batch several requests into one frame to server and also must be able to handle several replies received from server in one frame.

### Server side subscriptions (SSS)

It's also possible to subscribe connection to channels on server side. In this case we call this server-side subscription. Client should only handle asynchronous messages coming from a server without need to create subscriptions on client side.

* SSS should be kept separate from client-side subs
* SSS requires new event handlers on top-level of Client - Subscribe, Publish, Join, Leave, Unsubscribe, event handlers will be called with event context similar to client-side subs case but with `channel` field
* Connect Reply contains SSS set by a server on connect, on reconnect client has a chance to recover missed Publications
* Server side subscription can happen at any moment - `Sub` Push will be sent to client

### Message recovery

Client should automatically recover messages after being disconnected due to network problems and set appropriate fields in subscribe event context. Two important fields in `onSubscribeSuccess` event context are `isRecovered` and `isResubscribe`. First field let user know what server thinks about subscription state - were all messages recovered or not. The second field must only be true if resubscribe was caused by temporary network connection lost. If user initiated resubscribe himself (calling `unsubscribe` method and then `subscribe` method) then recover workflow should not be used and `isResubscribe` must be `false`.

### Disconnect code and reason

In case of Websocket it is sent by server in CLOSE Websocket frame. This is a string containing JSON object with fields: `reason` (string) and `reconnect` (bool). Client should give users access to these fields in disconnect event and automatically follow `reconnect` advice.

### Additional notes

Centrifugo and Centrifuge-based server do not allow one client connection to subscribe on the same channel twice. In this case client will receive `already subscribed` error in reply to subscribe command.
