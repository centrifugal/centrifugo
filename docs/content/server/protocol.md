# Client protocol

This chapter describes internal client-server protocol in details to help developers build new client libraries and understand how existing client libraries work.

Note that you can always look at [existing client implementations](../libraries/client.md) in case of any questions.

### Client implementation checklist

First we will look at list of features client library should support. Depending on client implementation some features can be not implemented. If you an author of client library you can use this list as checklist.

!!! note
    Field and method names presented in this checklist can have different names depending on the programmer's taste and language style guide.

So, client:

* Should work both with Centrifugo and Centrifuge library based server. To work with Centrifugo client must have a method to set connection token. To work with Centrifuge lib client must provide a method to set custom headers (the only exception is browser clients where browser automatically sets headers with respect to domain rules)
* Should allow to use JSON payload
* Should allow to use binary payload (actually you can only implement Protobuf protocol as you can pass JSON over it)
* Must handle cases when many different Replies received in one websocket frame. In case of JSON protocol newline-delimited JSON-streaming format is used to combine several Replies into one websocket frame. In case of Protobuf protocol varint length-prefixed format is used
* Must survive server reload/restart and internet connection lost. In this case client must try to reconnect with exponentioal backoff strategy
* Must have several callback methods: `onConnect`, `onDisconnect`. Depending on implementation you can also use `onError` callback for critical errors that could not be gracefully handled. 
* Should also have `onMessage` callback to handle async messages from server.
* Must have method to subscribe on channel and set several event handlers: `onPublish`, `onJoin`, `onLeave`, `onSubscribeSuccess`, `onSubscribeError`, `onUnsubscribe`. After subscribe method called it should return `Subscription` object to caller. This subscription object in turn should have some methods: `publish`, `unsubscribe`, `subscribe`, `history`, `presence`, `presence_stats`.
* Should have `publish` method to publish into channel without subscribing to it.
* Should have `rpc` method to send RPC request to server.
* Should have `send` method to send asynchronous message to server (without waiting response).
* Should handle disconnect reason. In case of Websocket it is sent by server in CLOSE Websocket frame. This is a string containing JSON object with fields: `reason` (string) and `reconnect` (bool). Client should give users access to these fields in disconnect event and automatically follow `reconnect` advice
* Must send periodic `ping` commands to server and thus detect broken connection. If no ping reply received from server in configured window reconnect workflow must be initiated
* Should fully reconnect if subscription request timed out. Timeout can be configured by client library users.
* Should send commands to server with timeout and handle timeout error - depending on method called timeout error handling can differ a bit. For example as said above timeout on subscription request results in full client reconnect workflow.
* Should support connection token refresh mechanism
* Should support private channel subscriptions and private subscription token refresh mechanism
* Should automatically recover messages after reconnect and set appropriate fields in subscribe event context. Two important fields in `onSubscribeSuccess` event context are `recovered` and `isResubscribe`. First field let user know what server thinks about subscription state - were all messages recovered or not. The second field must only be true if resubscribe was caused by temporary network connection lost. If user initiated resubscribe himself (calling `unsubscribe` method and then `subscribe` method) then recover workflow should not be used and `isResubscribe` must be `false`.

Below in this document we will describe protocol concepts in detail.

This document describes protocol specifics for Websocket transport which supports binary and text formats to transfer data. As Centrifuge has various types of messages it serializes protocol messages using JSON or Protobuf (in case of binary websockets).

!!! note
    SockJS works almost the same way as JSON websocket described here but has its own extra framing on top of Centrifuge protocol messages. SockJS can only work with JSON - it's not possible to transfer binary data over it. SockJS is only needed as fallback to Websocket in web browsers.

### Top level framing

Centrifuge protocol defined in [Protobuf schema](https://github.com/centrifugal/protocol/blob/master/definitions/client.proto). That schema is a source of truth and all protocol description below describes messages from that schema.

Client sends `Command` to server.

Server sends `Reply` to client.

One request from client to server and one response from server to client can have more than one `Command` or `Reply`.

When JSON format is used then many `Command` can be sent from client to server in JSON streaming line-delimited format. I.e. each individual `Command` encoded to JSON and then commands joined together using new line symbol `\n`:

```javascript
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
    This doc will use JSON format for examples because it's human-readable. Everything said here for JSON is also true for Protobuf encoded case. The only difference is how several individual `Command` or server `Reply` joined into one request – see below.

!!! note
    Method is made as ENUM in protobuf schema and can be sent as integer value but it's possible to send it as string in JSON case – this was made to make JSON protocol human-friendly.

When Protobuf format is used then many `Command` can be sent from client to server in length-delimited format where each individual `Command` marshaled to bytes prepended by `varint` length. See existing client implementations for encoding example.

The same rules relate to many `Reply` in one response from server to client. Line-delimited JSON and varint-length prefixed Protobuf.

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

As you can see each `Command` has `id` field. This is an incremental integer field. This field will be echoed in server to client replies to commands so client could match a certain `Reply` to `Command` sent before. This is important because Websocket is asynchronous protocol where server and client both send messages at any moment and there is no builtin request-response pattern. Having `id` allows to match reply to command send before.

So you can expect something like this in response after sending commands to server:

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
    "message": "internal server error"
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
        "token": "JWT",
        "data": {}
    }
}
```

Where params fields are passed to client from application backend:

* string `token` - connection token.
* JSON `data` - this is only available for Centrifuge library and not for Centrifugo server. It contains custom connect data, for example it can contain client settings. 

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

`result` can have the following fields that relate to subscription expiration:

* optional bool `expires` - indicates whether subscription expires or not.
* optional uint32 `ttl` - number of seconds until subscription expire.

And several fields that relate to message recovery:

* optional bool `recoverable` - means that messages can be recovered in this subscription.
* optional uint32 `seq` - current publication sequence inside channel
* optional uint32 `gen` - current publication generation inside channel
* optional string `epoch` - current epoch inside channel
* optional array `publications` - this is an array of missed publications in channel. When received client must call general publication event handler for each message in this array.
* optional bool `recovered` - this flag is set to `true` when server thinks that all missed publications were successfully recovered and send in subscribe reply (in `publications` array) and `false` otherwise.

See more about meaning of recovery related fields in [special doc chapter](recover.md).

After client received successful reply on `subscribe` command it will receive asynchronous reply messages published to this channel. Messages can be of several types:

* `Publication` message
* `Join` message
* `Leave` message
* `Unsub` message

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
        "token": "JWT"
    }
}
```

Just with actual `exp` and new `sign`.

The tip whether or not connection must be refreshed comes in reply to `connect` command shown above - fields `expires` and `ttl`.

When client connection expire mechanism is on the value of field `expires` in connect reply is `true`. In this case client implementation should look at `ttl` value which is seconds left until connection will be considered expired. Client must send `refresh` command after this `ttl` seconds. Server gives client a configured window to refresh token after `ttl` passed and then closes connection if client have not updated its token.

When connecting with already expired token an error will be returned (with code `109`). In this case client should refresh its token and reconnect with exponential backoff. 

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

Errors during `connect` must result in full client reconnect with exponential backoff strategy. The special case is error with code `110` which signals that connection token already expired. As we said above client should update its connection JWT before connecting to server again.  

Errors during `subscribe` must result in full client reconnect in case of internal error (code `100`). And be sent to subscribe error event handler of subscription if received error is persistent. Persistent errors are errors like `permission denied`, `bad request`, `namespace not found` etc. Persistent errors in most situation mean a mistake from developers side.

The special corner case is client-side timeout during `subscribe` operation. As protocol is asynchronous it's possible in this case that server will eventually subscribe client on channel but client will think that it's not subscribed. It's possible to retry subscription request and tolerate `already subscribed` (code `105`) error as expected. But the simplest solution is to reconnect entirely as this is simpler and gives client a chance to connect to working server instance.

Errors during rpc-like operations can be just returned to caller - i.e. user javascript code. Calls like `history` and `presence` are idempotent. You should be accurate with unidempotent operations like `publish` - in case of client timeout it's possible to send the same message into channel twice if retry publish after timeout - so users of libraries must care about this case – making sure they have some protection from displaying message twice on client side (maybe some sort of unique key in payload).

### Client implementation advices

Here are some advices about client public API. Examples here are in Javascript language. This is just an attempt to help in developing a client - but rules here is not obligatorily the best way to implement client.

Create client instance:

```javascript
var centrifuge = new Centrifuge("ws://localhost:8000/connection/websocket", {});
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
