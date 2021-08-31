# Client API

!!!danger

    This is a documentation for Centrifugo v2. The latest Centrifugo version is v3. Go to the [centrifugal.dev](https://centrifugal.dev) for v3 docs.

This chapter describes client Centrifugo API – i.e. **primitives to communicate with a server from an application front-end** (i.e. browser or mobile device).

This is a formal description – refer to each specific client implementation for concrete method names and possibilities. See [full list of Centrifugo client connectors](../libraries/client.md).

If you are looking for a detailed information about client-server protocol internals then [client protocol description](../transports/protocol.md) chapter can help.

We use Javascript client `centrifuge-js` for examples here.

## Connect

Each Centrifugo client allows connecting to a server.

```javascript
const centrifuge = new Centrifuge('ws://localhost:8000/connection/websocket');
centrifuge.connect();
```

In most cases you will need to pass JWT token for authentication, so the example above transforms to:

```javascript
const centrifuge = new Centrifuge('ws://localhost:8000/connection/websocket');
centrifuge.setToken('<USER-JWT-TOKEN>')
centrifuge.connect();
```

See [authentication](authentication.md) chapter for more information on how to generate connection JWT.

If you are using [connect proxy](proxy.md#connect-proxy) then you may go without setting JWT.

## Disconnect

After connecting you can disconnect from a server at any moment.

```javascript
centrifuge.disconnect();
```

## Connection lifecycle events

All client implementations allow setting handlers on connect and disconnect events.

For example:

```javascript
centrifuge.on('connect', function(connectCtx){
    console.log('connected', connectCtx)
});

centrifuge.on('disconnect', function(disconnectCtx){
    console.log('disconnected', disconnectCtx)
});
```

## Subscribe

Another core functionality of client API is possibility to subscribe on a channel to receive all messages published to that channel.

```javascript
centrifuge.subscribe('channel', function(messageCtx) {
    console.log(messageCtx);
})
```

Client can subscribe to [different channels](channels.md). Subscribe method returns `Subscription` object. It's also possible to react on different Subscription events: join and leave events, subscribe success and subscribe error events, unsubscribe event.

In idiomatic case messages published to channels from application backend [over Centrifugo server API](http_api.md). Though it's not always true.

Centrifugo also provides message recovery feature to restore missed publications in channels. Publications can be missed due to temporary disconnects (bad network) or server reloads. Recovery happens automatically on reconnect (due to bad network or server reloads) as soon as recovery in channel [properly configured](channels.md#channel-options). Client keeps last seen Publication offset and restores new publications since known offset. If recovery failed then client implementation provides flag inside subscribe event to let your application know that some publications missed – so you may need to load state from scratch from application backend. Not all Centrifugo clients implement recovery feature – refer to specific client implementation docs. More details about recovery in [a dedicated chapter](../transports/recovery.md).

## RPC

Client can send RPC to a server. RPC is a call which is not related to channels at all. It's just a way to call server method from client side over WebSocket or SockJS connection. RPC is only available when [RPC proxy](proxy.md#rpc-proxy) configured.

```javascript
const rpcRequest = {'key': 'value'};
const data = await centrifuge.namedRPC('example_method', rpcRequest);
```

## History

Once subscribed client can call publication history inside channel (only for channels where [history configured](channels.md#channel-options)) to get last publications in channel:

```javascript
const resp = await subscription.history();
```

## Presence and presence stats

Once subscribed client can call presence and presence stats information inside channel (only for channels where [presence configured](channels.md#channel-options)):

For presence (full information about active subscribers in channel):

```javascript
const resp = await subscription.presence();
```

For presence stats (just a number of clients and unique users in channel):

```javascript
const resp = await subscription.presenceStats();
```

## Server-side subscriptions

To handle publications coming from [server-side subscriptions](server_subs.md) client API allows listening publications simply on Centrifuge client instance:

```javascript
centrifuge.on('publish', function(messageCtx) {
    console.log(messageCtx);
});
```

It's also possible to react on different server-side Subscription events: join and leave events, subscribe success, unsubscribe event. There is no subscribe error event here since subscription initiated on a server side.
