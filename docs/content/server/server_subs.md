# Server-side subscriptions

Starting from v2.4.0 Centrifugo supports server-side subscriptions.

### Overview

Before v2.4.0 only client could initiate subscription to channel calling `Subscribe` method. While this is actually the most flexible approach as client side usually knows well which channels it needs to consume in concrete moment – in some situations all you need is to subscribe your connections to several channels on server side at the moment of connection establishement. So client effectively starts receiving publications from those channels without explicitly call `Subscribe` method.

You can set a list of channels for connection in two ways at moment:

* over connection JWT using `channels` claim, which is an array of strings
* over connect proxy returning `channels` field in result (also an array of strings)

On client side you need to listen for publications from server-side channels using top-level client event handler. **At this moment server-side subscriptions only supported by** `centrifuge-js` client. With it all you need to do on client side is sth like this:

```javascript
var centrifuge = new Centrifuge(address);

centrifuge.on('publish', function(ctx) {
    const channel = ctx.channel;
    const payload = JSON.stringify(ctx.data);
    console.log('Publication from server-side channel', channel, payload);
});

centrifuge.connect();
```

I.e. listen for publications without any usage of subscription objects. You can look at channel publication relates to using field in callback context as shown in example.

In future this mechanism will be supported by [all our clients](../libraries/client.md). Hopefully with community help this will happen very soon.

### Server-side subscription limitations

Such subscriptions do not fit well for dynamic subscriptions – as Centrifugo does not have `subscribe` server API at moment. Also the same [rules about best practices](../faq.md#what-about-best-practices-with-amount-of-channels) of working with channels and client-side subscriptions equally applicable to server-side subscription. 

### Automatic personal channel subscription

v2.4.0 also introduced some sugar on top of server-side subscriptions. It's possible to automatically subscribe user to personal server-side channel.

To enable this you need to enable `user_subscribe_to_personal` boolean option (by default `false`). As soon as you do this every connection with non-empty user ID will be automatically subscribed to personal user-limited channel. Anonymous users with empty user ID won't be subscribed to any channel.

For example, if you set this option and user with ID `87334` connects to Centrifugo it will be automatically subscribed to channel `#87334` and you can process personal publications on client side in the same way as shown above.

As you can see by default generated personal channel name belongs to default namespace (i.e. no explicit namespace used). To set custom namespace name use `user_personal_channel_namespace` option (string, default `""`) – i.e. the name of namespace from configured configuration namespaces array. In this case if you set `user_personal_channel_namespace` to `personal` for example – then the automatically generated personal channel will be `personal:#87334` – user will be automatically subscribed to it on connect and you can use this channel name to publish personal notifications to online user.

### Mark namespace as server-side

v2.4.0 also introduced new channel namespace boolean option called `server_side` (default `false`). When on then all client-side subscription requests to channels in namespace will be rejected with `PermissionDenied` error.
