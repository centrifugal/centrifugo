v0.8.2
======

* Fix Disconnect Code field unmarshalling, introduce helper method `Disconnect.CloseText()` to build close text sent to client in Close Frame. 
* Fix server-side Join event wrong channel when server subscribed client to several channels with JoinLeave feature on

v0.8.1
======

* Fix closing connections with `insufficient state` after publish when history recovery feature is on and `PublishOnHistoryAdd` is `false` in Redis Engine config. [#119](https://github.com/centrifugal/centrifuge/issues/119).

v0.8.0
======

This release is huge. In general, it does not change any previously defined semantics but changes API. The [list of changes is big enough](https://gist.github.com/FZambia/f59ce1c82ceb23286ccf427623a45e37) but fixes you need to do for this version adoption are mostly minimal. Except one thing emphasized below.

**So here is that thing.** Centrifuge now uses new `offset` `uint64` protocol field for Publication position inside history stream instead of previously used `seq` and `gen` (both `uint32`) fields. It replaces both `seq` and `gen`. This change required to simplify working with history API in perspective. This is a breaking change for library users in case of using history recovery feature – read migration steps below.

Our client libraries `centrifuge-js` and `centrifuge-go` were updated to use `offset` field. So if you are using these two libraries and utilizing recovery feature then you need to update `centrifuge-js` to at least `2.6.0`, and `centrifuge-go` to at least `0.5.0` to match server protocol (see the possibility to be backwards compatible on server below). All other client libraries do not support recovery at this moment so should not be affected by field changes described here.

It's important to mention that to provide backwards compatibility on client side both `centrifuge-js` and `centrifuge-go` will continue to properly work with a server which is using old `seq` and `gen` fields for recovery in its current form until v1 version of this library. It's possible to explicitly enable using old `seq` and `gen` fields on server side by calling:

```
centrifuge.CompatibilityFlags |= centrifuge.UseSeqGen
```

This allows doing migration to v0.8.0 and keeping everything compatible. Those `CompatibilityFlags` **will be supported until v1 library release**. Then we will only have one way to do things.

Other release highlights:

* support [Redis Streams](https://redis.io/topics/streams-intro) - radically reduces amount of allocations during recovery in large history streams, also provides a possibility to paginate over history stream (an API for pagination over stream added to `Node` - see `History` method)
* support [Redis Cluster](https://redis.io/topics/cluster-tutorial), client-side sharding between different Redis Clusters also works
* use [alternative library](https://github.com/cristalhq/jwt) for JWT parsing and verification - HMAC-based JWT parsing is about 2 times faster now. Related issues: [#109](https://github.com/centrifugal/centrifuge/pull/109) and [#107](https://github.com/centrifugal/centrifuge/pull/107).
* new data structure for in-memory streams (linked list + hash table) for faster insertion and recovery in large streams, also it's now possible to expire a stream meta information in case of Memory engine
* fix server side subscriptions to private channels (were ignored before)
* fix `channels` counter update frequency ([commit](https://github.com/centrifugal/centrifuge/commit/23a87fd538e44894f220146ced47a7e946bcf60d))
* drop Dep support - library now uses Go mod only for dependency management
* slightly improved test coverage
* lots of internal refactoring and style fixes

Thanks to [@Skarm](https://github.com/Skarm), [@cristaloleg](https://github.com/cristaloleg) and [@GSokol](https://github.com/GSokol) for contributions.

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
