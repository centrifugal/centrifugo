v0.7.0
======

* Refactor automatic subscribing to personal channel option. Option that enables feature renamed from `UserSubscribePersonal` to `UserSubscribeToPersonal`, also instead of `UserPersonalChannelPrefix` users can set `UserPersonalChannelNamespace` option, the general advice here is to create separate namespace for automatic personal channels if one requires custom channel options
* `WebsocketUseWriteBufferPool` option for SockJS handler

v0.6.0
======

* Simplify server-side subscription API replacing `[]centrifuge.Subscription` with just `[]string` - i.e. a slice of channels we want to subscribe connection to. For now it seems much more simple to just use a slice of strings and this must be sufficient for most use cases. It is also a bit more efficient for JWT use case in terms of its payload size. More complex logic can be introduced later over separate field of `ConnectReply` or `connectToken` if needed
* Support server-side subscriptions via JWT using `channels` claim field

v0.5.0
======

This release introduces server-side subscription support - see [#89](https://github.com/centrifugal/centrifuge/pull/89) for details. Release highlights:

* New field `Subscriptions` for `ConnectReply` to provide server side subscriptions
* New `Client.Subscribe` method
* New node configuration options: `UserSubscribePersonal` and `UserPersonalChannelPrefix`
* New `ServerSide` boolean namespace option
* Method `Client.Unsubscribe` now accepts one main argument (`channel`) and `UnsubscribeOptions`
* New option `UseWriteBufferPool` for WebSocket handler config
* Internal refactor of JWT related code, many thanks to [@Nesty92](https://github.com/Nesty92)
* Introduce subscription dissolver - node now reliably unsubscribes from PUB/SUB channels, see details in [#77](https://github.com/centrifugal/centrifuge/issues/77)

v0.4.0
======

* `Secret` config option renamed to `TokenHMACSecretKey`, that was reasonable due to recent addition of RSA-based tokens so we now have `TokenRSAPublicKey` option
