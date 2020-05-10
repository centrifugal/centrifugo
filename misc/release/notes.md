This release **contains one breaking change** – read migration instruction below. We suggest carefully test your application with a new version of Centrifugo if it uses history and history recovery features.

Here is that change. Centrifugo now uses new `offset` `uint64` protocol field for Publication position inside history stream instead of previously used `seq` and `gen` (both `uint32`) fields. `offset` replaces both `seq` and `gen`. This change required to simplify working with history API, and we have plans to extend history API in future releases. This is a breaking change for library users in case of using history recovery feature. Our client libraries `centrifuge-js` and `centrifuge-go` were updated to use `offset` field. So if you are using these two libraries and utilizing recovery feature then you need to update `centrifuge-js` to at least `2.6.0`, and `centrifuge-go` to at least `0.5.0` to match server protocol. All other client libraries do not support recovery at this moment so should not be affected by field changes described here.

It's important to mention that to provide backwards compatibility on client side both `centrifuge-js` and `centrifuge-go` will continue to properly work with a server which is using old `seq` and `gen` fields for recovery in its current form until Centrifugo v3 release. So if you update `centrifuge-js` and `centrifug-go` but don't upgrade Centrifugo to `v0.8.0` – nothing should break.

There is a way to enable option to migrate to this version of server and be **fully backwards compatible**. This can be achieved by using a boolean option `use_seq_gen`, so if you don't have a possibility to update mentioned client libraries then add this to server configuration:

```json
{
  ...
  "use_seq_gen": true
}
```

Improvements:

* support [Redis Streams](https://redis.io/topics/streams-intro) - radically reduces amount of memory allocations during recovery in large history streams, this also opens a road to paginate over history stream in future releases, see description of new `redis_streams` option [in Redis engine docs](https://centrifugal.github.io/centrifugo/server/engines/#redis-engine)
* support [Redis Cluster](https://redis.io/topics/cluster-tutorial), client-side sharding between different Redis Clusters also works, see more [in docs](https://centrifugal.github.io/centrifugo/server/engines/#redis-cluster)
* faster HMAC-based JWT parsing
* faster Memory engine, possibility to expire history stream metadata (more [in docs](https://centrifugal.github.io/centrifugo/server/engines/#memory-engine))
* releases for Centos 8, Debian Buster, Ubuntu Focal Fossa

Fixes:

* fix server side subscriptions to private channels (were ignored before)
* fix `channels` counter update frequency in server `info` – this includes how fast `channels` counter updated in admin web interface (previously `num clients` and `num users` updated once in 5 seconds while `num channels` only once in a minute, now `num channels` updated once in 5 seconds too)

This release based on Go 1.14.x
