v0.11.0
=======

* Refactor client channels API – see detailed changes below, [#146](https://github.com/centrifugal/centrifuge/pull/146)
* Fix atomic alignment in struct for 32-bit builds, [commit](https://github.com/centrifugal/centrifuge/commit/cafa94fbf4173ae46d1f5329a33adec97d0620c8)
* Field `Code` of `Disconnect` has `uint32` type now instead of `int`, [commit](https://github.com/centrifugal/centrifuge/commit/d86cea2c8b309e6d2ce1f1fa8ba6fcc7d06f7320)
* Refactor WebSocket graceful close – do not use a new goroutine for every read, [#144](https://github.com/centrifugal/centrifuge/pull/144)
* Support client name and version fields of `Connect` command which will be available in `ConnectEvent` struct (if set on client side), [#145](https://github.com/centrifugal/centrifuge/pull/145)

```
$ gorelease -base v0.10.1 -version v0.11.0
github.com/centrifugal/centrifuge
---------------------------------
Incompatible changes:
- (*Client).Channels: changed from func() map[string]ChannelContext to func() []string
- ChannelContext: removed
- Disconnect.Code: changed from int to uint32
Compatible changes:
- (*Client).IsSubscribed: added
- ConnectEvent.Name: added
- ConnectEvent.Version: added
- ErrorTooManyRequests: added

v0.11.0 is a valid semantic version for this release.
```

v0.10.1
=======

* Fix Redis Engine errors when epoch is missing inside stream/list meta hash. See [commit](https://github.com/centrifugal/centrifuge/commit/f5375855e3df8e1679e11775455597cc91b8e0e5)

v0.10.0
=======

This release is a massive rewrite of Centrifuge library (actually of some part of it) which should make library a more generic solution. Several opinionated and restrictive parts removed to make Centrifuge feel as a reasonably thin wrapper on top of strict client-server protocol.

Most work done inside [#129](https://github.com/centrifugal/centrifuge/pull/129) pr and relates to [#128](https://github.com/centrifugal/centrifuge/issues/128) issue.

Release highlights:

* Layer with namespace configuration and channel rules removed. Now developer is responsible for all permission checks and channel rules.
* Hard dependency on JWT and predefined claims removed. Users are now free to use any token implementation – like [Paceto](https://github.com/paragonie/paseto) tokens for example, use any custom claims etc.
* Event handlers that not set now always lead to `Not available` error returned to client.
* All event handlers now should be set to `Node` before calling its `Run` method.
* Centrifuge still needs to know some core options for channels to understand whether to use presence inside channels, keep Publication history stream or not. It's now done over user-defined callback function in Node Config called `ChannelOptionsFunc`. See its detailed description in [library docs](https://godoc.org/github.com/centrifugal/centrifuge#ChannelOptionsFunc).
* More idiomatic error handling in event handlers, see [#134](https://github.com/centrifugal/centrifuge/pull/134).
* Aliases to `Raw`, `Publication` and `ClientInfo` Protobuf types removed from library public API, see [#136](https://github.com/centrifugal/centrifuge/issues/136)
* Support Redis Sentinel password option

Look at updated [example in README](https://github.com/centrifugal/centrifuge#quick-example) and [examples](https://github.com/centrifugal/centrifuge/tree/master/_examples) folder to find out more. 

I hope to provide more guidance about library concepts in the future. I feel sorry for breaking things here but since we don't have v1 release yet, I believe this is acceptable. An important note is that while this release has lots of removed parts it's still possible (and not too hard) to implement the same functionality as before on top of this library. Feel free to ask any questions in our community chats.

v0.9.1
======

* fix `Close` method – do not use error channel since this leads to deadlock anyway, just close in goroutine.
* fix presence timer scheduling

```
gorelease -base=v0.9.0 -version=v0.9.1
github.com/centrifugal/centrifuge
---------------------------------
Incompatible changes:
- (*Client).Close: changed from func(*Disconnect) chan error to func(*Disconnect) error

v0.9.1 is a valid semantic version for this release.
```

v0.9.0
======

This release has some API changes. Here is a list of all changes in release:

```
Incompatible changes:
- (*Client).Close: changed from func(*Disconnect) error to func(*Disconnect) chan error
- (*Client).Handle: removed
- Config.ClientPresencePingInterval: removed
- NewClient: changed from func(context.Context, *Node, Transport) (*Client, error) to func(context.Context, *Node, Transport) (*TransportClient, CloseFunc, error)
- NodeEventHub.ClientRefresh: removed
- RefreshHandler: changed from func(context.Context, *Client, RefreshEvent) RefreshReply to func(RefreshEvent) RefreshReply
Compatible changes:
- (*ClientEventHub).Presence: added
- (*ClientEventHub).Refresh: added
- CloseFunc: added
- Config.ClientPresenceUpdateInterval: added
- ConnectReply.ClientSideRefresh: added
- PresenceEvent: added
- PresenceHandler: added
- PresenceReply: added
- RefreshEvent.Token: added
- RefreshReply.Disconnect: added
- TransportClient: added
```

Now let's try to highlight most notable changes and reasoning behind:

* `NewClient` returns `TransportClient` and `CloseFunc` to limit possible API on transport implementation level
* `ClientPresencePingInterval` config option renamed to `ClientPresenceUpdateInterval`
* Centrifuge now has `client.On().Presence` handler which will be called periodically while connection alive every `ClientPresenceUpdateInterval`
* `Client.Close` method now creates a goroutine internally - this was required to prevent deadlock when closing client from Presence and SubRefresh callback handlers. 
* Refresh handler moved to `Client` scope instead of being `Node` event handler
* `ConnectReply` now has new `ClientSideRefresh` field which allows setting what kind of refresh mechanism should be used for a client: server-side refresh or client-side refresh.
* It's now possible to do client-side refresh with custom token implementation ([example](https://github.com/centrifugal/centrifuge/tree/master/_examples/custom_token))
* Library now uses one concurrent timer per each connection instead of 3 - should perform a bit better

All examples updated to reflect all changes here. 

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
