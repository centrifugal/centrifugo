This release is based on latest refactoring of Centrifuge library. The refactoring opens a road for possible interesting improvements in Centrifugo – such as possibility to use any PUB/SUB broker instead of Redis (here is an [example of possible integration](https://github.com/centrifugal/centrifugo/pull/273) with Nats server), or even combine another broker with existing Redis engine features to still have recovery and presence features. Though these ideas are not implemented in Centrifugo yet. Performance of broadcast operations can be slightly decreased due to some internal changes in Centrifuge library. Also take a close look at backwards incompatible changes section below for one breaking change.

Improvements:

* Track client position in channels with `history_recover` option enabled and disconnect in case of insufficient state. This resolves an edge case when messages could be lost in channels with `history_recover` option enabled after node reconnect to Redis (imagine situation when Redis was unavailable for some time but before Centrifugo node reconnects publisher was able to successfully send a message to channel on another node which reconnected to Redis faster). With new mechanism client won't miss messages though can receive them with some delay. As such situations should be pretty rare on practice it should be a reasonable compromise for applications. New mechanism adds more load on Redis as Centrifugo node periodically polls channel history state. The load is linearly proportional to amount of active channels with `history_recover` option on. By default Centrifugo will check client position in channel stream not often than once in 40 seconds so an additional load on Redis should not be too high
* New options for more flexible conrol over exposed endpoint interfaces and ports: `internal_address`, `tls_external`, `admin_external`. See description calling `centrifugo -h`. [#262](https://github.com/centrifugal/centrifugo/pull/262), [#264](https://github.com/centrifugal/centrifugo/pull/264)
* Small optimizations in Websocket and SockjS transports writes
* Server initiated disconnect number metrics labeled with disconnect code 

Backwards incompatible changes:

* This release removes a possibility to set `uid` to Publication over API. This feature was not documented in [API reference](https://centrifugal.github.io/centrifugo/server/api/) and `uid` field does not make sense to be kept on client protocol top level as in Centrifugo v2 it does not serve any internal protocol purpose. This is just an application specific information that can be put into `data` payload

Release is based on Go 1.12.x
